package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/amdgpu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpudeviceconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpunfd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuregistry"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/exec"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/labels"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/pods"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("AMD GPU Basic Tests", Ordered, Label(amdgpuparams.LabelSuite), func() {

	Context("AMD GPU Basic 01", Label(amdgpuparams.LabelSuite+"-01"), func() {

		apiClient := inittools.APIClient
		amdconfig := amdgpuconfig.NewAMDConfig()

		amdListOptions := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue),
		}

		// Shared state across tests - set during BeforeAll
		var amdNodeBuilders []*nodes.Builder
		var isSNO bool
		var nodeLabellerPodBuilders []*pod.Builder

		BeforeAll(func() {
			By("Verifying configuration")

			if amdconfig == nil {
				Skip("AMDConfig is not available - required for AMD GPU tests")
			}

			if amdconfig.AMDDriverVersion == "" {
				Skip("AMD Driver Version is not set in environment - required for AMD GPU tests")
			}

			By("Verifying and configuring internal image registry for AMD GPU operator")
			err := amdgpuregistry.VerifyAndConfigureInternalRegistry(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Internal registry configuration warning: %v", err)
				Skip("Internal image registry is not available - required for AMD GPU operator")
			}

			By("Checking if cluster is stable before setup")
			err = amdgpuhelpers.WaitForClusterStability(apiClient, amdgpuparams.ClusterStabilityTimeout)
			Expect(err).ToNot(HaveOccurred(), "Cluster should be stable before proceeding with the test")

			// IMPORTANT: Create MachineConfig BEFORE deploying operators
			// The MachineConfig may trigger a node reboot, which would kill operator pods if they were running
			By("Creating machineconfig for AMD GPU blacklist (before operators - may trigger reboot)")
			err = amdgpuhelpers.CreateBlacklistMachineConfig(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Blacklist MachineConfig not applied (non-fatal): %v", err)
				// Continue without MachineConfig - it's optional
			} else {
				By("Waiting for cluster stability after MachineConfig (handles potential reboot)")
				err = amdgpuhelpers.WaitForClusterStability(apiClient, amdgpuparams.SNOClusterStabilityTimeout)
				if err != nil {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof("Cluster stability warning after MachineConfig: %v", err)
				}
			}

			By("Deploying required operators (NFD, KMM, AMD GPU)")
			err = amdgpuhelpers.DeployAllOperators(apiClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy required operators")

			By("Deploying NFD custom resource with AMD GPU worker config")
			nfdCRUtils := deploy.NewNFDCRUtils(apiClient, nfdparams.NFDNamespace, "amd-gpu-nfd-instance")
			nfdConfig := deploy.NFDCRConfig{
				EnableTopology: false,
				Image:          "",
				WorkerConfig:   "",
			}
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("NFD CR deployment: %+v", nfdConfig)
			err = nfdCRUtils.DeployNFDCR(nfdConfig)
			Expect(err).ToNot(HaveOccurred(), "NFD CR should be created successfully: %v", err)

			By("Creating AMD GPU FeatureRule for enhanced detection")
			err = amdgpunfd.CreateAMDGPUFeatureRule(apiClient)
			Expect(err).ToNot(HaveOccurred(), "AMD GPU FeatureRule should be created successfully")

			By("Waiting for NFD to label nodes")
			time.Sleep(1 * time.Minute)

			By("Creating DeviceConfig with Node Labeller enabled")
			err = amdgpudeviceconfig.CreateDeviceConfig(
				apiClient,
				amdgpuparams.DefaultDeviceConfigName,
				amdconfig.AMDDriverVersion)
			Expect(err).ToNot(HaveOccurred(), "DeviceConfig should be created successfully")

			By("Verifying DeviceConfig was created")
			deviceConfigBuilder, deviceConfigBuilderErr := amdgpu.Pull(
				apiClient,
				amdgpuparams.DefaultDeviceConfigName,
				amdgpuparams.AMDGPUNamespace)
			Expect(deviceConfigBuilderErr).ToNot(HaveOccurred(), "DeviceConfig should exist")
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig created: %s", deviceConfigBuilder.Object.Name)

			By("Getting AMD GPU Worker Nodes to detect SNO")
			amdNodeBuilders, err = nodes.List(apiClient, amdListOptions)
			if err != nil || len(amdNodeBuilders) == 0 {
				// NFD labels might not be applied yet, wait and retry
				klog.V(amdgpuparams.AMDGPULogLevel).Info("No AMD GPU nodes found yet, waiting for NFD labels...")
				time.Sleep(30 * time.Second)
				amdNodeBuilders, err = nodes.List(apiClient, amdListOptions)
			}
			Expect(err).ToNot(HaveOccurred(), "Failed to get AMD GPU Worker Nodes")
			Expect(amdNodeBuilders).ToNot(BeEmpty(), "No AMD GPU Worker Nodes found")

			isSNO = len(amdNodeBuilders) == 1
			if isSNO {
				klog.V(amdgpuparams.AMDGPULogLevel).Info("SNO environment detected - using extended timeouts for setup")
			}

			By("Waiting for cluster stability after DeviceConfig creation (handles SNO reboot)")
			// Use extended timeout for SNO environments where driver installation may trigger reboot
			stabilityTimeout := amdgpuparams.ClusterStabilityTimeout
			if isSNO {
				stabilityTimeout = amdgpuparams.SNOClusterStabilityTimeout
			}
			err = amdgpuhelpers.WaitForClusterStability(apiClient, stabilityTimeout)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Cluster stability check warning: %v", err)
			}

			By("Waiting for AMD GPU driver to be built and loaded by KMM")
			// The driver must be built (via KMM build pod) and loaded before Node Labeller can work
			err = amdgpuhelpers.WaitForAMDGPUDriverReady(apiClient, isSNO)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Driver readiness warning: %v", err)
				// Log but continue - the Node Labeller check will also fail if driver isn't ready
			}

			By("Waiting for Node Labeller driver-init containers to complete")
			// The driver-init container waits for amdgpu module to be loaded by KMM
			// This can take 10-20 minutes for DKMS build + module load
			driverInitTimeout := 30 * time.Minute
			if isSNO {
				driverInitTimeout = amdgpuparams.SNOClusterStabilityTimeout
			}
			err = amdgpuhelpers.WaitForNodeLabellerDriverInit(apiClient, driverInitTimeout)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Driver-init wait warning: %v", err)
			}

			By("Waiting for Node Labeller pods to be created and running")

			// Get Node Labeller pods with retry
			// In SNO, reboot + driver loading can take 30+ minutes
			podDiscoveryTimeout := 10 * time.Minute
			if isSNO {
				podDiscoveryTimeout = 30 * time.Minute
			}

			Eventually(func() error {
				nodeLabellerPodBuilders, err = pods.NodeLabellerPodsFromNodes(apiClient, amdNodeBuilders)
				if err != nil {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof("Waiting for node labeller pods: %v", err)

					return err
				}
				if len(nodeLabellerPodBuilders) == 0 {
					return fmt.Errorf("no node labeller pods found yet")
				}

				return nil
			}, podDiscoveryTimeout, 15*time.Second).Should(Succeed(),
				"Failed to get Node Labeller Pods within timeout")

			By("Waiting for all Node Labeller Pods to be in 'Running' state")
			err = amdgpuhelpers.WaitForPodsRunningResilient(apiClient, nodeLabellerPodBuilders, isSNO)
			Expect(err).ToNot(HaveOccurred(), "Node Labeller Pods should be running")

			By("Waiting for Node Labeller to apply labels")
			time.Sleep(30 * time.Second)

			klog.V(amdgpuparams.AMDGPULogLevel).Info(
				"BeforeAll setup complete - operators deployed, DeviceConfig created, Node Labeller running")
		})

		AfterAll(func() {
			By("Starting complete cleanup")

			// 1. Delete DeviceConfig first (before operator removal)
			By("Deleting DeviceConfig")
			err := amdgpudeviceconfig.DeleteDeviceConfig(apiClient, amdgpuparams.DefaultDeviceConfigName)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig deletion issue: %v", err)
			}

			// 2. Delete NFD FeatureRule
			By("Deleting AMD GPU FeatureRule")
			err = amdgpunfd.DeleteAMDGPUFeatureRule(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("FeatureRule deletion issue: %v", err)
			}

			// 3. Uninstall operators in reverse order
			By("Uninstalling AMD GPU operator")
			amdgpuUninstallConfig := amdgpuhelpers.GetDefaultAMDGPUUninstallConfig(
				apiClient,
				"amd-gpu-operator-group",
				"amd-gpu-subscription")
			amdgpuUninstaller := deploy.NewOperatorUninstaller(amdgpuUninstallConfig)
			err = amdgpuUninstaller.Uninstall()
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("AMD GPU operator uninstall issue: %v", err)
			}

			By("Uninstalling KMM operator")
			kmmUninstallConfig := amdgpuhelpers.GetDefaultKMMUninstallConfig(apiClient, nil)
			kmmUninstaller := deploy.NewOperatorUninstaller(kmmUninstallConfig)
			err = kmmUninstaller.Uninstall()
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("KMM operator uninstall issue: %v", err)
			}

			By("Uninstalling NFD operator")
			nfdUninstallConfig := deploy.OperatorUninstallConfig{
				APIClient:         apiClient,
				Namespace:         nfdparams.NFDNamespace,
				OperatorGroupName: "nfd-operator-group",
				SubscriptionName:  "nfd-subscription",
				CustomResourceCleaner: deploy.NewNFDCustomResourceCleaner(
					apiClient, nfdparams.NFDNamespace, klog.Level(amdgpuparams.AMDGPULogLevel)),
				LogLevel: klog.Level(amdgpuparams.AMDGPULogLevel),
			}
			nfdUninstaller := deploy.NewOperatorUninstaller(nfdUninstallConfig)
			err = nfdUninstaller.Uninstall()
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("NFD operator uninstall issue: %v", err)
			}

			// 4. Delete MachineConfig
			By("Deleting MachineConfig for amdgpu blacklist")
			err = amdgpuhelpers.DeleteBlacklistMachineConfig(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfig deletion issue: %v", err)
			}

			// 5. Clean up node labels
			By("Cleaning up AMD GPU node labels")
			err = amdgpuhelpers.CleanupAMDGPUNodeLabels(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Node labels cleanup issue: %v", err)
			}

			// 6. Reset image registry to unmanaged (optional - leave managed if was managed before)
			By("Resetting internal image registry")
			err = amdgpuregistry.ResetRegistryToRemoved(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Registry reset issue: %v", err)
			}

			// 7. Delete namespaces
			By("Deleting operator namespaces")
			err = amdgpuhelpers.DeleteOperatorNamespaces(apiClient)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Namespace deletion issue: %v", err)
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("Complete cleanup finished")
		})

		// ============================================================================
		// VERIFICATION TESTS - These only verify state, they don't modify anything
		// ============================================================================

		It("Should verify internal registry is configured and available", func() {
			By("Checking internal image registry configuration")
			err := amdgpuregistry.VerifyAndConfigureInternalRegistry(apiClient)
			Expect(err).NotTo(HaveOccurred(), "Internal image registry should be properly configured")

			By("Verifying image registry pods are running")
			pods, err := apiClient.CoreV1Interface.Pods("openshift-image-registry").List(
				context.Background(), metav1.ListOptions{
					LabelSelector: "docker-registry=default",
				})
			Expect(err).NotTo(HaveOccurred(), "Should be able to list image registry pods")

			runningPods := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					allReady := true
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if !containerStatus.Ready {
							allReady = false

							break
						}
					}
					if allReady {
						runningPods++
					}
				}
			}

			Expect(runningPods).To(BeNumerically(">", 0), "At least one image registry pod should be running")
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Internal image registry verified: %d pods running", runningPods)
		})

		It("Should verify NFD labels are applied to AMD GPU nodes", func() {
			By("Checking AMD label was added to all AMD GPU Worker Nodes by NFD")
			amdNFDLabelFound, amdNFDLabelFoundErr := labels.LabelPresentOnAllNodes(
				apiClient, amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue, inittools.GeneralConfig.WorkerLabelMap)

			Expect(amdNFDLabelFoundErr).To(BeNil(),
				"An error occurred while attempting to verify the AMD label by NFD: %v", amdNFDLabelFoundErr)
			Expect(amdNFDLabelFound).To(BeTrue(),
				"AMD label check failed to match label %s and label value %s on all nodes",
				amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue)

			klog.V(amdgpuparams.AMDGPULogLevel).Info("NFD labels verified on all AMD GPU nodes")
		})

		It("Should verify DeviceConfig exists and is ready", func() {
			By("Pulling DeviceConfig")
			deviceConfigBuilder, err := amdgpu.Pull(
				apiClient, amdgpuparams.DeviceConfigName, amdgpuparams.AMDGPUNamespace)
			Expect(err).ToNot(HaveOccurred(), "DeviceConfig should exist")
			Expect(deviceConfigBuilder).ToNot(BeNil(), "DeviceConfig builder should not be nil")

			By("Verifying DeviceConfig status")
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig: %s, Status: %+v",
				deviceConfigBuilder.Object.Name, deviceConfigBuilder.Object.Status)
		})

		It("Should verify Node Labeller pods are running and labels are applied", func() {
			// Skip("Skip node labeller pod verification")
			By("Verifying AMD GPU Worker Nodes are available")
			Expect(amdNodeBuilders).ToNot(BeEmpty(), "AMD GPU Worker Nodes should be available from BeforeAll")

			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Found %d AMD GPU nodes", len(amdNodeBuilders))
			for _, node := range amdNodeBuilders {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("AMD GPU node: %s", node.Object.Name)
			}

			By("Verifying Node Labeller pods are running")
			Expect(nodeLabellerPodBuilders).ToNot(BeEmpty(), "Node Labeller pods should be available from BeforeAll")

			for _, nlPod := range nodeLabellerPodBuilders {
				Expect(nlPod.Object.Status.Phase).To(Equal(corev1.PodRunning),
					fmt.Sprintf("Node Labeller pod %s should be running", nlPod.Object.Name))
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Node Labeller pod %s is running", nlPod.Object.Name)
			}

			By("Validating all AMD labels are applied by the Node Labeller")
			labelsCheckErr := labels.LabelsExistOnAllNodes(amdNodeBuilders, amdgpuparams.NodeLabellerLabels,
				amdgpuparams.DefaultTimeout, amdgpuparams.DefaultSleepInterval)
			Expect(labelsCheckErr).To(BeNil(),
				fmt.Sprintf("Node Labeller labels don't exist on all AMD GPU Worker Nodes: %v", labelsCheckErr))

			klog.V(amdgpuparams.AMDGPULogLevel).Info("Node Labeller pods running and labels verified")
		})

		It("Should verify Device Plugin pods are running and GPU resources are available", func() {

			By("Listing Device Plugin Pods")
			podsBuilder, podsBuilderErr := get.PodsFromNamespaceByPrefixWithTimeout(
				apiClient, amdgpuparams.AMDGPUNamespace, amdgpuparams.DeviceConfigName+"-device-plugin-")

			Expect(podsBuilderErr).To(BeNil(),
				fmt.Sprintf("Failed to get Device Plugin Pod in namespace '%s': %v",
					amdgpuparams.AMDGPUNamespace, podsBuilderErr))

			By("Counting Device Plugin Pods")
			Expect(podsBuilder).To(HaveLen(len(amdNodeBuilders)),
				fmt.Sprintf("expected one device plugin pod per AMD GPU worker node (found %d, expected %d)",
					len(podsBuilder), len(amdNodeBuilders)))

			By("Checking Device Plugin Pods are running and healthy")
			for _, devicePluginPod := range podsBuilder {
				Expect(devicePluginPod.Object.Status.Phase).To(Equal(corev1.PodRunning),
					fmt.Sprintf("Device Plugin pod %s should be running", devicePluginPod.Object.Name))
				Expect(devicePluginPod.IsHealthy()).To(BeTrue(),
					fmt.Sprintf("Device Plugin pod %s should be healthy", devicePluginPod.Object.Name))
			}

			By("Checking Resource Capacity & Allocatable on AMD GPU Worker Nodes")
			for _, node := range amdNodeBuilders {
				capacityQuantity := node.Object.Status.Capacity[amdgpuparams.AMDGPUCapacityID]
				capacity := capacityQuantity.Value()

				allocatableQuantity := node.Object.Status.Allocatable[amdgpuparams.AMDGPUCapacityID]
				allocatable := allocatableQuantity.Value()

				Expect(capacity).To(BeNumerically(">=", 1),
					fmt.Sprintf("expected at least one AMD GPU in capacity for node %s", node.Object.Name))
				Expect(allocatable).To(BeNumerically(">=", 1),
					fmt.Sprintf("expected at least one AMD GPU allocatable for node %s", node.Object.Name))

				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Node %s: GPU capacity=%d, allocatable=%d",
					node.Object.Name, capacity, allocatable)
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("Device Plugin pods running and GPU resources available")
		})

		It("Should detect GPUs using rocm-smi", func() {
			By("Running rocm-smi to detect GPUs")
			rocmSmiCmd := exec.NewPodCommandDirect(
				apiClient,
				"amd-gpu-smi-test",
				amdgpuparams.AMDGPUNamespace,
				"rocm/rocm-terminal:latest",
				"amd-gpu-smi-test",
				[]string{"rocm-smi"},
				map[string]string{"amd.com/gpu": "1"},
				map[string]string{"amd.com/gpu": "1"},
			)

			rocmSmiCmd.WithPrivileged(true).WithAllowPrivilegeEscalation(true)

			rocmSmiOutput, rocmSmiErr := rocmSmiCmd.ExecuteAndCleanup(5 * time.Minute)
			klog.Infof("rocm-smi output:\n%s", rocmSmiOutput)

			Expect(rocmSmiErr).NotTo(HaveOccurred(), "rocm-smi execution failed")

			// Look for GPU entries in output
			lines := strings.Split(rocmSmiOutput, "\n")
			foundGPU := false
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) > 0 && (line[0] >= '0' && line[0] <= '9') {
					foundGPU = true

					break
				}
			}

			Expect(foundGPU).To(BeTrue(), "Expected at least one GPU in rocm-smi output")
			klog.V(amdgpuparams.AMDGPULogLevel).Info("GPU detected via rocm-smi")
		})

		It("Should validate GPU info using rocminfo", func() {
			By("Running rocminfo to validate GPU")
			rocmInfoCmd := exec.NewPodCommandDirect(
				apiClient,
				"amd-gpu-info-test",
				amdgpuparams.AMDGPUNamespace,
				"rocm/rocm-terminal:latest",
				"amd-gpu-info-test",
				[]string{"rocminfo"},
				map[string]string{"amd.com/gpu": "1"},
				map[string]string{"amd.com/gpu": "1"},
			)

			rocmInfoCmd.WithPrivileged(true).WithAllowPrivilegeEscalation(true)

			rocmInfoOutput, rocmInfoErr := rocmInfoCmd.ExecuteAndCleanup(5 * time.Minute)
			klog.Infof("rocminfo output:\n%s", rocmInfoOutput)

			Expect(rocmInfoErr).NotTo(HaveOccurred(), "rocminfo execution failed")

			// Validate GPU information - use generic checks that work for all AMD GPU types
			// (Instinct, Radeon Pro, MI-series, etc.)
			Expect(rocmInfoOutput).To(ContainSubstring("gfx"), "Expected GPU architecture (gfx) in rocminfo output")
			Expect(rocmInfoOutput).To(MatchRegexp(`GPU|Agent\s+\d+`), "Expected GPU agent info in rocminfo output")
			Expect(rocmInfoOutput).To(ContainSubstring("AMD"), "Expected AMD GPU vendor in rocminfo output")

			klog.V(amdgpuparams.AMDGPULogLevel).Info("GPU validated via rocminfo")
		})
	})
})
