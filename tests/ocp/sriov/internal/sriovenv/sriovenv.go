// Package sriovenv provides SR-IOV test environment helpers for OCP tests.
// This package follows the CNF sriovenv pattern and uses eco-goinfra directly.
package sriovenv

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultMcpLabel is the default MachineConfigPool label for worker nodes.
	DefaultMcpLabel = "machineconfiguration.openshift.io/role=worker"
)

// ============================================================================
// Operator & Cluster Stability
// ============================================================================

// CheckSriovOperatorStatus checks if SR-IOV operator is running and healthy.
func CheckSriovOperatorStatus() error {
	return sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
}

// WaitForSriovPolicyReady waits for SR-IOV policy to be ready and MCP stable.
func WaitForSriovPolicyReady(timeout time.Duration) error {
	return WaitForSriovAndMCPStable(timeout, tsparams.MCPStableInterval)
}

// WaitForSriovAndMCPStable waits for SRIOV node states and MCP to be stable.
func WaitForSriovAndMCPStable(timeout, interval time.Duration) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	return wait.PollUntilContextTimeout(context.Background(), interval, timeout, false,
		func(ctx context.Context) (bool, error) {
			// Check SR-IOV node states synced
			nodeStates, err := sriov.ListNetworkNodeState(APIClient, sriovOpNs, client.ListOptions{})
			if err != nil || len(nodeStates) == 0 {
				return false, nil
			}

			for _, ns := range nodeStates {
				if ns.Objects != nil && ns.Objects.Status.SyncStatus != "Succeeded" {
					return false, nil
				}
			}

			// Check MCP stability
			mcpList, err := mco.ListMCP(APIClient)
			if err != nil {
				return false, nil
			}

			for _, mcp := range mcpList {
				if !mcp.IsInCondition(mcv1.MachineConfigPoolUpdated) {
					return false, nil
				}
			}

			return true, nil
		})
}

// ============================================================================
// Network Creation (using eco-goinfra directly)
// ============================================================================

// NetworkOption is a function that modifies a NetworkBuilder.
type NetworkOption func(*sriov.NetworkBuilder) *sriov.NetworkBuilder

// WithSpoof sets spoof checking on the network.
func WithSpoof(enabled bool) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithSpoof(enabled)
	}
}

// WithTrust sets trust flag on the network.
func WithTrust(enabled bool) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithTrustFlag(enabled)
	}
}

// WithVLAN sets VLAN ID on the network.
func WithVLAN(vlanID uint16) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithVLAN(vlanID)
	}
}

// WithVlanQoS sets VLAN QoS on the network.
func WithVlanQoS(qos uint16) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithVlanQoS(qos)
	}
}

// WithMinTxRate sets minimum TX rate on the network.
func WithMinTxRate(rate uint16) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithMinTxRate(rate)
	}
}

// WithMaxTxRate sets maximum TX rate on the network.
func WithMaxTxRate(rate uint16) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithMaxTxRate(rate)
	}
}

// WithLinkState sets link state on the network.
func WithLinkState(state string) NetworkOption {
	return func(nb *sriov.NetworkBuilder) *sriov.NetworkBuilder {
		return nb.WithLinkState(state)
	}
}

// CreateSriovNetwork creates a SRIOV network and waits for NAD.
func CreateSriovNetwork(name, resourceName, targetNs string, opts ...NetworkOption) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	networkBuilder := sriov.NewNetworkBuilder(APIClient, name, sriovOpNs, targetNs, resourceName).
		WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")

	for _, opt := range opts {
		networkBuilder = opt(networkBuilder)
	}

	if _, err := networkBuilder.Create(); err != nil {
		return fmt.Errorf("failed to create network %q: %w", name, err)
	}

	return WaitForNADCreation(name, targetNs, tsparams.NADTimeout)
}

// WaitForNADCreation waits for NetworkAttachmentDefinition to be created.
func WaitForNADCreation(name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), tsparams.PollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(APIClient, name, namespace)

			return err == nil, nil
		})
}

// RemoveSriovNetwork removes a SRIOV network by name.
func RemoveSriovNetwork(name string, timeout time.Duration) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	network, err := sriov.PullNetwork(APIClient, name, sriovOpNs)
	if err != nil {
		return nil // Already deleted
	}

	return network.DeleteAndWait(timeout)
}

// ============================================================================
// Policy Management
// ============================================================================

// RemoveSriovPolicy removes a SRIOV policy by name and waits for deletion.
func RemoveSriovPolicy(name string, timeout time.Duration) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	policy, err := sriov.PullPolicy(APIClient, name, sriovOpNs)
	if err != nil {
		return nil // Already deleted
	}

	if err := policy.Delete(); err != nil {
		return fmt.Errorf("failed to delete policy %q: %w", name, err)
	}

	// Wait for policy to be deleted
	return wait.PollUntilContextTimeout(context.Background(), tsparams.PollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := sriov.PullPolicy(APIClient, name, sriovOpNs)

			return pullErr != nil, nil // Policy is gone when pull fails
		})
}

// InitVF initializes VF for the given device using netdevice driver.
// Note: This function creates a policy on the first node where the device is found
// and returns immediately. It does not create policies on multiple nodes.
// This is suitable for single-node testing or when only one node needs the VF.
func InitVF(name, deviceID, interfaceName, vendor string, vfNum int, workerNodes []*nodes.Builder) (bool, error) {
	return initVFWithDevType(name, deviceID, interfaceName, vendor, "netdevice", vfNum, workerNodes)
}

// InitDpdkVF initializes DPDK VF for the given device using vfio-pci driver.
// Note: This function creates a policy on the first node where the device is found
// and returns immediately. It does not create policies on multiple nodes.
// This is suitable for single-node testing or when only one node needs the VF.
func InitDpdkVF(name, deviceID, interfaceName, vendor string, vfNum int, workerNodes []*nodes.Builder) (bool, error) {
	return initVFWithDevType(name, deviceID, interfaceName, vendor, "vfio-pci", vfNum, workerNodes)
}

// initVFWithDevType creates an SR-IOV policy for the specified device type.
// It iterates through worker nodes and creates a policy on the first node where
// the device is successfully discovered. Returns (true, nil) on success.
func initVFWithDevType(name, deviceID, interfaceName, vendor, devType string, vfNum int,
	workerNodes []*nodes.Builder) (bool, error) {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	// Cleanup existing policy
	_ = RemoveSriovPolicy(name, tsparams.NamespaceTimeout)

	for _, node := range workerNodes {
		nodeName := node.Definition.Name

		// Try to discover the actual interface name
		actualInterface := interfaceName

		// Only discover interface if not explicitly provided
		// This allows users to specify exact interface for multi-port NICs
		if interfaceName == "" && vendor != "" && deviceID != "" {
			if discovered, err := discoverInterfaceName(nodeName, vendor, deviceID); err == nil {
				actualInterface = discovered
			}
		}

		pfSelector := fmt.Sprintf("%s#0-%d", actualInterface, vfNum-1)

		policy := sriov.NewPolicyBuilder(APIClient, name, sriovOpNs, name, vfNum,
			[]string{pfSelector}, map[string]string{"kubernetes.io/hostname": nodeName}).
			WithDevType(devType)

		if vendor != "" {
			policy.Definition.Spec.NicSelector.Vendor = vendor
		}

		if deviceID != "" {
			policy.Definition.Spec.NicSelector.DeviceID = deviceID
		}

		if _, err := policy.Create(); err != nil {
			klog.V(90).Infof("Failed to create policy on node %q: %v", nodeName, err)

			continue
		}

		if err := WaitForSriovPolicyReady(tsparams.PolicyApplicationTimeout); err != nil {
			klog.V(90).Infof("Policy not ready on node %q: %v", nodeName, err)

			_ = policy.Delete()

			continue
		}

		klog.V(90).Infof("Successfully created policy %q on node %q", name, nodeName)

		return true, nil
	}

	return false, fmt.Errorf("failed to create policy %q on any node", name)
}

// discoverInterfaceName discovers the actual interface name on a node by matching Vendor and DeviceID.
func discoverInterfaceName(nodeName, vendor, deviceID string) (string, error) {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	nodeState := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, sriovOpNs)
	if err := nodeState.Discover(); err != nil {
		return "", err
	}

	if nodeState.Objects == nil || nodeState.Objects.Status.Interfaces == nil {
		return "", fmt.Errorf("no interfaces found on node %q", nodeName)
	}

	for _, iface := range nodeState.Objects.Status.Interfaces {
		if iface.Vendor == vendor && iface.DeviceID == deviceID {
			return iface.Name, nil
		}
	}

	return "", fmt.Errorf("no interface found matching vendor %q deviceID %q", vendor, deviceID)
}

// UpdateSriovPolicyMTU updates the MTU of an existing SR-IOV policy.
// PolicyBuilder does not have Update method, so we delete and recreate.
func UpdateSriovPolicyMTU(policyName string, mtuValue int) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	policy, err := sriov.PullPolicy(APIClient, policyName, sriovOpNs)
	if err != nil {
		return err
	}

	// Store the current spec before deletion
	spec := policy.Definition.Spec.DeepCopy()
	spec.Mtu = mtuValue

	// Delete the existing policy
	if err = policy.Delete(); err != nil {
		return fmt.Errorf("failed to delete policy %q for MTU update: %w", policyName, err)
	}

	// Recreate with updated MTU, preserving all original settings
	newPolicy := sriov.NewPolicyBuilder(
		APIClient,
		policyName,
		sriovOpNs,
		spec.ResourceName,
		spec.NumVfs,
		spec.NicSelector.PfNames,
		spec.NodeSelector,
	).WithMTU(mtuValue)

	// Preserve other policy settings from original spec
	if spec.DeviceType != "" {
		newPolicy = newPolicy.WithDevType(spec.DeviceType)
	}

	if spec.NicSelector.Vendor != "" {
		newPolicy.Definition.Spec.NicSelector.Vendor = spec.NicSelector.Vendor
	}

	if spec.NicSelector.DeviceID != "" {
		newPolicy.Definition.Spec.NicSelector.DeviceID = spec.NicSelector.DeviceID
	}

	if spec.Priority != 0 {
		newPolicy.Definition.Spec.Priority = spec.Priority
	}

	_, err = newPolicy.Create()
	if err != nil {
		return fmt.Errorf("failed to recreate policy %q with new MTU: %w", policyName, err)
	}

	return nil
}

// ============================================================================
// Pod Creation (following CNF pattern)
// ============================================================================

// CreateTestPod creates a test pod with SRIOV network.
func CreateTestPod(name, namespace, networkName, ip, mac string) (*pod.Builder, error) {
	secNetwork := pod.StaticIPAnnotationWithMacAddress(networkName, []string{ip}, mac)

	podBuilder := pod.NewBuilder(APIClient, name, namespace, SriovOcpConfig.OcpSriovTestContainer).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork)

	createdPod, err := podBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create pod %q: %w", name, err)
	}

	if err := createdPod.WaitUntilReady(tsparams.PodReadyTimeout); err != nil {
		return createdPod, fmt.Errorf("pod %q not ready: %w", name, err)
	}

	return createdPod, nil
}

// CreateDpdkTestPod creates a DPDK test pod with SR-IOV network.
func CreateDpdkTestPod(name, namespace, networkName string) (*pod.Builder, error) {
	secNetwork := pod.StaticIPAnnotationWithMacAddress(networkName,
		[]string{tsparams.TestPodClientIP}, tsparams.TestPodClientMAC)

	podBuilder := pod.NewBuilder(APIClient, name, namespace, SriovOcpConfig.OcpSriovTestContainer).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		WithLabel("name", "sriov-dpdk")

	createdPod, err := podBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create DPDK pod %q: %w", name, err)
	}

	if err := createdPod.WaitUntilReady(tsparams.PodReadyTimeout); err != nil {
		return createdPod, fmt.Errorf("DPDK pod %q not ready: %w", name, err)
	}

	return createdPod, nil
}

// DeleteDpdkTestPod deletes a DPDK test pod.
func DeleteDpdkTestPod(name, namespace string, timeout time.Duration) error {
	podBuilder, err := pod.Pull(APIClient, name, namespace)
	if err != nil {
		return nil // Already deleted
	}

	_, err = podBuilder.DeleteAndWait(timeout)

	return err
}

// WaitForPodWithLabelReady waits for a pod with specific label to be ready.
func WaitForPodWithLabelReady(namespace, labelSelector string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), tsparams.PollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			podList, err := pod.List(APIClient, namespace, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil || len(podList) == 0 {
				return false, nil
			}

			// Check if all pods are ready
			for _, podItem := range podList {
				if podItem.Object == nil {
					return false, nil
				}

				// Check pod phase
				if podItem.Object.Status.Phase != corev1.PodRunning {
					return false, nil
				}

				// Check container ready conditions
				allReady := true

				for _, containerStatus := range podItem.Object.Status.ContainerStatuses {
					if !containerStatus.Ready {
						allReady = false

						break
					}
				}

				if !allReady {
					return false, nil
				}
			}

			return true, nil
		})
}

// ============================================================================
// Interface Verification
// ============================================================================

// VerifyInterfaceReady verifies that a pod's network interface is ready.
func VerifyInterfaceReady(podObj *pod.Builder, interfaceName string) error {
	output, err := podObj.ExecCommand([]string{"ip", "link", "show", interfaceName})
	if err != nil {
		return fmt.Errorf("failed to check interface %q: %w", interfaceName, err)
	}

	if !strings.Contains(output.String(), "UP") {
		return fmt.Errorf("interface %q is not UP", interfaceName)
	}

	return nil
}

// CheckInterfaceCarrier checks if interface has carrier.
func CheckInterfaceCarrier(podObj *pod.Builder, interfaceName string) (bool, error) {
	output, err := podObj.ExecCommand([]string{"ip", "link", "show", interfaceName})
	if err != nil {
		return false, err
	}

	return !strings.Contains(output.String(), "NO-CARRIER"), nil
}

// ExtractPodInterfaceMAC extracts the MAC address from a pod's interface.
func ExtractPodInterfaceMAC(podObj *pod.Builder, interfaceName string) (string, error) {
	output, err := podObj.ExecCommand([]string{"ip", "link", "show", interfaceName})
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(output.String(), "\n") {
		if strings.Contains(line, "link/ether") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "link/ether" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("MAC not found for %q", interfaceName)
}

// ============================================================================
// Traffic & Connectivity Tests
// ============================================================================

// CheckVFStatusWithPassTraffic creates test pods and verifies connectivity.
func CheckVFStatusWithPassTraffic(networkName, interfaceName, namespace, description string,
	timeout time.Duration) error {
	klog.V(90).Infof("Checking VF status: %q (network: %q, ns: %q)", description, networkName, namespace)

	// Create client and server pods
	clientPod, err := CreateTestPod("client", namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return fmt.Errorf("failed to create client pod: %w", err)
	}

	defer func() {
		if clientPod != nil {
			_, _ = clientPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	serverPod, err := CreateTestPod("server", namespace, networkName,
		tsparams.TestPodServerIP, tsparams.TestPodServerMAC)
	if err != nil {
		return fmt.Errorf("failed to create server pod: %w", err)
	}

	defer func() {
		if serverPod != nil {
			_, _ = serverPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	// Verify interfaces
	if err := VerifyInterfaceReady(clientPod, "net1"); err != nil {
		return fmt.Errorf("client interface not ready: %w", err)
	}

	if err := VerifyInterfaceReady(serverPod, "net1"); err != nil {
		return fmt.Errorf("server interface not ready: %w", err)
	}

	// Check carrier
	hasCarrier, err := CheckInterfaceCarrier(clientPod, "net1")
	if err != nil {
		return fmt.Errorf("failed to check carrier: %w", err)
	}

	if !hasCarrier {
		return fmt.Errorf("NO-CARRIER: no physical connection")
	}

	// Verify spoof checking if in description
	if strings.Contains(description, "spoof checking") {
		expectedState := "on"
		if strings.Contains(description, "off") {
			expectedState = "off"
		}

		if err := verifySpoofCheck(clientPod, interfaceName, expectedState); err != nil {
			return fmt.Errorf("spoof check verification failed: %w", err)
		}
	}

	// Test connectivity
	serverIP := strings.Split(tsparams.TestPodServerIP, "/")[0]

	output, err := clientPod.ExecCommand([]string{"ping", "-c", "3", serverIP})
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	if strings.Contains(output.String(), "100% packet loss") {
		return fmt.Errorf("ping failed with 100%% packet loss")
	}

	klog.V(90).Infof("VF status verification completed successfully: %q", description)

	return nil
}

// VerifyLinkStateConfiguration verifies link state configuration without requiring connectivity.
// It waits up to CarrierWaitTimeout for carrier status to be established before returning.
func VerifyLinkStateConfiguration(networkName, namespace, description string,
	timeout time.Duration) (bool, error) {
	klog.V(90).Infof("Verifying link state: %q (network: %q, ns: %q)", description, networkName, namespace)

	testPod, err := CreateTestPod("linkstate-test", namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return false, fmt.Errorf("failed to create test pod: %w", err)
	}

	defer func() {
		if testPod != nil {
			_, _ = testPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	if err := VerifyInterfaceReady(testPod, "net1"); err != nil {
		return false, fmt.Errorf("interface not ready: %w", err)
	}

	// Wait for carrier status with retry - VF link state may take time to propagate
	var hasCarrier bool

	err = wait.PollUntilContextTimeout(context.Background(), tsparams.PollingInterval,
		tsparams.CarrierWaitTimeout, true, func(ctx context.Context) (bool, error) {
			carrier, checkErr := CheckInterfaceCarrier(testPod, "net1")
			if checkErr != nil {
				klog.V(90).Infof("Carrier check failed (will retry): %v", checkErr)

				return false, nil
			}

			hasCarrier = carrier
			if !carrier {
				klog.V(90).Infof("Interface has NO-CARRIER, waiting for link...")

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		// Timeout waiting for carrier - return false but no error (let test decide to skip)
		klog.V(90).Infof("Carrier wait timed out after %v", tsparams.CarrierWaitTimeout)

		return false, nil
	}

	return hasCarrier, nil
}

// ============================================================================
// Spoof Check Verification
// ============================================================================

func verifySpoofCheck(clientPod *pod.Builder, interfaceName, expectedState string) error {
	// Get node name and MAC
	refreshedPod, err := pod.Pull(APIClient, clientPod.Definition.Name, clientPod.Definition.Namespace)
	if err != nil {
		return fmt.Errorf("failed to refresh pod: %w", err)
	}

	nodeName := refreshedPod.Definition.Spec.NodeName
	if nodeName == "" {
		return fmt.Errorf("pod node name is empty")
	}

	mac, err := ExtractPodInterfaceMAC(clientPod, "net1")
	if err != nil {
		return fmt.Errorf("failed to extract MAC: %w", err)
	}

	// Execute on node via debug pod
	output, err := executeOnNode(nodeName, refreshedPod.Definition.Namespace,
		[]string{"ip", "link", "show", interfaceName})
	if err != nil {
		return fmt.Errorf("failed to execute on node: %w", err)
	}

	// Find line with MAC and check spoof state
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, mac) {
			if strings.Contains(line, fmt.Sprintf("spoof checking %s", expectedState)) ||
				strings.Contains(line, fmt.Sprintf("spoofchk %s", expectedState)) {
				klog.V(90).Infof("Spoof check verified: %s for MAC %s", expectedState, mac)

				return nil
			}
		}
	}

	return fmt.Errorf("spoof check %s not found for MAC %s", expectedState, mac)
}

func executeOnNode(nodeName, namespace string, cmd []string) (string, error) {
	debugPodName := "sriov-debug-" + strings.ReplaceAll(nodeName, ".", "-")
	if len(debugPodName) > 63 {
		debugPodName = debugPodName[:63]
	}

	// Cleanup any existing debug pod
	if existing, _ := pod.Pull(APIClient, debugPodName, namespace); existing != nil {
		_, _ = existing.DeleteAndWait(tsparams.DebugPodCleanupTimeout)
	}

	debugPod := pod.NewBuilder(APIClient, debugPodName, namespace, SriovOcpConfig.OcpSriovTestContainer).
		WithPrivilegedFlag().
		WithHostNetwork().
		WithHostPid(true).
		WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName}).
		WithRestartPolicy(corev1.RestartPolicyNever)

	createdPod, err := debugPod.Create()
	if err != nil {
		return "", fmt.Errorf("failed to create debug pod: %w", err)
	}

	defer func() {
		_, _ = createdPod.DeleteAndWait(tsparams.DebugPodCleanupTimeout)
	}()

	if err := createdPod.WaitUntilReady(tsparams.PodReadyTimeout); err != nil {
		return "", fmt.Errorf("debug pod not ready: %w", err)
	}

	nsenterCmd := append([]string{"nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "--"}, cmd...)

	output, err := createdPod.ExecCommand(nsenterCmd)
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	return output.String(), nil
}

// ============================================================================
// PCI Address
// ============================================================================

// GetPciAddress gets the PCI address for a pod from network status annotation.
func GetPciAddress(namespace, podName, networkName string) (string, error) {
	podObj, err := pod.Pull(APIClient, podName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull pod: %w", err)
	}

	annotation := podObj.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if annotation == "" {
		return "", fmt.Errorf("no network status annotation")
	}

	var status []struct {
		Name       string `json:"name"`
		DeviceInfo struct {
			Pci struct {
				PciAddress string `json:"pci-address"`
			} `json:"pci"`
		} `json:"device-info"`
	}

	if err := json.Unmarshal([]byte(annotation), &status); err != nil {
		return "", fmt.Errorf("failed to parse network status: %w", err)
	}

	for _, networkStatus := range status {
		netName := networkStatus.Name
		if idx := strings.LastIndex(netName, "/"); idx >= 0 {
			netName = netName[idx+1:]
		}

		if netName == networkName && networkStatus.DeviceInfo.Pci.PciAddress != "" {
			return networkStatus.DeviceInfo.Pci.PciAddress, nil
		}
	}

	return "", fmt.Errorf("PCI address not found for network %q", networkName)
}

// ============================================================================
// Cleanup Functions
// ============================================================================

// CleanupLeftoverResources cleans up leftover test resources.
// This is a best-effort cleanup that logs errors but continues cleaning.
func CleanupLeftoverResources() error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	klog.V(90).Info("Cleaning up leftover test resources")

	// Cleanup test namespaces (prefixed with "e2e-")
	namespaces, err := namespace.List(APIClient, metav1.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Warning: failed to list namespaces: %v", err)
	}

	for _, ns := range namespaces {
		if strings.HasPrefix(ns.Definition.Name, "e2e-") {
			klog.V(90).Infof("Removing leftover namespace %q", ns.Definition.Name)

			if delErr := ns.DeleteAndWait(tsparams.CleanupTimeout); delErr != nil {
				klog.V(90).Infof("Warning: failed to delete namespace %q: %v", ns.Definition.Name, delErr)
			}
		}
	}

	// Cleanup test networks matching naming conventions:
	// - "^\d{5}-" matches networks prefixed with test case IDs (e.g., "25959-cx7anl244")
	// - "\w+dpdknet$" matches DPDK networks suffixed with "dpdknet" (e.g., "cx7anl244dpdknet")
	networks, err := sriov.List(APIClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Warning: failed to list networks: %v", err)
	}

	pattern := regexp.MustCompile(`^\d{5}-|\w+dpdknet$`)

	for _, net := range networks {
		if pattern.MatchString(net.Definition.Name) {
			klog.V(90).Infof("Removing leftover network %q", net.Definition.Name)

			if delErr := net.Delete(); delErr != nil {
				klog.V(90).Infof("Warning: failed to delete network %q: %v", net.Definition.Name, delErr)
			}
		}
	}

	// Cleanup test policies matching configured device names
	policies, err := sriov.ListPolicy(APIClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Warning: failed to list policies: %v", err)
	}

	deviceNames, err := getTestDeviceNames()
	if err != nil {
		klog.V(90).Infof("Warning: failed to get device names: %v", err)
	}

	for _, policy := range policies {
		for _, name := range deviceNames {
			if policy.Definition.Name == name || strings.HasPrefix(policy.Definition.Name, name) {
				klog.V(90).Infof("Removing leftover policy %q", policy.Definition.Name)

				if delErr := policy.Delete(); delErr != nil {
					klog.V(90).Infof("Warning: failed to delete policy %q: %v", policy.Definition.Name, delErr)
				}

				break
			}
		}
	}

	klog.V(90).Info("Cleanup completed")

	return nil
}

func getTestDeviceNames() ([]string, error) {
	configs, err := SriovOcpConfig.GetSriovDevices()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(configs))

	for _, c := range configs {
		names = append(names, c.Name)
	}

	return names, nil
}
