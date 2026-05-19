package netenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// DoesClusterHasEnoughNodes verifies if given cluster has enough nodes to run tests.
func DoesClusterHasEnoughNodes(
	apiClient *clients.Settings,
	netConfig *netconfig.NetworkConfig,
	requiredCPNodeNumber int,
	requiredWorkerNodeNumber int) error {
	klog.V(90).Infof("Verifying if cluster has enough workers to run tests")

	workerNodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(netConfig.WorkerLabelMap).String()},
	)
	if err != nil {
		return err
	}

	if len(workerNodeList) < requiredWorkerNodeNumber {
		return fmt.Errorf("cluster has less than %d worker nodes", requiredWorkerNodeNumber)
	}

	controlPlaneNodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(netConfig.ControlPlaneLabelMap).String()},
	)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Verifying if cluster has enough control-plane nodes to run tests")

	if len(controlPlaneNodeList) < requiredCPNodeNumber {
		return fmt.Errorf("cluster has less than %d control-plane nodes", requiredCPNodeNumber)
	}

	return nil
}

// BFDHasStatus verifies that BFD session on a pod has given status.
func BFDHasStatus(frrPod *pod.Builder, bfdPeer string, status string) error {
	var (
		bfdStatusOut bytes.Buffer
		err          error
	)

	err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 10*time.Second, true,
		func(ctx context.Context) (bool, error) {
			bfdStatusOut, err = frrPod.ExecCommand(append(netparam.VtySh, "sh bfd peers brief json"))
			if err != nil {
				klog.V(90).Infof("Failed to execute BFD status command: %v", err)

				return false, err
			}

			if strings.TrimSpace(bfdStatusOut.String()) == "" {
				klog.V(90).Infof("BFD status command returned empty output. retrying...")

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("BFD status command returned empty output")
		}

		return err
	}

	var peers []netparam.BFDDescription

	err = json.Unmarshal(bfdStatusOut.Bytes(), &peers)
	if err != nil {
		klog.V(90).Infof("Failed to Unmarshal bfdStatus string: %s into bfdStatus struct", bfdStatusOut.String())

		return fmt.Errorf(
			"failed to unmarshal BFD status output with error: %w, bfd status output: %s", err, bfdStatusOut.String())
	}

	for _, peer := range peers {
		if peer.BFDPeer == bfdPeer {
			if peer.BFDStatus != status {
				return fmt.Errorf("BFD peer %s on pod %s has status %s (expected %s)",
					bfdPeer, frrPod.Object.Name, peer.BFDStatus, status)
			}

			return nil
		}
	}

	return fmt.Errorf("BFD peer %s not found on pod %s", bfdPeer, frrPod.Object.Name)
}

// MapFirstKeyValue returns the first key-value pair found in the input map.
// If the input map is empty, it returns empty strings.
func MapFirstKeyValue(inputMap map[string]string) (string, string) {
	for key, value := range inputMap {
		return key, value
	}

	return "", ""
}

// BuildRoutesMapWithSpecificRoutes creates a route map with specific routes.
func BuildRoutesMapWithSpecificRoutes(podList []*pod.Builder, workerNodeList []*nodes.Builder,
	nextHopList []string) (map[string]string, error) {
	if len(podList) == 0 {
		klog.V(90).Infof("Pod list is empty")

		return nil, fmt.Errorf("pod list is empty")
	}

	if len(nextHopList) == 0 {
		klog.V(90).Infof("Nexthop IP addresses list is empty")

		return nil, fmt.Errorf("nexthop IP addresses list is empty")
	}

	if len(nextHopList) < len(podList) {
		klog.V(90).Infof("Number of speaker IP addresses[%d] is less than the number of pods[%d]",
			len(nextHopList), len(podList))

		return nil, fmt.Errorf("insufficient speaker IP addresses: got %d, need at least %d",
			len(nextHopList), len(podList))
	}

	routesMap := make(map[string]string)

	for _, frrPod := range podList {
		if frrPod.Definition.Spec.NodeName == workerNodeList[0].Definition.Name {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[1]
		} else {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[0]
		}
	}

	return routesMap, nil
}

// SetStaticRoute could set or delete static route on all Speaker pods.
func SetStaticRoute(frrPod *pod.Builder, action, destIP, containerName string,
	nextHopMap map[string]string) (string, error) {
	buffer, err := frrPod.ExecCommand(
		[]string{"ip", "route", action, destIP, "via", nextHopMap[frrPod.Definition.Spec.NodeName]}, containerName)
	if err != nil {
		if strings.Contains(buffer.String(), "File exists") {
			klog.V(90).Infof("Warning: Route to %s already exist", destIP)

			return buffer.String(), nil
		}

		if strings.Contains(buffer.String(), "No such process") {
			klog.V(90).Infof("Warning: Route to %s already absent", destIP)

			return buffer.String(), nil
		}

		return buffer.String(), err
	}

	return buffer.String(), nil
}

// GetClusterIPFamily detects the cluster's IP family by checking node external OVN addresses.
// Returns netparam.IPV4Family, netparam.IPV6Family, or netparam.DualIPFamily.
func GetClusterIPFamily(apiClient *clients.Settings) (string, error) {
	klog.V(90).Infof("Detecting cluster IP family from OVN external addresses")

	const ovnExternalAddresses = "k8s.ovn.org/node-primary-ifaddr"

	nodesList, err := nodes.List(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodesList) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	// OVN's node-primary-ifaddr is expected to be consistent cluster-wide; use the first listed node only.
	node := nodesList[0]

	raw := node.Object.Annotations[ovnExternalAddresses]
	if raw == "" {
		return "", fmt.Errorf("node %q missing %q annotation", node.Object.Name, ovnExternalAddresses)
	}

	var extNetwork nodes.ExternalNetworks

	err = json.Unmarshal([]byte(raw), &extNetwork)
	if err != nil {
		return "", fmt.Errorf("failed to parse external network annotation on node %s: %w",
			node.Object.Name, err)
	}

	var clusterIPFamily string

	switch {
	case extNetwork.IPv4 != "" && extNetwork.IPv6 != "":
		clusterIPFamily = netparam.DualIPFamily
	case extNetwork.IPv4 != "":
		clusterIPFamily = netparam.IPV4Family
	case extNetwork.IPv6 != "":
		clusterIPFamily = netparam.IPV6Family
	default:
		return "", fmt.Errorf("no IPv4 or IPv6 external address found on node %s", node.Object.Name)
	}

	klog.V(90).Infof("Cluster IP family: %s", clusterIPFamily)

	return clusterIPFamily, nil
}

// ClusterSupportsIPv4 returns true if the cluster supports IPv4 (single-stack or dual-stack).
func ClusterSupportsIPv4(ipFamily string) bool {
	return ipFamily == netparam.IPV4Family || ipFamily == netparam.DualIPFamily
}

// ClusterSupportsIPv6 returns true if the cluster supports IPv6 (single-stack or dual-stack).
func ClusterSupportsIPv6(ipFamily string) bool {
	return ipFamily == netparam.IPV6Family || ipFamily == netparam.DualIPFamily
}

// ClusterSupportsDualStack returns true if the cluster is dual-stack.
func ClusterSupportsDualStack(ipFamily string) bool {
	return ipFamily == netparam.DualIPFamily
}

// WaitUntilVfsCreated waits until all expected SR-IOV VFs are created.
func WaitUntilVfsCreated(
	apiClient *clients.Settings,
	sriovOperatorNamespace string,
	nodeList []*nodes.Builder,
	sriovInterfaceName string,
	numberOfVfs int,
	timeout time.Duration,
) error {
	klog.V(90).Infof("Waiting for the creation of all VFs (%d) under"+
		" the %s interface in the SriovNetworkState.", numberOfVfs, sriovInterfaceName)

	for _, node := range nodeList {
		err := wait.PollUntilContextTimeout(
			context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
				sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
					apiClient, node.Object.Name, sriovOperatorNamespace)

				err := sriovNetworkState.Discover()
				if err != nil {
					return false, nil
				}

				err = isVfCreated(sriovNetworkState, numberOfVfs, sriovInterfaceName)
				if err != nil {
					return false, nil
				}

				return true, nil
			})
		if err != nil {
			return err
		}
	}

	return nil
}

// IsMellanoxDevice checks if a given network interface on a node is a Mellanox device.
func IsMellanoxDevice(apiClient *clients.Settings, sriovOperatorNamespace, intName, nodeName string) (bool, error) {
	klog.V(90).Infof("Checking if specific interface %s on node %s is a Mellanox device.", intName, nodeName)

	sriovNetworkState := sriov.NewNetworkNodeStateBuilder(apiClient, nodeName, sriovOperatorNamespace)

	driverName, err := sriovNetworkState.GetDriverName(intName)
	if err != nil {
		return false, fmt.Errorf("failed to get driver name for interface %s on node %s: %w", intName, nodeName, err)
	}

	return driverName == "mlx5_core", nil
}

// ConfigureSriovMlnxFirmwareOnWorkers configures SR-IOV firmware on a given Mellanox device.
func ConfigureSriovMlnxFirmwareOnWorkers(
	apiClient *clients.Settings,
	sriovOperatorNamespace string,
	workerNodes []*nodes.Builder,
	sriovInterfaceName string,
	enableSriov bool,
	numVfs int,
) error {
	for _, workerNode := range workerNodes {
		klog.V(90).Infof("Configuring SR-IOV firmware on the Mellanox device %s on worker %s"+
			" with parameters: enableSriov %t and numVfs %d",
			sriovInterfaceName, workerNode.Object.Name, enableSriov, numVfs)

		sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
			apiClient, workerNode.Object.Name, sriovOperatorNamespace)

		pciAddress, err := sriovNetworkState.GetPciAddress(sriovInterfaceName)
		if err != nil {
			klog.V(90).Infof("Failed to get PCI address for the interface %s", sriovInterfaceName)

			return fmt.Errorf("failed to get PCI address: %s", err.Error())
		}

		mstconfigCmd := fmt.Sprintf("mstconfig -y -d %s set SRIOV_EN=%t NUM_OF_VFS=%d",
			pciAddress, enableSriov, numVfs)

		output, err := runCommandOnConfigDaemon(apiClient, sriovOperatorNamespace, workerNode.Object.Name,
			[]string{"bash", "-c", mstconfigCmd})
		if err != nil {
			klog.V(90).Infof("Failed to configure SR-IOV firmware.")

			return fmt.Errorf("failed to configure Mellanox firmware for interface %s on a node %s: %s\n %s",
				pciAddress, workerNode.Object.Name, output, err.Error())
		}

		// Reboot is issued separately: the exec session is expected to drop when the node reboots.
		_, rebootErr := runCommandOnConfigDaemon(apiClient, sriovOperatorNamespace, workerNode.Object.Name,
			[]string{"bash", "-c", "chroot /host reboot"})
		if rebootErr != nil && !isRebootExecDisconnectError(rebootErr) {
			return fmt.Errorf("failed to reboot node %s after Mellanox firmware configuration: %s",
				workerNode.Object.Name, rebootErr.Error())
		}
	}

	return nil
}

// ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP configures Mellanox firmware and wait for the cluster becomes stable.
func ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
	apiClient *clients.Settings,
	mcpTimeout time.Duration,
	stableDuration time.Duration,
	mcpLabel string,
	sriovOperatorNamespace string,
	workerNodes []*nodes.Builder,
	sriovInterfaceName string,
	enableSriov bool,
	numVfs int,
) error {
	klog.V(90).Infof("Enabling SR-IOV on Mellanox device")

	err := ConfigureSriovMlnxFirmwareOnWorkers(
		apiClient, sriovOperatorNamespace, workerNodes, sriovInterfaceName, enableSriov, numVfs)
	if err != nil {
		klog.V(90).Infof("Failed to configure SR-IOV Mellanox firmware")

		return err
	}

	time.Sleep(10 * time.Second)

	err = cluster.WaitForMcpStable(apiClient, mcpTimeout, stableDuration, mcpLabel)
	if err != nil {
		klog.V(90).Infof("Machineconfigpool is not stable")

		return err
	}

	return nil
}

// isRebootExecDisconnectError reports whether err is the expected exec failure when reboot closes the session.
func isRebootExecDisconnectError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "unable to upgrade connection") ||
		strings.Contains(msg, "command terminated") ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// isVfCreated checks that the expected number of VFs exists on the given SR-IOV interface.
func isVfCreated(sriovNodeState *sriov.NetworkNodeStateBuilder, vfNumber int, sriovInterfaceName string) error {
	sriovNumVfs, err := sriovNodeState.GetNumVFs(sriovInterfaceName)
	if err != nil {
		return err
	}

	if sriovNumVfs != vfNumber {
		return fmt.Errorf("expected number of VFs %d is not equal to the actual number of VFs %d", vfNumber, sriovNumVfs)
	}

	return nil
}

// runCommandOnConfigDaemon executes command on the sriov-network-config-daemon pod for nodeName.
func runCommandOnConfigDaemon(
	apiClient *clients.Settings,
	sriovOperatorNamespace string,
	nodeName string,
	command []string,
) (string, error) {
	pods, err := pod.List(apiClient, sriovOperatorNamespace, metav1.ListOptions{
		LabelSelector: "app=sriov-network-config-daemon", FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName)})
	if err != nil {
		return "", err
	}

	if len(pods) != 1 {
		return "", fmt.Errorf("there should be exactly one 'sriov-network-config-daemon' pod per node,"+
			" but found %d on node %s", len(pods), nodeName)
	}

	output, err := pods[0].ExecCommand(command)

	return output.String(), err
}
