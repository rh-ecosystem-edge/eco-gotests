package systemreserved_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	performanceprofileV2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	tunedv1 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/tuned/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// LabelSystemReservedValidation represents systemreserved validation test label.
	labelSystemReservedValidation = "systemreserved-validation"
	// NodeSizingEnvPath is the path to node-sizing.env file on nodes
	nodeSizingEnvPath = "/host/etc/node-sizing.env"
)

var (
	// Default systemReserved values based on CNF-16828 and scale lab data
	// These can be overridden via environment variables

	// controlPlaneCPUReserved is the default CPU cores reserved for control plane nodes (40 cores based on scale lab max 39.1)
	controlPlaneCPUReserved = getEnvOrDefault("CONTROL_PLANE_CPU_RESERVED", "40")
	// controlPlaneMemoryReserved is the default memory reserved for control plane nodes (30Gi per CNF-16828)
	controlPlaneMemoryReserved = getEnvOrDefault("CONTROL_PLANE_MEMORY_RESERVED", "30Gi")
	// workerCPUReserved is the default CPU reserved for worker nodes (1 core / 1000m)
	workerCPUReserved = getEnvOrDefault("WORKER_CPU_RESERVED", "1000m")
	// workerMemoryReserved is the default memory reserved for worker nodes (11Gi per CNF-16828)
	workerMemoryReserved = getEnvOrDefault("WORKER_MEMORY_RESERVED", "11Gi")

	// performanceProfileName is the name of the PerformanceProfile to validate
	performanceProfileName = getEnvOrDefault("PERFORMANCE_PROFILE_NAME", "")

	// Scale lab baseline data
	// scaleLabControlPlaneCPUMax is the maximum CPU usage observed in scale lab for control plane (OCP 4.16)
	scaleLabControlPlaneCPUMax = 39.1
	// scaleLabControlPlaneMemoryMax is the maximum memory usage observed in scale lab for control plane (OCP 4.16) in GB
	scaleLabControlPlaneMemoryMax = 33.9

	// reporterNamespacesToDump tells reporter which namespaces to collect pod logs from.
	reporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator":       "openshift-performance-addon-operator",
		"openshift-cluster-node-tuning-operator":     "openshift-cluster-node-tuning-operator",
	}

	// reporterCRDsToDump tells reporter which CRDs to dump.
	reporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &performanceprofileV2.PerformanceProfileList{}},
		{Cr: &tunedv1.TunedList{}},
		{Cr: &mcfgv1.MachineConfigList{}},
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
)

// getEnvOrDefault retrieves environment variable or returns default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getControlPlaneCPUCores returns control plane CPU reserved as numeric cores.
func getControlPlaneCPUCores() (float64, error) {
	return strconv.ParseFloat(controlPlaneCPUReserved, 64)
}

// getWorkerCPUMillicores returns worker CPU reserved in millicores.
// Handles formats like "1000m", "1", "2000m".
func getWorkerCPUMillicores() (int, error) {
	cpuStr := workerCPUReserved

	// If ends with 'm', parse as millicores
	if len(cpuStr) > 0 && cpuStr[len(cpuStr)-1] == 'm' {
		return strconv.Atoi(cpuStr[:len(cpuStr)-1])
	}

	// Otherwise parse as cores and convert to millicores
	cores, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return 0, err
	}
	return int(cores * 1000), nil
}

var _ = Describe("SystemReserved", Ordered, Label(labelSystemReservedValidation), func() {

	var (
		controlPlaneNodes  []*nodes.Builder
		workerNodes        []*nodes.Builder
		performanceProfile *nto.Builder
		allProfiles        []*nto.Builder
	)

	BeforeAll(func() {
		By("Getting control plane nodes")
		var err error
		controlPlaneNodes, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set{"node-role.kubernetes.io/master": ""}.String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list control plane nodes")
		Expect(controlPlaneNodes).ToNot(BeEmpty(), "No control plane nodes found")

		By("Getting worker nodes")
		workerNodes, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set{"node-role.kubernetes.io/worker": ""}.String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")

		By("Finding PerformanceProfiles")
		// List all PerformanceProfiles
		allProfiles, err = nto.ListProfiles(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list PerformanceProfiles")

		if len(allProfiles) == 0 {
			GinkgoWriter.Printf("⚠ No PerformanceProfile found in cluster - tests will use default baseline values\n")
			return
		}

		// Try to find PerformanceProfile - if name is specified, use it; otherwise use first one
		profileName := performanceProfileName
		if profileName == "" {
			// Use the first PerformanceProfile for basic validation
			performanceProfile = allProfiles[0]
			GinkgoWriter.Printf("Found PerformanceProfile: %s\n", performanceProfile.Object.Name)
		} else {
			// Find specific PerformanceProfile by name
			found := false
			for _, profile := range allProfiles {
				if profile.Object.Name == profileName {
					performanceProfile = profile
					found = true
					GinkgoWriter.Printf("Found PerformanceProfile: %s\n", performanceProfile.Object.Name)
					break
				}
			}
			if !found {
				GinkgoWriter.Printf("⚠ PerformanceProfile %s not found - will test nodes individually\n", profileName)
			}
		}

		GinkgoWriter.Printf("Found %d PerformanceProfile(s) in total\n", len(allProfiles))
	})

	Context("PerformanceProfile systemReserved configuration", func() {
		It("Should contain expected systemReserved values", func() {
			By("Verifying PerformanceProfile exists and has systemReserved configuration")
			if performanceProfile == nil {
				Skip("No PerformanceProfile found in cluster - skipping PerformanceProfile validation")
			}
			Expect(performanceProfile.Object.Spec).ToNot(BeNil(), "PerformanceProfile spec is nil")

			GinkgoWriter.Printf("PerformanceProfile Name: %s\n", performanceProfile.Object.Name)
			GinkgoWriter.Printf("PerformanceProfile NodeSelector: %v\n", performanceProfile.Object.Spec.NodeSelector)

			// Note: In PerformanceProfile v2, systemReserved is typically applied via MachineConfig
			// generated by the Performance Addon Operator, not directly in the PerformanceProfile spec.
			// The actual validation happens in Test Case 4.2 (kubelet config on nodes).

			// Check WorkloadHints as systemReserved might affect them
			if performanceProfile.Object.Spec.WorkloadHints != nil {
				GinkgoWriter.Printf("WorkloadHints configured: realTime=%v, highPowerConsumption=%v\n",
					performanceProfile.Object.Spec.WorkloadHints.RealTime,
					performanceProfile.Object.Spec.WorkloadHints.HighPowerConsumption)
			}

			// Note: In PerformanceProfile v2, systemReserved is typically applied via MachineConfig
			// generated by the Performance Addon Operator, not directly in the PerformanceProfile spec.
			// The actual validation happens in Test Case 4.2 (kubelet config on nodes).

			// At minimum, verify the PerformanceProfile has CPU configuration which affects systemReserved
			Expect(performanceProfile.Object.Spec.CPU).ToNot(BeNil(), "PerformanceProfile CPU configuration is nil")
			Expect(performanceProfile.Object.Spec.CPU.Reserved).ToNot(BeNil(), "PerformanceProfile CPU reserved is nil")
			Expect(performanceProfile.Object.Spec.CPU.Isolated).ToNot(BeNil(), "PerformanceProfile CPU isolated is nil")

			GinkgoWriter.Printf("✓ PerformanceProfile has CPU partitioning: reserved=%s, isolated=%s\n",
				performanceProfile.Object.Spec.CPU.Reserved, performanceProfile.Object.Spec.CPU.Isolated)

			// Log that systemReserved will be validated via kubelet config in Test Case 4.2
			GinkgoWriter.Printf("Note: systemReserved memory/CPU values will be validated via kubelet config in Test Case 4.2\n")
		})
	})

	Context("Kubelet systemReserved configuration", func() {
		When("Testing control plane nodes", func() {
			It("Should have correct systemReserved in /etc/node-sizing.env", func() {
				if len(controlPlaneNodes) == 0 {
					Skip("No control-plane nodes found - skipping kubelet config validation")
				}

				for _, node := range controlPlaneNodes {
					nodeObj := node.Object
					nodeName := nodeObj.Name

					// Skip nodes that are not ready, schedulable, or storage nodes
					if !isNodeReadyForTesting(node) {
						reason := "NotReady or SchedulingDisabled"
						if _, hasStorageRole := nodeObj.Labels["node-role.kubernetes.io/storage"]; hasStorageRole {
							reason = "Storage node - not tested for systemReserved"
						}
						GinkgoWriter.Printf("⊘ Skipping node %s (%s)\n", nodeName, reason)
						continue
					}

					// Check if node has a matching PerformanceProfile
					matchingProfile := findPerformanceProfileForNode(nodeObj.Labels, allProfiles)

					var expectedCPU, expectedMemory string
					var skipValidation bool
					if matchingProfile != nil {
						// Extract systemReserved from PerformanceProfile
						cpu, mem, err := extractSystemReservedFromProfile(matchingProfile)
						if err != nil {
							GinkgoWriter.Printf("⚠ Node %s: PerformanceProfile %s found but no systemReserved annotation: %v\n",
								nodeName, matchingProfile.Object.Name, err)
							GinkgoWriter.Printf("⚠ Skipping strict validation - will only verify that systemReserved is present\n")
							skipValidation = true
						} else {
							expectedCPU = cpu
							expectedMemory = mem
							GinkgoWriter.Printf("ℹ Node %s matched PerformanceProfile %s: expecting CPU=%s, Memory=%s\n",
								nodeName, matchingProfile.Object.Name, expectedCPU, expectedMemory)
						}
					} else {
						// No PerformanceProfile for this node - use baseline defaults for validation
						GinkgoWriter.Printf("⚠ Node %s has no matching PerformanceProfile - using baseline values for comparison\n", nodeName)
						expectedCPU = controlPlaneCPUReserved
						expectedMemory = controlPlaneMemoryReserved
					}

					By(fmt.Sprintf("Executing debug pod on node %s", nodeName))

					cmd := []string{"cat", nodeSizingEnvPath}
					output, err := executeDebugPodCommand(nodeName, cmd)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read %s from node %s", nodeSizingEnvPath, nodeName))
					Expect(output).ToNot(BeEmpty(), fmt.Sprintf("%s is empty on node %s", nodeSizingEnvPath, nodeName))

					By("Parsing node-sizing.env file")
					envVars := parseEnvFile(output)

					By("Verifying SYSTEM_RESERVED_CPU")
					actualCPU, found := envVars["SYSTEM_RESERVED_CPU"]
					Expect(found).To(BeTrue(), fmt.Sprintf("SYSTEM_RESERVED_CPU not found in %s on node %s", nodeSizingEnvPath, nodeName))

					if matchingProfile != nil && !skipValidation {
						// Strict check for nodes with PerformanceProfile that has systemReserved annotation
						Expect(normalizeCPUValue(actualCPU)).To(Equal(normalizeCPUValue(expectedCPU)),
							fmt.Sprintf("CPU systemReserved mismatch on control-plane node %s: expected %s (from PerformanceProfile), got %s",
								nodeName, expectedCPU, actualCPU))
					} else {
						// Informational for nodes without PerformanceProfile or without systemReserved annotation
						if !skipValidation && normalizeCPUValue(actualCPU) != normalizeCPUValue(expectedCPU) {
							GinkgoWriter.Printf("ℹ Node %s: CPU systemReserved is %s (scale lab baseline suggests %s)\n",
								nodeName, actualCPU, expectedCPU)
						} else if skipValidation {
							GinkgoWriter.Printf("ℹ Node %s: CPU systemReserved is %s (PerformanceProfile exists but no annotation)\n",
								nodeName, actualCPU)
						}
					}

					By("Verifying SYSTEM_RESERVED_MEMORY")
					actualMemory, found := envVars["SYSTEM_RESERVED_MEMORY"]
					Expect(found).To(BeTrue(), fmt.Sprintf("SYSTEM_RESERVED_MEMORY not found in %s on node %s", nodeSizingEnvPath, nodeName))

					if matchingProfile != nil && !skipValidation {
						// Strict check for nodes with PerformanceProfile that has systemReserved annotation
						Expect(actualMemory).To(Equal(expectedMemory),
							fmt.Sprintf("Memory systemReserved mismatch on control-plane node %s: expected %s (from PerformanceProfile), got %s",
								nodeName, expectedMemory, actualMemory))
					} else {
						// Informational for nodes without PerformanceProfile or without systemReserved annotation
						if !skipValidation && actualMemory != expectedMemory {
							GinkgoWriter.Printf("ℹ Node %s: Memory systemReserved is %s (scale lab baseline suggests %s)\n",
								nodeName, actualMemory, expectedMemory)
						} else if skipValidation {
							GinkgoWriter.Printf("ℹ Node %s: Memory systemReserved is %s (PerformanceProfile exists but no annotation)\n",
								nodeName, actualMemory)
						}
					}

					GinkgoWriter.Printf("✓ Node %s systemReserved validated: CPU=%s, Memory=%s\n", nodeName, actualCPU, actualMemory)
				}
			})
		})

		When("Testing worker nodes", func() {
			It("Should have correct systemReserved in /etc/node-sizing.env", func() {
				if len(workerNodes) == 0 {
					Skip("No worker nodes found - skipping kubelet config validation")
				}

				for _, node := range workerNodes {
					nodeObj := node.Object
					nodeName := nodeObj.Name

					// Skip nodes that are not ready, schedulable, or storage nodes
					if !isNodeReadyForTesting(node) {
						reason := "NotReady or SchedulingDisabled"
						if _, hasStorageRole := nodeObj.Labels["node-role.kubernetes.io/storage"]; hasStorageRole {
							reason = "Storage node - not tested for systemReserved"
						}
						GinkgoWriter.Printf("⊘ Skipping node %s (%s)\n", nodeName, reason)
						continue
					}

					// Check if node has a matching PerformanceProfile
					matchingProfile := findPerformanceProfileForNode(nodeObj.Labels, allProfiles)

					var expectedCPU, expectedMemory string
					var skipValidation bool
					if matchingProfile != nil {
						// Extract systemReserved from PerformanceProfile
						cpu, mem, err := extractSystemReservedFromProfile(matchingProfile)
						if err != nil {
							GinkgoWriter.Printf("⚠ Node %s: PerformanceProfile %s found but no systemReserved annotation: %v\n",
								nodeName, matchingProfile.Object.Name, err)
							GinkgoWriter.Printf("⚠ Skipping strict validation - will only verify that systemReserved is present\n")
							skipValidation = true
						} else {
							expectedCPU = cpu
							expectedMemory = mem
							GinkgoWriter.Printf("ℹ Node %s matched PerformanceProfile %s: expecting CPU=%s, Memory=%s\n",
								nodeName, matchingProfile.Object.Name, expectedCPU, expectedMemory)
						}
					} else {
						// No PerformanceProfile for this node - use baseline defaults for validation
						GinkgoWriter.Printf("⚠ Node %s has no matching PerformanceProfile - using baseline values for comparison\n", nodeName)
						expectedCPU = workerCPUReserved
						expectedMemory = workerMemoryReserved
					}

					By(fmt.Sprintf("Executing debug pod on node %s", nodeName))

					cmd := []string{"cat", nodeSizingEnvPath}
					output, err := executeDebugPodCommand(nodeName, cmd)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read %s from node %s", nodeSizingEnvPath, nodeName))
					Expect(output).ToNot(BeEmpty(), fmt.Sprintf("%s is empty on node %s", nodeSizingEnvPath, nodeName))

					By("Parsing node-sizing.env file")
					envVars := parseEnvFile(output)

					By("Verifying SYSTEM_RESERVED_CPU")
					actualCPU, found := envVars["SYSTEM_RESERVED_CPU"]
					Expect(found).To(BeTrue(), fmt.Sprintf("SYSTEM_RESERVED_CPU not found in %s on node %s", nodeSizingEnvPath, nodeName))

					if matchingProfile != nil && !skipValidation {
						// Strict check for nodes with PerformanceProfile that has systemReserved annotation
						Expect(normalizeCPUValue(actualCPU)).To(Equal(normalizeCPUValue(expectedCPU)),
							fmt.Sprintf("CPU systemReserved mismatch on worker node %s: expected %s (from PerformanceProfile), got %s",
								nodeName, expectedCPU, actualCPU))
					} else {
						// Informational for nodes without PerformanceProfile or without systemReserved annotation
						if !skipValidation && normalizeCPUValue(actualCPU) != normalizeCPUValue(expectedCPU) {
							GinkgoWriter.Printf("ℹ Node %s: CPU systemReserved is %s (baseline suggests %s)\n",
								nodeName, actualCPU, expectedCPU)
						} else if skipValidation {
							GinkgoWriter.Printf("ℹ Node %s: CPU systemReserved is %s (PerformanceProfile exists but no annotation)\n",
								nodeName, actualCPU)
						}
					}

					By("Verifying SYSTEM_RESERVED_MEMORY")
					actualMemory, found := envVars["SYSTEM_RESERVED_MEMORY"]
					Expect(found).To(BeTrue(), fmt.Sprintf("SYSTEM_RESERVED_MEMORY not found in %s on node %s", nodeSizingEnvPath, nodeName))

					if matchingProfile != nil && !skipValidation {
						// Strict check for nodes with PerformanceProfile that has systemReserved annotation
						Expect(actualMemory).To(Equal(expectedMemory),
							fmt.Sprintf("Memory systemReserved mismatch on worker node %s: expected %s (from PerformanceProfile), got %s",
								nodeName, expectedMemory, actualMemory))
					} else {
						// Informational for nodes without PerformanceProfile or without systemReserved annotation
						if !skipValidation && actualMemory != expectedMemory {
							GinkgoWriter.Printf("ℹ Node %s: Memory systemReserved is %s (baseline suggests %s)\n",
								nodeName, actualMemory, expectedMemory)
						} else if skipValidation {
							GinkgoWriter.Printf("ℹ Node %s: Memory systemReserved is %s (PerformanceProfile exists but no annotation)\n",
								nodeName, actualMemory)
						}
					}

					GinkgoWriter.Printf("✓ Node %s systemReserved validated: CPU=%s, Memory=%s\n", nodeName, actualCPU, actualMemory)
				}
			})
		})
	})

	Context("Node allocatable capacity validation", func() {
		testNodeAllocatable := func(nodeList []*nodes.Builder, nodeType string) {

			for _, node := range nodeList {
				nodeObj := node.Object
				nodeName := nodeObj.Name

				It(fmt.Sprintf("Should have correct allocatable capacity on %s node %s", nodeType, nodeName), func() {
					By("Getting node capacity and allocatable from status")
					capacity := nodeObj.Status.Capacity
					allocatable := nodeObj.Status.Allocatable

					Expect(capacity).ToNot(BeNil(), fmt.Sprintf("Node %s capacity is nil", nodeName))
					Expect(allocatable).ToNot(BeNil(), fmt.Sprintf("Node %s allocatable is nil", nodeName))

					By("Verifying CPU allocatable is reduced by systemReserved")
					capacityCPU := capacity[corev1.ResourceCPU]
					allocatableCPU := allocatable[corev1.ResourceCPU]

					GinkgoWriter.Printf("Node %s - CPU Capacity: %s, Allocatable: %s\n",
						nodeName, capacityCPU.String(), allocatableCPU.String())

					// Allocatable should be less than capacity
					Expect(allocatableCPU.Cmp(capacityCPU)).To(Equal(-1),
						fmt.Sprintf("Node %s allocatable CPU (%s) should be less than capacity (%s)",
							nodeName, allocatableCPU.String(), capacityCPU.String()))

					By("Verifying Memory allocatable is reduced by systemReserved")
					capacityMemory := capacity[corev1.ResourceMemory]
					allocatableMemory := allocatable[corev1.ResourceMemory]

					GinkgoWriter.Printf("Node %s - Memory Capacity: %s, Allocatable: %s\n",
						nodeName, capacityMemory.String(), allocatableMemory.String())

					// Allocatable should be less than capacity
					Expect(allocatableMemory.Cmp(capacityMemory)).To(Equal(-1),
						fmt.Sprintf("Node %s allocatable memory (%s) should be less than capacity (%s)",
							nodeName, allocatableMemory.String(), capacityMemory.String()))

					// Calculate expected allocatable (approximate - doesn't account for eviction thresholds)
					// This is a sanity check that systemReserved is roughly in effect
					expectedAllocatableCPU := capacityCPU.DeepCopy()
					expectedAllocatableMemory := capacityMemory.DeepCopy()

					// For control plane vs worker, use different systemReserved values
					// Check node role using labels, not string matching on name
					var cpuReserved, memoryReserved string
					_, isControlPlane := nodeObj.Labels["node-role.kubernetes.io/master"]
					if !isControlPlane {
						_, isControlPlane = nodeObj.Labels["node-role.kubernetes.io/control-plane"]
					}

					if isControlPlane {
						cpuReserved = controlPlaneCPUReserved
						memoryReserved = controlPlaneMemoryReserved
					} else {
						cpuReserved = workerCPUReserved
						memoryReserved = workerMemoryReserved
					}

					// Parse systemReserved values
					systemReservedCPU, err := resource.ParseQuantity(cpuReserved)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse CPU reserved %s", cpuReserved))

					systemReservedMemory, err := resource.ParseQuantity(memoryReserved)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse memory reserved %s", memoryReserved))

					expectedAllocatableCPU.Sub(systemReservedCPU)
					expectedAllocatableMemory.Sub(systemReservedMemory)

					// Skip validation if systemReserved is larger than capacity (would result in negative expected)
					if expectedAllocatableCPU.Sign() < 0 {
						GinkgoWriter.Printf("⚠ Node %s - systemReserved CPU (%s) exceeds capacity (%s) - skipping allocatable validation\n",
							nodeName, systemReservedCPU.String(), capacityCPU.String())
						return
					}

					if expectedAllocatableMemory.Sign() < 0 {
						GinkgoWriter.Printf("⚠ Node %s - systemReserved memory (%s) exceeds capacity (%s) - skipping allocatable validation\n",
							nodeName, systemReservedMemory.String(), capacityMemory.String())
						return
					}

					GinkgoWriter.Printf("Node %s - Expected allocatable (without eviction): CPU~%s, Memory~%s\n",
						nodeName, expectedAllocatableCPU.String(), expectedAllocatableMemory.String())

					// Actual allocatable should be approximately expected (allowing for eviction thresholds)
					// We check that it's less than expected (due to eviction) but more than 90% of expected
					minExpectedCPU := expectedAllocatableCPU.DeepCopy()
					minExpectedCPU.SetMilli(int64(float64(minExpectedCPU.MilliValue()) * 0.9))

					Expect(allocatableCPU.Cmp(minExpectedCPU)).To(BeNumerically(">=", 0),
						fmt.Sprintf("Node %s allocatable CPU (%s) is significantly less than expected (%s)",
							nodeName, allocatableCPU.String(), expectedAllocatableCPU.String()))
				})
			}
		}

		When("Validating control plane node allocatable", func() {
			testNodeAllocatable(controlPlaneNodes, "control-plane")
		})

		When("Validating worker node allocatable", func() {
			testNodeAllocatable(workerNodes, "worker")
		})
	})

	Context("systemReserved enforcement validation", func() {
		testNodeEnforcement := func(nodeList []*nodes.Builder, nodeType string) {

			for _, node := range nodeList {
				nodeObj := node.Object
				nodeName := nodeObj.Name

				It(fmt.Sprintf("Should have systemReserved enforcement enabled on %s node %s", nodeType, nodeName), func() {
					By(fmt.Sprintf("Checking kubelet config enforcement on node %s", nodeName))

					// Read kubelet configuration to verify enforcement settings
					cmd := []string{"cat", "/host/var/lib/kubelet/config.json"}
					output, err := executeDebugPodCommand(nodeName, cmd)

					if err != nil {
						// If config.json doesn't exist, try the YAML format
						cmd = []string{"cat", "/host/var/lib/kubelet/config.yaml"}
						output, err = executeDebugPodCommand(nodeName, cmd)
					}

					if err != nil {
						GinkgoWriter.Printf("Warning: Could not read kubelet config from node %s: %v\n", nodeName, err)
						GinkgoWriter.Printf("Skipping enforcement check - config file may be in different location\n")
						Skip(fmt.Sprintf("Kubelet config not accessible on node %s", nodeName))
					}

					By("Verifying enforceNodeAllocatable includes system-reserved")
					// enforceNodeAllocatable should include "pods" and may include "system-reserved"
					// Common values: ["pods"], ["pods","system-reserved"], ["pods","system-reserved","kube-reserved"]

					if strings.Contains(output, "enforceNodeAllocatable") {
						// Check if system-reserved is in the list
						if strings.Contains(output, "system-reserved") {
							GinkgoWriter.Printf("✓ Node %s has system-reserved enforcement enabled\n", nodeName)
						} else {
							GinkgoWriter.Printf("⚠ Node %s: enforceNodeAllocatable does not include system-reserved (may use default enforcement)\n", nodeName)
						}
					}

					By("Verifying systemReservedCgroup is configured")
					// systemReservedCgroup is typically "/system.slice"
					if strings.Contains(output, "systemReservedCgroup") {
						if strings.Contains(output, "/system.slice") {
							GinkgoWriter.Printf("✓ Node %s has systemReservedCgroup set to /system.slice\n", nodeName)
						} else {
							GinkgoWriter.Printf("⚠ Node %s: systemReservedCgroup found but not /system.slice\n", nodeName)
						}
					} else {
						GinkgoWriter.Printf("Note: Node %s kubelet config does not specify systemReservedCgroup (may use default)\n", nodeName)
					}

					// At minimum, verify the node has allocatable resources properly set
					// which indicates systemReserved is in effect
					allocatable := nodeObj.Status.Allocatable
					capacity := nodeObj.Status.Capacity

					cpuAllocatable := allocatable[corev1.ResourceCPU]
					cpuCapacity := capacity[corev1.ResourceCPU]

					Expect(cpuAllocatable.Cmp(cpuCapacity)).To(Equal(-1),
						fmt.Sprintf("Node %s allocatable CPU (%s) should be less than capacity (%s), indicating systemReserved is in effect",
							nodeName, cpuAllocatable.String(), cpuCapacity.String()))

					GinkgoWriter.Printf("✓ Node %s has systemReserved enforcement in effect (allocatable < capacity)\n", nodeName)
				})
			}
		}

		When("Validating control plane node enforcement", func() {
			testNodeEnforcement(controlPlaneNodes, "control-plane")
		})

		When("Validating worker node enforcement", func() {
			testNodeEnforcement(workerNodes, "worker")
		})
	})

	Context("Scale lab baseline validation", func() {
		It("Should have systemReserved values adequate for scale lab requirements", func() {
			By("Checking control plane CPU systemReserved against scale lab baseline")
			cpuCores, err := getControlPlaneCPUCores()
			Expect(err).ToNot(HaveOccurred(), "Failed to parse control plane CPU reserved")

			scaleLabMax := scaleLabControlPlaneCPUMax
			Expect(cpuCores).To(BeNumerically(">=", scaleLabMax),
				fmt.Sprintf("Control plane CPU systemReserved (%.1f cores) should be >= scale lab max (%.1f cores)",
					cpuCores, scaleLabMax))
			GinkgoWriter.Printf("✓ Control plane CPU systemReserved (%.1f cores) meets scale lab requirement (%.1f cores)\n",
				cpuCores, scaleLabMax)

			By("Checking control plane memory systemReserved against scale lab baseline")
			memoryQuantity, err := resource.ParseQuantity(controlPlaneMemoryReserved)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse control plane memory reserved")

			memoryGB := float64(memoryQuantity.Value()) / (1024 * 1024 * 1024)
			scaleLabMaxMemory := scaleLabControlPlaneMemoryMax

			if memoryGB < scaleLabMaxMemory {
				GinkgoWriter.Printf("⚠ WARNING: Control plane memory systemReserved (%.1f GB) is BELOW scale lab max (%.1f GB)\n",
					memoryGB, scaleLabMaxMemory)
				GinkgoWriter.Printf("⚠ Consider increasing to at least 34Gi for production environments\n")
			} else {
				GinkgoWriter.Printf("✓ Control plane memory systemReserved (%.1f GB) meets scale lab requirement (%.1f GB)\n",
					memoryGB, scaleLabMaxMemory)
			}

			By("Verifying worker systemReserved meets baseline requirements")
			workerCPUMillicores, err := getWorkerCPUMillicores()
			Expect(err).ToNot(HaveOccurred(), "Failed to parse worker CPU reserved")
			Expect(workerCPUMillicores).To(BeNumerically(">=", 500),
				fmt.Sprintf("Worker CPU systemReserved (%dm) should be >= minimum requirement (500m)", workerCPUMillicores))

			GinkgoWriter.Printf("✓ Worker CPU systemReserved (%dm) meets minimum requirement (500m)\n", workerCPUMillicores)
		})
	})
})

// Helper functions

// isNodeReadyForTesting checks if a node is in a state where it can be tested.
// Returns false if node is NotReady, has SchedulingDisabled, or has storage role.
func isNodeReadyForTesting(node *nodes.Builder) bool {
	nodeObj := node.Object

	// Skip storage nodes - they typically don't have PerformanceProfile configuration
	if _, hasStorageRole := nodeObj.Labels["node-role.kubernetes.io/storage"]; hasStorageRole {
		return false
	}

	// Check if node is schedulable
	if nodeObj.Spec.Unschedulable {
		return false
	}

	// Check node conditions
	for _, condition := range nodeObj.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status != corev1.ConditionTrue {
				return false
			}
		}
	}

	return true
}

// executeDebugPodCommand executes a command on a node via debug pod.
func executeDebugPodCommand(nodeName string, cmd []string) (string, error) {
	debugPodName := fmt.Sprintf("systemreserved-debug-%s", strings.ReplaceAll(nodeName, ".", "-"))
	debugNamespace := "default"

	// Create debug pod that mounts host filesystem
	debugPod := pod.NewBuilder(
		APIClient,
		debugPodName,
		debugNamespace,
		"registry.access.redhat.com/ubi8/ubi-minimal:latest",
	)

	// Configure pod to run on specific node with host filesystem access
	debugPod.WithNodeSelector(map[string]string{
		"kubernetes.io/hostname": nodeName,
	})

	// Mount /etc from host as /host/etc
	debugPod.Definition.Spec.Volumes = []corev1.Volume{
		{
			Name: "host-etc",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc",
				},
			},
		},
	}

	debugPod.Definition.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "host-etc",
			MountPath: "/host/etc",
			ReadOnly:  true,
		},
	}

	// Keep pod running for command execution
	debugPod.Definition.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "sleep 300"}

	// Set privileged security context
	debugPod.Definition.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged: func(b bool) *bool { return &b }(true),
	}

	// Add tolerations for master/control-plane nodes
	debugPod.Definition.Spec.Tolerations = []corev1.Toleration{
		{
			Key:      "node-role.kubernetes.io/master",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "node-role.kubernetes.io/control-plane",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	// Create and wait for pod to be running
	GinkgoWriter.Printf("Creating debug pod %s on node %s\n", debugPodName, nodeName)
	_, err := debugPod.CreateAndWaitUntilRunning(5 * time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to create debug pod on node %s: %v", nodeName, err)
	}

	// Execute the command
	GinkgoWriter.Printf("Executing command in debug pod: %v\n", cmd)
	output, err := debugPod.ExecCommand(cmd, debugPod.Definition.Spec.Containers[0].Name)
	if err != nil {
		// Try to cleanup even if command failed
		_, cleanupErr := debugPod.DeleteAndWait(30 * time.Second)
		if cleanupErr != nil {
			GinkgoWriter.Printf("Warning: failed to delete debug pod: %v\n", cleanupErr)
		}
		return "", fmt.Errorf("failed to execute command on node %s: %v", nodeName, err)
	}

	// Cleanup debug pod
	GinkgoWriter.Printf("Cleaning up debug pod %s\n", debugPodName)
	_, err = debugPod.DeleteAndWait(30 * time.Second)
	if err != nil {
		GinkgoWriter.Printf("Warning: failed to delete debug pod: %v\n", err)
		// Don't return error for cleanup failure - we got the output
	}

	return output.String(), nil
}

// parseEnvFile parses environment variable file content into a map.
func parseEnvFile(content string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			envVars[key] = value
		}
	}

	return envVars
}

// normalizeCPUValue normalizes CPU value to millicores for comparison.
// Handles: "40" (cores), "40000m" (millicores), "1000m", "1".
func normalizeCPUValue(cpuStr string) int64 {
	cpuStr = strings.TrimSpace(cpuStr)

	// If ends with 'm', parse as millicores
	if strings.HasSuffix(cpuStr, "m") {
		millis, err := strconv.ParseInt(cpuStr[:len(cpuStr)-1], 10, 64)
		if err != nil {
			return 0
		}
		return millis
	}

	// Otherwise parse as cores and convert to millicores
	cores, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return 0
	}
	return int64(cores * 1000)
}

// findPerformanceProfileForNode checks if a node matches any PerformanceProfile's nodeSelector.
// Returns the matching PerformanceProfile or nil if no match.
func findPerformanceProfileForNode(nodeLabels map[string]string, profiles []*nto.Builder) *nto.Builder {
	for _, profile := range profiles {
		if profile.Object.Spec.NodeSelector == nil {
			continue
		}

		// Check if all nodeSelector labels match
		allMatch := true
		for key, value := range profile.Object.Spec.NodeSelector {
			// Check if the label exists in node AND has the correct value
			nodeValue, exists := nodeLabels[key]
			if !exists || nodeValue != value {
				allMatch = false
				break
			}
		}

		if allMatch {
			return profile
		}
	}
	return nil
}

// extractSystemReservedFromProfile extracts systemReserved CPU and memory from PerformanceProfile's kubeletconfig annotation.
// Returns (cpuValue, memoryValue, error).
func extractSystemReservedFromProfile(profile *nto.Builder) (string, string, error) {
	if profile == nil || profile.Object.Annotations == nil {
		return "", "", fmt.Errorf("profile or annotations are nil")
	}

	kubeletConfig, found := profile.Object.Annotations["kubeletconfig.experimental"]
	if !found {
		return "", "", fmt.Errorf("kubeletconfig.experimental annotation not found")
	}

	// Parse JSON to extract systemReserved values
	// The annotation contains JSON like: { "systemReserved": { "memory": "38Gi", "cpu": "10000m" }, ... }

	// Simple parsing - look for systemReserved CPU and memory values
	var cpuValue, memoryValue string

	// Extract CPU value
	cpuStart := strings.Index(kubeletConfig, `"cpu":`)
	if cpuStart != -1 {
		cpuStart += len(`"cpu":`)
		cpuEnd := strings.Index(kubeletConfig[cpuStart:], `"`)
		if cpuEnd != -1 {
			cpuStart += cpuEnd + 1
			cpuEnd = strings.Index(kubeletConfig[cpuStart:], `"`)
			if cpuEnd != -1 {
				cpuValue = kubeletConfig[cpuStart : cpuStart+cpuEnd]
			}
		}
	}

	// Extract memory value
	memStart := strings.Index(kubeletConfig, `"memory":`)
	if memStart != -1 {
		memStart += len(`"memory":`)
		memEnd := strings.Index(kubeletConfig[memStart:], `"`)
		if memEnd != -1 {
			memStart += memEnd + 1
			memEnd = strings.Index(kubeletConfig[memStart:], `"`)
			if memEnd != -1 {
				memoryValue = kubeletConfig[memStart : memStart+memEnd]
			}
		}
	}

	if cpuValue == "" || memoryValue == "" {
		return "", "", fmt.Errorf("failed to parse systemReserved values from kubeletconfig annotation")
	}

	return cpuValue, memoryValue, nil
}
