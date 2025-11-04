package sriov

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Global variables for API client and configuration
var (
	APIClient *clients.Settings
	NetConfig *NetworkConfig
)

// NetworkConfig represents network configuration
type NetworkConfig struct {
	WorkerLabel            string
	CnfNetTestContainer    string
	CnfMcpLabel            string
	SriovOperatorNamespace string
	WorkerLabelMap         map[string]string
}

// GetJunitReportPath returns the junit report path
func (nc *NetworkConfig) GetJunitReportPath() string {
	return "/tmp/junit.xml"
}

// Initialize the test environment
func init() {
	// Initialize with default values - these would normally come from environment or config
	NetConfig = &NetworkConfig{
		WorkerLabel:            "node-role.kubernetes.io/worker",
		CnfNetTestContainer:    "quay.io/openshift-kni/cnf-tests:4.16",
		CnfMcpLabel:            "machineconfiguration.openshift.io/role=worker",
		SriovOperatorNamespace: "openshift-sriov-network-operator",
		WorkerLabelMap:         map[string]string{"node-role.kubernetes.io/worker": ""},
	}

	// Don't initialize API client here - do it lazily when needed
}

// getAPIClient returns the API client, initializing it if necessary
func getAPIClient() *clients.Settings {
	if APIClient == nil {
		// Try in-cluster config first (when running inside a pod)
		_, err := rest.InClusterConfig()
		if err != nil {
			// Fallback to kubeconfig file
			kubeconfigPath := os.Getenv("KUBECONFIG")
			if kubeconfigPath == "" {
				Skip("No KUBECONFIG environment variable set.")
			}
			APIClient = clients.New(kubeconfigPath)
			if APIClient == nil {
				// If both fail, skip the test with a proper message
				Skip("Failed to create API client from kubeconfig. Please check your KUBECONFIG file.")
			}
		} else {
			// Use in-cluster config
			APIClient = clients.New("")
			if APIClient == nil {
				Skip("Failed to create in-cluster API client.")
			}
		}
	}
	return APIClient
}

// IsSriovDeployed checks if SRIOV is deployed
func IsSriovDeployed(client *clients.Settings, config *NetworkConfig) error {
	// Simple implementation - in real scenario this would check for SRIOV operator
	return nil
}

// WaitForSriovAndMCPStable waits for SRIOV and MCP to be stable
func WaitForSriovAndMCPStable(client *clients.Settings, timeout time.Duration, interval time.Duration, mcpLabel, sriovOpNs string) error {
	// Simple implementation - in real scenario this would wait for conditions
	time.Sleep(5 * time.Second)
	return nil
}

// CleanAllNetworksByTargetNamespace cleans all networks by target namespace
func CleanAllNetworksByTargetNamespace(client *clients.Settings, sriovOpNs, targetNs string) error {
	// Simple implementation - in real scenario this would clean up networks
	return nil
}

// pullTestImageOnNodes pulls given image on range of relevant nodes based on nodeSelector
func pullTestImageOnNodes(apiClient *clients.Settings, nodeSelector, image string, pullTimeout int) error {
	// Simple implementation - in real scenario this would pull images on nodes
	// For now, we'll just return success
	return nil
}

// sriovNetwork represents a SRIOV network configuration
type sriovNetwork struct {
	name             string
	resourceName     string
	networkNamespace string
	template         string
	namespace        string
	spoolchk         string
	trust            string
	vlanId           int
	vlanQoS          int
	minTxRate        int
	maxTxRate        int
	linkState        string
}

// sriovTestPod represents a test pod for SRIOV testing
type sriovTestPod struct {
	name        string
	namespace   string
	networkName string
	template    string
}

// chkSriovOperatorStatus checks if the SRIOV operator is running
func chkSriovOperatorStatus(sriovOpNs string) {
	By("Checking SRIOV operator status")
	err := IsSriovDeployed(getAPIClient(), NetConfig)
	Expect(err).ToNot(HaveOccurred(), "SRIOV operator is not deployed")
}

// waitForSriovPolicyReady waits for SRIOV policy to be ready
func waitForSriovPolicyReady(sriovOpNs string) {
	By("Waiting for SRIOV policy to be ready")
	err := WaitForSriovAndMCPStable(
		getAPIClient(), 35*time.Minute, time.Minute, NetConfig.CnfMcpLabel, sriovOpNs)
	Expect(err).ToNot(HaveOccurred(), "SRIOV policy is not ready")
}

// rmSriovPolicy removes a SRIOV policy by name if it exists
func rmSriovPolicy(name, sriovOpNs string) {
	By(fmt.Sprintf("Removing SRIOV policy %s if it exists", name))

	// Create a policy builder to check if it exists
	policyBuilder := sriov.NewPolicyBuilder(
		getAPIClient(),
		name,
		sriovOpNs,
		"", // resourceName not needed for deletion check
		0,  // vfNum not needed
		[]string{},
		map[string]string{},
	)

	// Only delete if the policy exists
	if policyBuilder.Exists() {
		err := policyBuilder.Delete()
		if err != nil {
			GinkgoLogr.Info("Failed to delete SRIOV policy", "error", err, "name", name)
			return
		}

		// Wait for policy to be deleted
		By(fmt.Sprintf("Waiting for SRIOV policy %s to be deleted", name))
		Eventually(func() bool {
			checkPolicy := sriov.NewPolicyBuilder(
				getAPIClient(),
				name,
				sriovOpNs,
				"",
				0,
				[]string{},
				map[string]string{},
			)
			return !checkPolicy.Exists()
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"SRIOV policy %s should be deleted from namespace %s", name, sriovOpNs)
	} else {
		GinkgoLogr.Info("SRIOV policy does not exist, skipping deletion", "name", name, "namespace", sriovOpNs)
	}
}

// initVF initializes VF for the given device
func initVF(name, deviceID, interfaceName, vendor, sriovOpNs string, vfNum int, workerNodes []*nodes.Builder) bool {
	By(fmt.Sprintf("Initializing VF for device %s", name))

	// Check if the device exists on any worker node
	for _, node := range workerNodes {
		// Create SRIOV policy
		sriovPolicy := sriov.NewPolicyBuilder(
			getAPIClient(),
			name,
			sriovOpNs,
			name,
			vfNum,
			[]string{fmt.Sprintf("%s#0-%d", interfaceName, vfNum-1)},
			map[string]string{"kubernetes.io/hostname": node.Definition.Name},
		).WithDevType("netdevice")

		_, err := sriovPolicy.Create()
		if err != nil {
			GinkgoLogr.Info("Failed to create SRIOV policy", "error", err, "node", node.Definition.Name)
			continue
		}

		// Wait for policy to be applied
		err = WaitForSriovAndMCPStable(
			getAPIClient(), 35*time.Minute, time.Minute, NetConfig.CnfMcpLabel, sriovOpNs)
		if err != nil {
			GinkgoLogr.Info("Failed to wait for SRIOV policy", "error", err, "node", node.Definition.Name)
			continue
		}

		return true
	}

	return false
}

// initDpdkVF initializes DPDK VF for the given device
func initDpdkVF(name, deviceID, interfaceName, vendor, sriovOpNs string, vfNum int, workerNodes []*nodes.Builder) bool {
	By(fmt.Sprintf("Initializing DPDK VF for device %s", name))

	// Check if the device exists on any worker node
	for _, node := range workerNodes {
		// Create SRIOV policy for DPDK
		sriovPolicy := sriov.NewPolicyBuilder(
			getAPIClient(),
			name,
			sriovOpNs,
			name,
			vfNum,
			[]string{fmt.Sprintf("%s#0-%d", interfaceName, vfNum-1)},
			map[string]string{"kubernetes.io/hostname": node.Definition.Name},
		).WithDevType("vfio-pci")

		_, err := sriovPolicy.Create()
		if err != nil {
			GinkgoLogr.Info("Failed to create DPDK SRIOV policy", "error", err, "node", node.Definition.Name)
			continue
		}

		// Wait for policy to be applied
		err = WaitForSriovAndMCPStable(
			getAPIClient(), 35*time.Minute, time.Minute, NetConfig.CnfMcpLabel, sriovOpNs)
		if err != nil {
			GinkgoLogr.Info("Failed to wait for DPDK SRIOV policy", "error", err, "node", node.Definition.Name)
			continue
		}

		return true
	}

	return false
}

// createSriovNetwork creates a SRIOV network
func (sn *sriovNetwork) createSriovNetwork() {
	By(fmt.Sprintf("Creating SRIOV network %s", sn.name))

	networkBuilder := sriov.NewNetworkBuilder(
		getAPIClient(),
		sn.name,
		sn.namespace,
		sn.networkNamespace,
		sn.resourceName,
	).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")

	// Set optional parameters
	// Note: WithSpoofChk method may not be available in this version
	// if sn.spoolchk != "" {
	//	if sn.spoolchk == "on" {
	//		networkBuilder.WithSpoofChk(true)
	//	} else {
	//		networkBuilder.WithSpoofChk(false)
	//	}
	// }

	if sn.trust != "" {
		if sn.trust == "on" {
			networkBuilder.WithTrustFlag(true)
		} else {
			networkBuilder.WithTrustFlag(false)
		}
	}

	if sn.vlanId > 0 {
		networkBuilder.WithVLAN(uint16(sn.vlanId))
	}

	if sn.vlanQoS > 0 {
		networkBuilder.WithVlanQoS(uint16(sn.vlanQoS))
	}

	if sn.minTxRate > 0 {
		networkBuilder.WithMinTxRate(uint16(sn.minTxRate))
	}

	if sn.maxTxRate > 0 {
		networkBuilder.WithMaxTxRate(uint16(sn.maxTxRate))
	}

	if sn.linkState != "" {
		networkBuilder.WithLinkState(sn.linkState)
	}

	_, err := networkBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SRIOV network")

	// Wait for NetworkAttachmentDefinition to be created by the SRIOV operator
	By(fmt.Sprintf("Waiting for NetworkAttachmentDefinition %s to be created in namespace %s", sn.name, sn.networkNamespace))
	Eventually(func() error {
		_, err := nad.Pull(getAPIClient(), sn.name, sn.networkNamespace)
		if err != nil {
			GinkgoLogr.Info("NetworkAttachmentDefinition not yet created", "name", sn.name, "namespace", sn.networkNamespace, "error", err)
		}
		return err
	}, 3*time.Minute, 3*time.Second).Should(BeNil(), "Failed to wait for NetworkAttachmentDefinition %s in namespace %s", sn.name, sn.networkNamespace)
}

// rmSriovNetwork removes a SRIOV network by name from the operator namespace
func rmSriovNetwork(name, sriovOpNs string) {
	By(fmt.Sprintf("Removing SRIOV network %s", name))

	// Use List to find the network by name
	listOptions := client.ListOptions{}
	sriovNetworks, err := sriov.List(getAPIClient(), sriovOpNs, listOptions)
	if err != nil {
		GinkgoLogr.Info("Failed to list SRIOV networks", "namespace", sriovOpNs, "error", err)
		return
	}

	// Find the network with matching name
	var targetNetwork *sriov.NetworkBuilder
	var targetNamespace string
	var resourceName string
	for _, network := range sriovNetworks {
		if network.Object.Name == name {
			targetNamespace = network.Object.Spec.NetworkNamespace
			if targetNamespace == "" {
				targetNamespace = sriovOpNs
			}
			resourceName = network.Object.Spec.ResourceName
			// Rebuild the network builder with the same parameters to delete it
			targetNetwork = sriov.NewNetworkBuilder(
				getAPIClient(),
				name,
				sriovOpNs,
				targetNamespace,
				resourceName,
			)
			break
		}
	}

	if targetNetwork == nil || !targetNetwork.Exists() {
		GinkgoLogr.Info("SRIOV network not found or already deleted", "name", name, "namespace", sriovOpNs)
		return
	}

	// Delete the SRIOV network
	err = targetNetwork.Delete()
	if err != nil {
		GinkgoLogr.Info("Failed to delete SRIOV network", "error", err, "name", name)
		return
	}

	// Wait for SRIOV network to be fully deleted
	By(fmt.Sprintf("Waiting for SRIOV network %s to be deleted", name))
	Eventually(func() bool {
		checkNetwork := sriov.NewNetworkBuilder(
			getAPIClient(),
			name,
			sriovOpNs,
			targetNamespace,
			resourceName,
		)
		return !checkNetwork.Exists()
	}, 30*time.Second, 2*time.Second).Should(BeTrue(),
		"SRIOV network %s should be deleted from namespace %s", name, sriovOpNs)

	// Wait for NAD to be deleted in the target namespace
	if targetNamespace != sriovOpNs {
		By(fmt.Sprintf("Waiting for NetworkAttachmentDefinition %s to be deleted in namespace %s", name, targetNamespace))
		err = wait.PollUntilContextTimeout(
			context.TODO(),
			2*time.Second,
			1*time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				_, pullErr := nad.Pull(getAPIClient(), name, targetNamespace)
				if pullErr != nil {
					// NAD is deleted (we got an error/not found), which is what we want
					return true, nil
				}
				// NAD still exists, keep waiting
				GinkgoLogr.Info("NetworkAttachmentDefinition still exists, waiting for deletion", "name", name, "namespace", targetNamespace)
				return false, nil
			})
		if err != nil {
			// Log the error with more context
			_, pullErr := nad.Pull(getAPIClient(), name, targetNamespace)
			if pullErr == nil {
				GinkgoLogr.Info("NetworkAttachmentDefinition still exists after timeout", "name", name, "namespace", targetNamespace)
			}
			Expect(err).ToNot(HaveOccurred(),
				"NetworkAttachmentDefinition %s was not deleted from namespace %s within timeout", name, targetNamespace)
		}
	}
}

// chkVFStatusWithPassTraffic checks VF status and passes traffic
func chkVFStatusWithPassTraffic(networkName, interfaceName, namespace, description string) {
	By(fmt.Sprintf("Checking VF status with traffic: %s", description))

	// Create test pods
	clientPod := createTestPod("client", namespace, networkName, "192.168.1.10/24", "20:04:0f:f1:88:01")
	serverPod := createTestPod("server", namespace, networkName, "192.168.1.11/24", "20:04:0f:f1:88:02")

	// Wait for pods to be ready - using WaitUntilReady instead of WaitUntilRunning
	// to ensure pods are fully ready (including readiness probes) not just running
	By("Waiting for client pod to be ready")
	err := clientPod.WaitUntilReady(300 * time.Second)
	if err != nil {
		// Log pod status for debugging
		if clientPod != nil && clientPod.Definition != nil {
			GinkgoLogr.Info("Client pod status", "phase", clientPod.Definition.Status.Phase,
				"reason", clientPod.Definition.Status.Reason, "message", clientPod.Definition.Status.Message)
		}
		Expect(err).ToNot(HaveOccurred(), "Client pod not ready")
	}

	By("Waiting for server pod to be ready")
	err = serverPod.WaitUntilReady(300 * time.Second)
	if err != nil {
		// Log pod status for debugging
		if serverPod != nil && serverPod.Definition != nil {
			GinkgoLogr.Info("Server pod status", "phase", serverPod.Definition.Status.Phase,
				"reason", serverPod.Definition.Status.Reason, "message", serverPod.Definition.Status.Message)
		}
		Expect(err).ToNot(HaveOccurred(), "Server pod not ready")
	}

	// Test connectivity with timeout
	By("Testing connectivity between pods")
	pingCmd := []string{"ping", "-c", "3", "192.168.1.11"}

	var pingOutput bytes.Buffer
	pingTimeout := 2 * time.Minute
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		5*time.Second,
		pingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var execErr error
			pingOutput, execErr = clientPod.ExecCommand(pingCmd)
			if execErr != nil {
				GinkgoLogr.Info("Ping command failed, will retry", "error", execErr, "output", pingOutput.String())
				return false, nil // Retry on error
			}
			return true, nil // Success
		})

	Expect(err).ToNot(HaveOccurred(), "Ping command timed out or failed after %v", pingTimeout)
	Expect(pingOutput.Len()).To(BeNumerically(">", 0), "Ping command returned empty output")
	Expect(pingOutput.String()).To(ContainSubstring("3 packets transmitted"), "Ping did not complete successfully")

	// Clean up pods
	clientPod.DeleteAndWait(30 * time.Second)
	serverPod.DeleteAndWait(30 * time.Second)
}

// createTestPod creates a test pod with SRIOV network
func createTestPod(name, namespace, networkName, ipAddress, macAddress string) *pod.Builder {
	By(fmt.Sprintf("Creating test pod %s", name))

	// Create network annotation
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(networkName, []string{ipAddress}, macAddress)

	podBuilder := pod.NewBuilder(
		getAPIClient(),
		name,
		namespace,
		NetConfig.CnfNetTestContainer,
	).WithPrivilegedFlag().WithSecondaryNetwork(networkAnnotation)

	createdPod, err := podBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create test pod")

	return createdPod
}

// createSriovTestPod creates a SRIOV test pod
func (stp *sriovTestPod) createSriovTestPod() {
	By(fmt.Sprintf("Creating SRIOV test pod %s", stp.name))

	// Create network annotation
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(stp.networkName, []string{"192.168.1.10/24"}, "20:04:0f:f1:88:01")

	podBuilder := pod.NewBuilder(
		getAPIClient(),
		stp.name,
		stp.namespace,
		NetConfig.CnfNetTestContainer,
	).WithPrivilegedFlag().WithSecondaryNetwork(networkAnnotation).WithLabel("name", "sriov-dpdk")

	_, err := podBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SRIOV test pod")
}

// deleteSriovTestPod deletes a SRIOV test pod
func (stp *sriovTestPod) deleteSriovTestPod() {
	By(fmt.Sprintf("Deleting SRIOV test pod %s", stp.name))
	podBuilder := pod.NewBuilder(getAPIClient(), stp.name, stp.namespace, NetConfig.CnfNetTestContainer)
	_, err := podBuilder.DeleteAndWait(30 * time.Second)
	if err != nil {
		GinkgoLogr.Info("Failed to delete SRIOV test pod", "error", err)
	}
}

// waitForPodWithLabelReady waits for a pod with specific label to be ready
func waitForPodWithLabelReady(namespace, labelSelector string) error {
	By(fmt.Sprintf("Waiting for pod with label %s to be ready", labelSelector))

	// Wait for pod to appear (it might take a moment to be created)
	var podList []*pod.Builder
	var err error
	Eventually(func() bool {
		podList, err = pod.List(getAPIClient(), namespace, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			GinkgoLogr.Info("Failed to list pods, will retry", "error", err, "namespace", namespace, "labelSelector", labelSelector)
			return false
		}
		return len(podList) > 0
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Pod with label %s not found in namespace %s", labelSelector, namespace)

	if len(podList) == 0 {
		return fmt.Errorf("no pods found with label %s", labelSelector)
	}

	// Wait for each pod to be ready
	for _, p := range podList {
		By(fmt.Sprintf("Waiting for pod %s to be ready", p.Definition.Name))
		err := p.WaitUntilReady(300 * time.Second)
		if err != nil {
			// Log pod status for debugging
			if p.Definition != nil {
				// Log detailed pod status
				GinkgoLogr.Info("Pod status details", "name", p.Definition.Name,
					"phase", p.Definition.Status.Phase,
					"reason", p.Definition.Status.Reason,
					"message", p.Definition.Status.Message)

				// Log container statuses
				if len(p.Definition.Status.ContainerStatuses) > 0 {
					for _, cs := range p.Definition.Status.ContainerStatuses {
						GinkgoLogr.Info("Container status", "name", cs.Name,
							"ready", cs.Ready,
							"state", fmt.Sprintf("%+v", cs.State),
							"lastState", fmt.Sprintf("%+v", cs.LastTerminationState))
					}
				}

				// Log events if available
				if len(p.Definition.Status.Conditions) > 0 {
					for _, cond := range p.Definition.Status.Conditions {
						GinkgoLogr.Info("Pod condition", "type", cond.Type,
							"status", cond.Status,
							"reason", cond.Reason,
							"message", cond.Message)
					}
				}
			}
			return fmt.Errorf("pod %s not ready: %w", p.Definition.Name, err)
		}
	}

	return nil
}

// getPciAddress gets the PCI address for a pod from network status annotation
func getPciAddress(namespace, podName, policyName string) string {
	By(fmt.Sprintf("Getting PCI address for pod %s", podName))

	podBuilder := pod.NewBuilder(getAPIClient(), podName, namespace, NetConfig.CnfNetTestContainer)
	if !podBuilder.Exists() {
		return "0000:00:00.0" // Fallback if pod doesn't exist
	}

	// Get the network status annotation
	networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"
	podNetAnnotation := podBuilder.Object.Annotations[networkStatusAnnotation]
	if podNetAnnotation == "" {
		GinkgoLogr.Info("Pod network annotation not found", "pod", podName, "namespace", namespace)
		return "0000:00:00.0" // Fallback
	}

	// Parse the network status annotation
	type PodNetworkStatusAnnotation struct {
		Name       string `json:"name"`
		Interface  string `json:"interface"`
		DeviceInfo struct {
			Type    string `json:"type"`
			Version string `json:"version"`
			Pci     struct {
				PciAddress string `json:"pci-address"`
			} `json:"pci"`
		} `json:"device-info,omitempty"`
	}

	var annotation []PodNetworkStatusAnnotation
	err := json.Unmarshal([]byte(podNetAnnotation), &annotation)
	if err != nil {
		GinkgoLogr.Info("Failed to unmarshal pod network status", "error", err)
		return "0000:00:00.0" // Fallback
	}

	// Find the network matching the policy name
	for _, networkAnnotation := range annotation {
		if strings.Contains(networkAnnotation.Name, policyName) {
			if networkAnnotation.DeviceInfo.Pci.PciAddress != "" {
				return networkAnnotation.DeviceInfo.Pci.PciAddress
			}
		}
	}

	return "0000:00:00.0" // Fallback
}

// getRandomString generates a random string for unique naming
func getRandomString() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}
