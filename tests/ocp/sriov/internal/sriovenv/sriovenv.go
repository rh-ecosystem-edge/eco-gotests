// Package sriovenv provides SR-IOV test environment helpers for OCP tests.
// This package follows the CNF sriovenv pattern and uses eco-goinfra directly.
package sriovenv

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
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
// Uses the existing sriovoperator.WaitForSriovAndMCPStable function.
func WaitForSriovPolicyReady(timeout time.Duration) error {
	return sriovoperator.WaitForSriovAndMCPStable(
		APIClient,
		timeout,
		tsparams.MCPStableInterval,
		"worker",
		SriovOcpConfig.OcpSriovOperatorNamespace,
	)
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
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}

		return fmt.Errorf("failed to pull network %q: %w", name, err)
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
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}

		return fmt.Errorf("failed to pull policy %q: %w", name, err)
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
	if vfNum <= 0 {
		return false, fmt.Errorf("vfNum must be > 0, got %d", vfNum)
	}

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

		if actualInterface == "" {
			klog.V(90).Infof("No interface resolved on node %q (vendor=%q deviceID=%q)", nodeName, vendor, deviceID)

			continue
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
// Note: This causes brief service disruption as VFs are deallocated and reallocated.
func UpdateSriovPolicyMTU(policyName string, mtuValue int) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	policy, err := sriov.PullPolicy(APIClient, policyName, sriovOpNs)
	if err != nil {
		return err
	}

	// Store a deep copy of the entire spec to preserve all fields
	// (IsRdma, LinkType, ESwitchMode, ExternallyManaged, VdpaType, etc.)
	spec := policy.Definition.Spec.DeepCopy()
	spec.Mtu = mtuValue

	// Delete the existing policy
	if err = policy.Delete(); err != nil {
		return fmt.Errorf("failed to delete policy %q for MTU update: %w", policyName, err)
	}

	// Recreate with the full spec (preserving all original settings)
	newPolicy := sriov.NewPolicyBuilder(
		APIClient,
		policyName,
		sriovOpNs,
		spec.ResourceName,
		spec.NumVfs,
		spec.NicSelector.PfNames,
		spec.NodeSelector,
	)

	// Copy the entire spec to preserve all fields
	newPolicy.Definition.Spec = *spec

	_, err = newPolicy.Create()
	if err != nil {
		return fmt.Errorf("failed to recreate policy %q with new MTU: %w", policyName, err)
	}

	// Wait for policy to be applied and MCP to stabilize
	if err := WaitForSriovPolicyReady(tsparams.PolicyApplicationTimeout); err != nil {
		return fmt.Errorf("policy %q not ready after MTU update: %w", policyName, err)
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
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}

		return fmt.Errorf("failed to pull pod %q: %w", name, err)
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

	// Generate unique suffix for pod names to avoid collisions
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)

	// Create client and server pods with unique names
	clientPodName := fmt.Sprintf("client-%s", suffix)
	serverPodName := fmt.Sprintf("server-%s", suffix)

	clientPod, err := CreateTestPod(clientPodName, namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return fmt.Errorf("failed to create client pod: %w", err)
	}

	defer func() {
		if clientPod != nil {
			_, _ = clientPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	serverPod, err := CreateTestPod(serverPodName, namespace, networkName,
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
	if err := VerifyInterfaceReady(clientPod, interfaceName); err != nil {
		return fmt.Errorf("client interface not ready: %w", err)
	}

	if err := VerifyInterfaceReady(serverPod, interfaceName); err != nil {
		return fmt.Errorf("server interface not ready: %w", err)
	}

	// Check carrier
	hasCarrier, err := CheckInterfaceCarrier(clientPod, interfaceName)
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

	_, err = clientPod.ExecCommand([]string{"ping", "-c", "3", serverIP})
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	klog.V(90).Infof("VF status verification completed successfully: %q", description)

	return nil
}

// VerifyLinkStateConfiguration verifies link state configuration without requiring connectivity.
// It waits up to CarrierWaitTimeout for carrier status to be established before returning.
func VerifyLinkStateConfiguration(networkName, interfaceName, namespace, description string,
	timeout time.Duration) (bool, error) {
	klog.V(90).Infof("Verifying link state: %q (network: %q, ns: %q)", description, networkName, namespace)

	// Generate unique pod name to avoid collisions
	podName := fmt.Sprintf("linkstate-test-%d", time.Now().UnixNano()%100000)

	testPod, err := CreateTestPod(podName, namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return false, fmt.Errorf("failed to create test pod: %w", err)
	}

	defer func() {
		if testPod != nil {
			_, _ = testPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	if err := VerifyInterfaceReady(testPod, interfaceName); err != nil {
		return false, fmt.Errorf("interface not ready: %w", err)
	}

	// Wait for carrier status with retry - VF link state may take time to propagate
	var hasCarrier bool

	err = wait.PollUntilContextTimeout(context.Background(), tsparams.PollingInterval,
		tsparams.CarrierWaitTimeout, true, func(ctx context.Context) (bool, error) {
			carrier, checkErr := CheckInterfaceCarrier(testPod, interfaceName)
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

	// Execute on node via cluster helper
	shellCmd := fmt.Sprintf("ip link show %s", interfaceName)

	outputMap, err := cluster.ExecCmdWithStdout(APIClient, shellCmd,
		metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName)})
	if err != nil {
		return fmt.Errorf("failed to execute on node: %w", err)
	}

	output, ok := outputMap[nodeName]
	if !ok {
		// Try to find by short hostname
		for host, out := range outputMap {
			if strings.HasPrefix(nodeName, host) || strings.HasPrefix(host, nodeName) {
				output = out

				break
			}
		}

		if output == "" {
			return fmt.Errorf("no output from node %s", nodeName)
		}
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

// ============================================================================
// PCI Address
// ============================================================================

// GetPciAddress gets the PCI address for a pod interface from network status annotation.
// The podInterface parameter should be the interface name (e.g., "net1", "net2") which is unique per pod.
func GetPciAddress(namespace, podName, podInterface string) (string, error) {
	podObj, err := pod.Pull(APIClient, podName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull pod: %w", err)
	}

	annotation := podObj.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if annotation == "" {
		return "", fmt.Errorf("no network status annotation")
	}

	var status []struct {
		Interface  string `json:"interface"`
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
		if networkStatus.Interface == podInterface {
			if networkStatus.DeviceInfo.Pci.PciAddress != "" {
				return networkStatus.DeviceInfo.Pci.PciAddress, nil
			}

			return "", fmt.Errorf("PCI address not present for interface %s", podInterface)
		}
	}

	return "", fmt.Errorf("interface %s not found in pod %s", podInterface, podName)
}

// ============================================================================
// Cleanup Functions
// ============================================================================

// CleanupLeftoverResources cleans up leftover test resources.
// Uses existing sriovoperator functions to remove all networks and policies.
// The enablePrefixCleanup parameter controls whether to perform risky prefix-based
// namespace cleanup (e2e- prefix) in shared clusters. Set to false for safety in
// shared environments, true only when you have exclusive cluster access.
func CleanupLeftoverResources(enablePrefixCleanup bool) error {
	sriovOpNs := SriovOcpConfig.OcpSriovOperatorNamespace

	klog.V(90).Info("Cleaning up leftover test resources")

	// Cleanup test namespaces using label selector for safety in shared clusters.
	labelSelector := fmt.Sprintf("%s=%s", tsparams.TestResourceLabelKey, tsparams.TestResourceLabelValue)

	namespaces, err := namespace.List(APIClient, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		klog.V(90).Infof("Warning: failed to list namespaces with label selector: %v", err)
	}

	for _, ns := range namespaces {
		klog.V(90).Infof("Removing labeled test namespace %q", ns.Definition.Name)

		if delErr := ns.DeleteAndWait(tsparams.CleanupTimeout); delErr != nil {
			klog.V(90).Infof("Warning: failed to delete namespace %q: %v", ns.Definition.Name, delErr)
		}
	}

	// Optional fallback: cleanup namespaces with e2e- prefix (requires explicit opt-in)
	// WARNING: This is risky in shared clusters as it may delete namespaces from other tests.
	// Only enable when you have exclusive cluster access or in isolated test environments.
	if enablePrefixCleanup {
		allNamespaces, err := namespace.List(APIClient, metav1.ListOptions{})
		if err != nil {
			klog.V(90).Infof("Warning: failed to list all namespaces: %v", err)
		} else {
			for _, ns := range allNamespaces {
				if strings.HasPrefix(ns.Definition.Name, "e2e-") {
					klog.V(90).Infof("Removing leftover namespace %q (e2e- prefix)", ns.Definition.Name)

					if delErr := ns.DeleteAndWait(tsparams.CleanupTimeout); delErr != nil {
						klog.V(90).Infof("Warning: failed to delete namespace %q: %v", ns.Definition.Name, delErr)
					}
				}
			}
		}
	}

	// Remove all SR-IOV networks using existing function
	if err := sriovoperator.RemoveAllSriovNetworks(APIClient, sriovOpNs, tsparams.CleanupTimeout); err != nil {
		klog.V(90).Infof("Warning: failed to remove SR-IOV networks: %v", err)
	}

	// Remove all SR-IOV policies and wait for stability using existing function
	if err := sriovoperator.RemoveAllPoliciesAndWaitForSriovAndMCPStable(
		APIClient, "worker", sriovOpNs, tsparams.CleanupTimeout); err != nil {
		klog.V(90).Infof("Warning: failed to remove SR-IOV policies: %v", err)
	}

	klog.V(90).Info("Cleanup completed")

	return nil
}
