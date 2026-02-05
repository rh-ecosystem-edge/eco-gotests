package sriovenv

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	nadV1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateSriovInterfaces checks that provided interfaces by env var exist on the nodes.
func ValidateSriovInterfaces(workerNodeList []*nodes.Builder, requestedNumber int) error {
	var validSriovIntefaceList []sriovV1.InterfaceExt

	availableUpSriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(APIClient,
		workerNodeList[0].Definition.Name, NetConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		return fmt.Errorf("failed get SR-IOV devices from the node %s", workerNodeList[0].Definition.Name)
	}

	requestedSriovInterfaceList, err := NetConfig.GetSriovInterfaces(requestedNumber)
	if err != nil {
		return err
	}

	for _, availableUpSriovInterface := range availableUpSriovInterfaces {
		for _, requestedSriovInterface := range requestedSriovInterfaceList {
			if availableUpSriovInterface.Name == requestedSriovInterface {
				validSriovIntefaceList = append(validSriovIntefaceList, availableUpSriovInterface)
			}
		}
	}

	if len(validSriovIntefaceList) < requestedNumber {
		return fmt.Errorf("requested interfaces %v are not present on the cluster node", requestedSriovInterfaceList)
	}

	return nil
}

// CreateSriovNetworkAndWaitForNADCreation creates a SriovNetwork and waits for NAD Creation on the test namespace.
func CreateSriovNetworkAndWaitForNADCreation(sNet *sriov.NetworkBuilder, timeout time.Duration) error {
	klog.V(90).Infof("Creating SriovNetwork %s and waiting for net-attach-def to be created", sNet.Definition.Name)

	sriovNetwork, err := sNet.Create()
	if err != nil {
		return err
	}

	return WaitForNADCreation(sriovNetwork.Object.Name, TargetNamespaceOf(sriovNetwork), timeout)
}

// WaitForNADCreation waits for the NAD to be created.
func WaitForNADCreation(name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.TODO(),
		time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(APIClient, name, namespace)
			if err != nil {
				klog.V(100).Infof("Failed to get NAD %s in namespace %s: %v",
					name, namespace, err)

				return false, nil
			}

			return true, nil
		})
}

// WaitForNADDeletion waits for the NAD to be deleted.
func WaitForNADDeletion(name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.TODO(),
		time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var testNAD nadV1.NetworkAttachmentDefinition

			err := APIClient.Client.Get(context.TODO(), k8sclient.ObjectKey{Name: name, Namespace: namespace}, &testNAD)
			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})
}

// TargetNamespaceOf returns the target namespace of a SriovNetwork.
// If the target namespace is not set, it returns the namespace of the SriovNetwork.
func TargetNamespaceOf(sriovNetwork *sriov.NetworkBuilder) string {
	if sriovNetwork.Object.Spec.NetworkNamespace != "" {
		return sriovNetwork.Object.Spec.NetworkNamespace
	}

	return sriovNetwork.Object.Namespace
}

// DefineAndCreateSriovNetwork creates an enhanced SriovNetwork with optional features and waits for NAD creation.
func DefineAndCreateSriovNetwork(networkName, resourceName string, withStaticIP, withTrust bool) error {
	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, networkName, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithMacAddressSupport().
		WithLogLevel(netparam.LogLevelDebug)

	// Enable VF trust for advanced network operations (balance-tlb/alb.)
	if withTrust {
		networkBuilder = networkBuilder.WithTrustFlag(true)
	}

	if withStaticIP {
		networkBuilder = networkBuilder.WithStaticIpam()
	}

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// DiscoverInterfaceUnderTestDeviceID discovers device ID for a given SR-IOV interface.
func DiscoverInterfaceUnderTestDeviceID(srIovInterfaceUnderTest, workerNodeName string) string {
	sriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		klog.V(90).Infof("Failed to discover device ID for network interface %s: %v",
			srIovInterfaceUnderTest, err)

		return ""
	}

	for _, srIovInterface := range sriovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			return srIovInterface.DeviceID
		}
	}

	return ""
}

// WaitUntilVfsCreated waits until all expected SR-IOV VFs are created.
func WaitUntilVfsCreated(
	nodeList []*nodes.Builder, sriovInterfaceName string, numberOfVfs int, timeout time.Duration) error {
	klog.V(90).Infof("Waiting for the creation of all VFs (%d) under"+
		" the %s interface in the SriovNetworkState.", numberOfVfs, sriovInterfaceName)

	for _, node := range nodeList {
		err := wait.PollUntilContextTimeout(
			context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
				sriovNetworkState := sriov.NewNetworkNodeStateBuilder(APIClient, node.Object.Name, NetConfig.SriovOperatorNamespace)

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
func IsMellanoxDevice(intName, nodeName string) bool {
	klog.V(90).Infof("Checking if specific interface %s on node %s is a Mellanox device.", intName, nodeName)
	sriovNetworkState := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, NetConfig.SriovOperatorNamespace)

	driverName, err := sriovNetworkState.GetDriverName(intName)
	if err != nil {
		klog.V(90).Infof("Failed to get driver name for interface %s on node %s: %v", intName, nodeName, err)

		return false
	}

	return driverName == "mlx5_core"
}

// ConfigureSriovMlnxFirmwareOnWorkers configures SR-IOV firmware on a given Mellanox device.
func ConfigureSriovMlnxFirmwareOnWorkers(
	workerNodes []*nodes.Builder, sriovInterfaceName string, enableSriov bool, numVfs int) error {
	for _, workerNode := range workerNodes {
		klog.V(90).Infof("Configuring SR-IOV firmware on the Mellanox device %s on the workers"+
			" %v with parameters: enableSriov %t and numVfs %d", sriovInterfaceName, workerNodes, enableSriov, numVfs)

		sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
			APIClient, workerNode.Object.Name, NetConfig.SriovOperatorNamespace)

		pciAddress, err := sriovNetworkState.GetPciAddress(sriovInterfaceName)
		if err != nil {
			klog.V(90).Infof("Failed to get PCI address for the interface %s", sriovInterfaceName)

			return fmt.Errorf("failed to get PCI address: %s", err.Error())
		}

		output, err := runCommandOnConfigDaemon(workerNode.Object.Name,
			[]string{"bash", "-c",
				fmt.Sprintf("mstconfig -y -d %s set SRIOV_EN=%t NUM_OF_VFS=%d && chroot /host reboot",
					pciAddress, enableSriov, numVfs)})
		if err != nil {
			klog.V(90).Infof("Failed to configure SR-IOV firmware.")

			return fmt.Errorf("failed to configure Mellanox firmware for interface %s on a node %s: %s\n %s",
				pciAddress, workerNode.Object.Name, output, err.Error())
		}
	}

	return nil
}

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

func runCommandOnConfigDaemon(nodeName string, command []string) (string, error) {
	pods, err := pod.List(APIClient, NetConfig.SriovOperatorNamespace, metav1.ListOptions{
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

// createAndWaitTestPods creates test pods and waits until they are in the ready state.
func createAndWaitTestPods(
	clientNodeName string,
	serverNodeName string,
	sriovResNameClient string,
	sriovResNameServer string,
	clientMac string,
	serverMac string,
	clientIPs []string,
	serverIPs []string) (client *pod.Builder, server *pod.Builder, err error) {
	klog.V(90).Infof("Creating client pod with IPs %v, mac %s, SR-IOV resourceName %s"+
		" and server pod with IPs %v, mac %s, SR-IOV resourceName %s.",
		clientIPs, clientMac, sriovResNameClient, serverIPs, serverMac, sriovResNameServer)

	clientPod, err := CreateAndWaitTestPodWithSecondaryNetwork("client", clientNodeName,
		sriovResNameClient, clientMac, clientIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create clientPod")

		return nil, nil, err
	}

	serverPod, err := CreateAndWaitTestPodWithSecondaryNetwork("server", serverNodeName,
		sriovResNameServer, serverMac, serverIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create serverPod")

		return nil, nil, err
	}

	return clientPod, serverPod, nil
}

// CreateAndWaitTestPodWithSecondaryNetwork creates test pod with secondary network
// and waits until it is in the ready state.
func CreateAndWaitTestPodWithSecondaryNetwork(
	podName string,
	testNodeName string,
	sriovResNameTest string,
	testMac string,
	testIPs []string) (*pod.Builder, error) {
	klog.V(90).Infof("Creating a test pod name %s", podName)

	secNetwork := pod.StaticIPAnnotationWithMacAddress(sriovResNameTest, testIPs, testMac)

	testPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(testNodeName).WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	if err != nil {
		klog.V(90).Infof("Failed to create pod %s with secondary network", podName)

		return nil, err
	}

	return testPod, nil
}

// CreatePodsAndRunTraffic creates test pods and verifies connectivity between them.
func CreatePodsAndRunTraffic(
	clientNodeName string,
	serverNodeName string,
	sriovResNameClient string,
	sriovResNameServer string,
	clientMac string,
	serverMac string,
	clientIPs []string,
	serverIPs []string) error {
	klog.V(90).Infof("Creating test pods and checking ICMP connectivity between them")

	clientPod, _, err := createAndWaitTestPods(
		clientNodeName,
		serverNodeName,
		sriovResNameClient,
		sriovResNameServer,
		clientMac,
		serverMac,
		clientIPs,
		serverIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create test pods")

		return err
	}

	return cmd.ICMPConnectivityCheck(clientPod, serverIPs)
}

// ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP configures Mellanox firmware and wait for the cluster becomes stable.
func ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
	workerNodes []*nodes.Builder, sriovInterfaceName string, enableSriov bool, numVfs int) error {
	klog.V(90).Infof("Enabling SR-IOV on Mellanox device")

	err := ConfigureSriovMlnxFirmwareOnWorkers(workerNodes, sriovInterfaceName, enableSriov, numVfs)
	if err != nil {
		klog.V(90).Infof("Failed to configure SR-IOV Mellanox firmware")

		return err
	}

	time.Sleep(10 * time.Second)

	err = cluster.WaitForMcpStable(APIClient, tsparams.MCOWaitTimeout, 1*time.Minute, NetConfig.CnfMcpLabel)
	if err != nil {
		klog.V(90).Infof("Machineconfigpool is not stable")

		return err
	}

	return nil
}

// DefinePod returns basic test pod definition with and without secondary interface.
func DefinePod(name, role, ifName, worker string, secondaryInterface bool) *pod.Builder {
	klog.V(90).Infof("Defining test pod %s on worker %s", name, worker)

	podbuild := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithNodeSelector(map[string]string{corev1.LabelHostname: worker}).
		WithPrivilegedFlag()

	if secondaryInterface {
		var netAnnotation []*types.NetworkSelectionElement

		if role == "server" {
			netAnnotation = []*types.NetworkSelectionElement{
				{
					Name:       ifName,
					MacRequest: tsparams.ServerMacAddress,
					IPRequest:  []string{tsparams.ServerIPv4IPAddress},
				},
			}
		} else {
			netAnnotation = []*types.NetworkSelectionElement{
				{
					Name:       ifName,
					MacRequest: tsparams.ClientMacAddress,
					IPRequest:  []string{tsparams.ClientIPv4IPAddress},
				},
			}
		}

		podbuild.WithSecondaryNetwork(netAnnotation)
	}

	return podbuild
}

// DiscoverInterfaceUnderTestVendorID discovers vendor ID for a given SR-IOV interface.
func DiscoverInterfaceUnderTestVendorID(srIovInterfaceUnderTest, workerNodeName string) (string, error) {
	sriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		return "", err
	}

	for _, srIovInterface := range sriovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			return srIovInterface.Vendor, nil
		}
	}

	return "", fmt.Errorf("interface %s not found", srIovInterfaceUnderTest)
}

// CreateSriovNetworkWithStaticIPAM creates an SR-IOV network with static IPAM, IP address, and MAC address support.
func CreateSriovNetworkWithStaticIPAM(name, resourceName string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with static IPAM", name)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithStaticIpam().
		WithIPAddressSupport().
		WithMacAddressSupport().
		WithLogLevel(netparam.LogLevelDebug)

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// CreateSriovNetworkWithWhereaboutsIPAM creates an SR-IOV network with whereabouts IPAM for dynamic IP assignment.
// ipRange should be in CIDR notation (e.g., "2001:100::/64" for IPv6 or "192.168.1.0/24" for IPv4).
// gateway is the gateway address for the range.
func CreateSriovNetworkWithWhereaboutsIPAM(
	name, resourceName, ipRange, gateway, networkName string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with whereabouts IPAM, range %s, gateway %s",
		name, ipRange, gateway)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).WithWhereaboutsIPAM(ipRange, gateway, "", networkName)

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// CreateSriovNetworkWithVLANAndWhereabouts creates an SR-IOV network with Whereabouts IPAM and VLAN tagging.
func CreateSriovNetworkWithVLANAndWhereabouts(name, resourceName string, vlanID uint16,
	ipRange, gateway string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with Whereabouts IPAM, VLAN %d, range %s",
		name, vlanID, ipRange)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).WithVLAN(vlanID).WithWhereaboutsIPAM(ipRange, gateway, "", "")

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// getInterfaceIPs returns all IPs assigned to an interface from the pod's network-status annotation.
func getInterfaceIPs(podBuilder *pod.Builder, interfaceName string) ([]string, error) {
	podObj, err := pod.Pull(APIClient, podBuilder.Definition.Name, podBuilder.Definition.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull pod %s: %w", podBuilder.Definition.Name, err)
	}

	annotation := podObj.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if annotation == "" {
		return nil, fmt.Errorf("no network-status annotation on pod %s", podBuilder.Definition.Name)
	}

	var statuses []struct {
		Interface string   `json:"interface"`
		IPs       []string `json:"ips"`
	}

	if err := json.Unmarshal([]byte(annotation), &statuses); err != nil {
		return nil, fmt.Errorf("failed to parse network-status annotation: %w", err)
	}

	for _, status := range statuses {
		if status.Interface == interfaceName {
			return status.IPs, nil
		}
	}

	return nil, fmt.Errorf("interface %s not found in network-status annotation", interfaceName)
}

// GetPodIPFromInterface retrieves an IP address of a specific interface from a pod's network-status annotation.
// ipFamily should be "ipv4" or "ipv6". For dual-stack, call this function twice with each family.
func GetPodIPFromInterface(podBuilder *pod.Builder, interfaceName, ipFamily string) (string, error) {
	klog.V(90).Infof("Getting %s from interface %s on pod %s", ipFamily, interfaceName, podBuilder.Definition.Name)

	ips, err := getInterfaceIPs(podBuilder, interfaceName)
	if err != nil {
		return "", err
	}

	for _, ip := range ips {
		ipClean := strings.Split(ip, "/")[0]
		isIPv6 := strings.Contains(ipClean, ":")

		if ipFamily == "ipv4" && !isIPv6 {
			return ipClean, nil
		}

		// Skip link-local addresses (fe80::) for IPv6 - return only global/ULA addresses.
		if ipFamily == "ipv6" && isIPv6 && !strings.HasPrefix(strings.ToLower(ipClean), "fe80") {
			return ipClean, nil
		}
	}

	return "", fmt.Errorf("no %s found for interface %s in network-status annotation", ipFamily, interfaceName)
}

// CreatePodPair creates a client and server pod pair for traffic testing.
func CreatePodPair(
	clientName string,
	serverName string,
	clientNode string,
	serverNode string,
	clientNetwork string,
	serverNetwork string,
	clientIPs []string,
	serverIPs []string,
	serverBindIP string,
	clientMAC string,
	serverMAC string,
	mtu int,
) (*pod.Builder, *pod.Builder, error) {
	klog.V(90).Infof("Creating client pod %s and server pod %s", clientName, serverName)

	client, err := CreateTestClientPod(clientName, clientNode, clientNetwork, clientIPs, clientMAC)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client pod: %w", err)
	}

	server, err := CreateTestServerPod(
		serverName, serverNode, serverNetwork, serverIPs, serverBindIP, serverMAC, mtu)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server pod: %w", err)
	}

	return client, server, nil
}

// CreateAllSriovPolicies creates all SR-IOV policies for testing.
// It creates policies for PF1 and PF2 at two MTU sizes (small and large).
// VF allocation: 10 total VFs per PF, VFs 0-4 for small MTU, VFs 5-9 for large MTU.
// mtuSmall is typically 500 for IPv4 or 1280 for IPv6.
func CreateAllSriovPolicies(
	pf1 string,
	pf2 string,
	resourcePF1SmallMTU string,
	resourcePF1LargeMTU string,
	resourcePF2SmallMTU string,
	resourcePF2LargeMTU string,
	policyPrefix string,
	mtuSmall int,
	mtuLarge int,
) error {
	klog.V(90).Infof("Creating SR-IOV policies for testing")

	const (
		vfStartSmallMTU = 0
		vfEndSmallMTU   = 4
		vfStartLargeMTU = 5
		vfEndLargeMTU   = 9
	)

	// Create policy for PF1 with small MTU
	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf1-mtu%d", policyPrefix, mtuSmall),
		resourcePF1SmallMTU, pf1, mtuSmall,
		vfStartSmallMTU, vfEndSmallMTU); err != nil {
		return fmt.Errorf("failed to create PF1 MTU%d policy: %w", mtuSmall, err)
	}

	// Create policy for PF1 with large MTU
	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf1-mtu%d", policyPrefix, mtuLarge),
		resourcePF1LargeMTU, pf1, mtuLarge,
		vfStartLargeMTU, vfEndLargeMTU); err != nil {
		return fmt.Errorf("failed to create PF1 MTU%d policy: %w", mtuLarge, err)
	}

	// Create policy for PF2 with small MTU
	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf2-mtu%d", policyPrefix, mtuSmall),
		resourcePF2SmallMTU, pf2, mtuSmall,
		vfStartSmallMTU, vfEndSmallMTU); err != nil {
		return fmt.Errorf("failed to create PF2 MTU%d policy: %w", mtuSmall, err)
	}

	// Create policy for PF2 with large MTU
	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf2-mtu%d", policyPrefix, mtuLarge),
		resourcePF2LargeMTU, pf2, mtuLarge,
		vfStartLargeMTU, vfEndLargeMTU); err != nil {
		return fmt.Errorf("failed to create PF2 MTU%d policy: %w", mtuLarge, err)
	}

	if err := sriovoperator.WaitForSriovAndMCPStable(
		APIClient,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration,
		NetConfig.WorkerLabelEnvVar,
		NetConfig.SriovOperatorNamespace); err != nil {
		return fmt.Errorf("failed to wait for SR-IOV and MCP stability: %w", err)
	}

	return nil
}

// CreateSriovPolicy creates a single SR-IOV policy without waiting for MCP stability.
func CreateSriovPolicy(
	name string,
	resourceName string,
	pfName string,
	mtu int,
	vfStart int,
	vfEnd int,
) error {
	klog.V(90).Infof("Creating SR-IOV policy %s", name)

	const totalVFs = 10

	policy := sriov.NewPolicyBuilder(
		APIClient,
		name,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		totalVFs,
		[]string{pfName},
		NetConfig.WorkerLabelMap).
		WithMTU(mtu).
		WithVFRange(vfStart, vfEnd)

	_, err := policy.Create()
	if err != nil {
		return err
	}

	return nil
}

// CreateTestClientPod creates a client pod with SR-IOV interface.
func CreateTestClientPod(
	name string,
	nodeName string,
	networkName string,
	ipAddresses []string,
	macAddress string,
) (*pod.Builder, error) {
	klog.V(90).Infof("Creating client pod %s on node %s", name, nodeName)

	secNetwork := []*types.NetworkSelectionElement{{Name: networkName}}

	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	command := []string{"bash", "-c", "sleep infinity"}

	container, err := pod.NewContainerBuilder("test", NetConfig.CnfNetTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	return pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*container).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
}

// CreateTestServerPod creates a server pod with testcmd listeners for TCP, UDP, SCTP, and multicast.
func CreateTestServerPod(
	name string,
	nodeName string,
	networkName string,
	ipAddresses []string,
	serverBindIP string,
	macAddress string,
	mtu int,
) (*pod.Builder, error) {
	klog.V(90).Infof("Creating server pod %s on node %s", name, nodeName)

	secNetwork := []*types.NetworkSelectionElement{{Name: networkName}}

	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	command := BuildServerCommand(serverBindIP, tsparams.Net1Interface, mtu)

	container, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	serverPod, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*container).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	if err != nil {
		return nil, err
	}

	// Wait for testcmd listeners to be ready.
	if err := WaitForServerReady(serverPod, tsparams.WaitTimeout); err != nil {
		return nil, fmt.Errorf("server pod %s not ready: %w", name, err)
	}

	return serverPod, nil
}

// WaitForServerReady waits for the server pod's testcmd listeners to be ready.
func WaitForServerReady(serverPod *pod.Builder, timeout time.Duration) error {
	klog.V(90).Infof("Waiting for server pod %s to be ready", serverPod.Definition.Name)

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.RetryInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, execErr := serverPod.ExecCommand([]string{"bash", "-c", "pgrep -f testcmd"})
			if execErr != nil {
				klog.V(90).Infof("testcmd not ready on pod %s: %v", serverPod.Definition.Name, execErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("testcmd listeners not ready on pod %s: %w", serverPod.Definition.Name, err)
	}

	return nil
}

// BuildServerCommand builds the command to start testcmd listeners on the server pod.
// For dynamic IP (serverBindIP == ""), the IP is discovered at runtime inside the pod.
// For static IP, the provided serverBindIP is used directly.
func BuildServerCommand(serverBindIP, interfaceName string, mtu int) []string {
	klog.V(90).Infof("Building server command for interface %s with MTU %d, serverBindIP=%q",
		interfaceName, mtu, serverBindIP)

	// Subtract header overhead to fit within MTU.
	// Accounts for IP headers, protocol headers, and testcmd overhead.
	packetSize := mtu - 100

	if serverBindIP == "" {
		return buildDynamicIPServerCommand(interfaceName, mtu, packetSize)
	}

	return buildStaticIPServerCommand(serverBindIP, interfaceName, mtu, packetSize)
}

// getIPv4MulticastConfig returns the IPv4 multicast group and MAC address based on MTU.
func getIPv4MulticastConfig(mtu int) (group, mac string) {
	if mtu > 1500 {
		return tsparams.MulticastIPv4GroupLargeMTU, tsparams.MulticastIPv4MACLargeMTU
	}

	return tsparams.MulticastIPv4Group, tsparams.MulticastIPv4MAC
}

// buildTestcmdListeners returns the shell commands to start all testcmd listeners.
// serverIP and mcastGroup can be shell variables (e.g., $SERVER_IP, $MCAST_GROUP) or literal values.
func buildTestcmdListeners(interfaceName, serverIP, mcastGroup string, packetSize int) string {
	return fmt.Sprintf(
		"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
			"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port 5003 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & "+
			"sleep infinity",
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, serverIP, packetSize,
		interfaceName, mcastGroup, packetSize)
}

// buildMulticastSetup returns the shell command to configure multicast and the multicast group address.
// For IPv6, it also adds a route to the local table for the multicast group.
func buildMulticastSetup(isIPv6 bool, interfaceName string, mtu int) (setupCmd, multicastGroup string) {
	ipv4Group, ipv4MAC := getIPv4MulticastConfig(mtu)

	ipv6Group := tsparams.MulticastIPv6Group
	ipv6MAC := tsparams.MulticastIPv6MAC

	if isIPv6 {
		return fmt.Sprintf(
			"ip maddr add %s dev %s 2>/dev/null || true; "+
				"ip -6 route add %s/128 dev %s table local 2>/dev/null || true; ",
			ipv6MAC, interfaceName, ipv6Group, interfaceName), ipv6Group
	}

	return fmt.Sprintf("ip maddr add %s dev %s 2>/dev/null || true; ",
		ipv4MAC, interfaceName), ipv4Group
}

// buildDynamicIPServerCommand builds the server command for Whereabouts IPAM
// where the IP is discovered at runtime inside the pod.
func buildDynamicIPServerCommand(interfaceName string, mtu, packetSize int) []string {
	// Step 1: Discover server IP (try IPv4 first, then IPv6).
	discoverIP := fmt.Sprintf(
		"for _ in $(seq 1 10); do "+
			"SERVER_IP=$(ip -4 -o addr show %s 2>/dev/null | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
			"[ -n \"$SERVER_IP\" ] && break; "+
			"SERVER_IP=$(ip -6 -o addr show %s 2>/dev/null | grep -v fe80 | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
			"[ -n \"$SERVER_IP\" ] && break; "+
			"sleep 1; done; "+
			"[ -n \"$SERVER_IP\" ] || { echo 'Failed to discover server IP'; exit 1; }; "+
			"echo \"Discovered server IP: $SERVER_IP\"; ",
		interfaceName, interfaceName)

	// Step 2: Configure multicast based on discovered IP version (determined at runtime).
	ipv6Setup, ipv6Group := buildMulticastSetup(true, interfaceName, mtu)
	ipv4Setup, ipv4Group := buildMulticastSetup(false, interfaceName, mtu)

	setupMulticast := fmt.Sprintf(
		"if echo \"$SERVER_IP\" | grep -q ':'; then "+
			"MCAST_GROUP='%s'; %s"+
			"else "+
			"MCAST_GROUP='%s'; %s"+
			"fi; ",
		ipv6Group, ipv6Setup,
		ipv4Group, ipv4Setup)

	// Step 3: Start listeners using shell variables set above.
	listeners := buildTestcmdListeners(interfaceName, "$SERVER_IP", "$MCAST_GROUP", packetSize)

	return []string{"bash", "-c", discoverIP + setupMulticast + listeners}
}

// buildStaticIPServerCommand builds the server command when the IP is known at pod creation time.
func buildStaticIPServerCommand(serverBindIP, interfaceName string, mtu, packetSize int) []string {
	isIPv6 := strings.Contains(serverBindIP, ":")
	multicastSetup, multicastGroup := buildMulticastSetup(isIPv6, interfaceName, mtu)

	listeners := buildTestcmdListeners(interfaceName, serverBindIP, multicastGroup, packetSize)

	return []string{"bash", "-c", multicastSetup + "sleep 5; " + listeners}
}

// RunTrafficTestsForBothMTUs runs traffic tests for two different MTU configurations.
// mtuSmall is typically 500 for IPv4 or 1280 for IPv6, mtuLarge is typically 9000.
func RunTrafficTestsForBothMTUs(
	clientSmallMTU, clientLargeMTU *pod.Builder,
	serverIP1, serverIP2 string,
	mtuSmall, mtuLarge int,
) error {
	klog.V(90).Infof("Running traffic tests with MTU %d", mtuSmall)

	err := RunTrafficTest(clientSmallMTU, serverIP1, mtuSmall)
	if err != nil {
		return fmt.Errorf("traffic tests failed for MTU %d: %w", mtuSmall, err)
	}

	klog.V(90).Infof("Running traffic tests with MTU %d", mtuLarge)

	err = RunTrafficTest(clientLargeMTU, serverIP2, mtuLarge)
	if err != nil {
		return fmt.Errorf("traffic tests failed for MTU %d: %w", mtuLarge, err)
	}

	return nil
}

// CreateSriovNetworksForBothMTUs creates SR-IOV networks for two MTU configurations.
func CreateSriovNetworksForBothMTUs(
	networkNameSmallMTU,
	networkNameLargeMTU,
	resourceSmallMTU,
	resourceLargeMTU string,
) error {
	klog.V(90).Infof("Creating SR-IOV networks for both MTU sizes")

	err := CreateSriovNetworkWithStaticIPAM(networkNameSmallMTU, resourceSmallMTU)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for small MTU: %w", err)
	}

	err = CreateSriovNetworkWithStaticIPAM(networkNameLargeMTU, resourceLargeMTU)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for large MTU: %w", err)
	}

	return nil
}

// RunTrafficTest runs all traffic type tests (ICMP, TCP, UDP, SCTP, multicast) between client and server pods.
func RunTrafficTest(clientPod *pod.Builder, serverIP string, mtu int) error {
	klog.V(90).Infof("Running traffic tests against %s with MTU %d", serverIP, mtu)
	serverIPAddress := ipaddr.RemovePrefix(serverIP)

	// Subtract header overhead to fit within MTU.
	// Accounts for IP headers, protocol headers, and testcmd overhead.
	packetSize := mtu - 100

	var failedProtocols []string

	serverIPWithPrefix := serverIPAddress + "/32"
	if strings.Contains(serverIPAddress, ":") {
		serverIPWithPrefix = serverIPAddress + "/128"
	}

	if err := cmd.ICMPConnectivityCheck(
		clientPod, []string{serverIPWithPrefix}, tsparams.Net1Interface); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("ICMP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("TCP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("UDP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("SCTP: %v", err))
	}

	multicastGroup := tsparams.MulticastIPv4Group
	if strings.Contains(serverIPAddress, ":") {
		multicastGroup = tsparams.MulticastIPv6Group
	} else if mtu == 9000 {
		multicastGroup = tsparams.MulticastIPv4GroupLargeMTU
	}

	if err := RunProtocolTest(clientPod, "multicast",
		fmt.Sprintf("testcmd -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, multicastGroup, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("multicast: %v", err))
	}

	if len(failedProtocols) > 0 {
		return fmt.Errorf("traffic tests failed: %s", strings.Join(failedProtocols, "; "))
	}

	return nil
}

// RunProtocolTest executes a protocol-specific connectivity test command.
func RunProtocolTest(clientPod *pod.Builder, protocol, cmdStr string) error {
	klog.V(90).Infof("Running %s connectivity test", protocol)

	output, err := clientPod.ExecCommand([]string{"bash", "-c", cmdStr})
	if err != nil {
		return fmt.Errorf("%s connectivity check failed (output: %s): %w", protocol, output.String(), err)
	}

	return nil
}
