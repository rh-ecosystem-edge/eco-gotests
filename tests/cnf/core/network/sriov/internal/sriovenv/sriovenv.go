package sriovenv

import (
	"context"
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
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
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

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
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

// PolicyConfig defines a single SR-IOV policy configuration.
type PolicyConfig struct {
	Name         string
	ResourceName string
	PFName       string
	MTU          int
	NumVFs       int
	VFStart      int
	VFEnd        int
}

// CreateSriovPolicies creates multiple SR-IOV policies from a slice of configurations.
// This allows creating all policies upfront with a single node reboot instead of multiple reboots.
func CreateSriovPolicies(configs []PolicyConfig) error {
	for _, cfg := range configs {
		klog.V(90).Infof("Creating SR-IOV policy %s with MTU %d, VFs %d-%d",
			cfg.Name, cfg.MTU, cfg.VFStart, cfg.VFEnd)

		err := CreateSriovPolicyWithMTU(cfg.Name, cfg.ResourceName, cfg.PFName,
			cfg.MTU, cfg.NumVFs, cfg.VFStart, cfg.VFEnd)
		if err != nil {
			return fmt.Errorf("failed to create SR-IOV policy %s: %w", cfg.Name, err)
		}
	}

	return nil
}

// CreateSriovPolicyWithMTU creates an SR-IOV network node policy with MTU and VF range configuration.
func CreateSriovPolicyWithMTU(name, resourceName, pfName string, mtu, numVfs, vfStart, vfEnd int) error {
	klog.V(90).Infof("Creating SR-IOV policy %s with MTU %d, VFs %d-%d", name, mtu, vfStart, vfEnd)

	_, err := sriov.NewPolicyBuilder(
		APIClient,
		name,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		numVfs,
		[]string{pfName},
		NetConfig.WorkerLabelMap).
		WithMTU(mtu).
		WithVFRange(vfStart, vfEnd).
		Create()

	return err
}

// CreateSriovNetworkWithStaticIPAM creates an SR-IOV network with static IPAM, IP address, and MAC address support.
func CreateSriovNetworkWithStaticIPAM(name, resourceName string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with static IPAM", name)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithStaticIpam().
		WithIPAddressSupport().
		WithMacAddressSupport()

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
}

// CreateSriovNetworkWithWhereaboutsIPAM creates an SR-IOV network with whereabouts IPAM for dynamic IP assignment.
// ipRange should be in CIDR notation (e.g., "2001:100::/64" for IPv6 or "192.168.1.0/24" for IPv4).
// gateway is the gateway address for the range.
func CreateSriovNetworkWithWhereaboutsIPAM(name, resourceName, ipRange, gateway string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with whereabouts IPAM, range %s, gateway %s",
		name, ipRange, gateway)

	// Build whereabouts IPAM JSON.
	ipamJSON := fmt.Sprintf(`{"type": "whereabouts", "range": "%s", "gateway": "%s"}`, ipRange, gateway)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName)

	// Set the IPAM directly on the spec.
	networkBuilder.Definition.Spec.IPAM = ipamJSON

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
}

// CreateSriovNetworkWithVLAN creates an SR-IOV network with static IPAM and VLAN tagging.
// vlanID is the 802.1Q VLAN ID to tag traffic with.
func CreateSriovNetworkWithVLAN(name, resourceName string, vlanID uint16) error {
	klog.V(90).Infof("Creating SR-IOV network %s with static IPAM and VLAN %d", name, vlanID)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithStaticIpam().
		WithIPAddressSupport().
		WithMacAddressSupport().
		WithVLAN(vlanID)

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
}

// CreateSriovNetworkWithVLANAndWhereabouts creates an SR-IOV network with Whereabouts IPAM and VLAN tagging.
// This enables dynamic IP assignment with VLAN isolation.
func CreateSriovNetworkWithVLANAndWhereabouts(name, resourceName string, vlanID uint16,
	ipRange, gateway string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with Whereabouts IPAM, VLAN %d, range %s",
		name, vlanID, ipRange)

	// Build whereabouts IPAM JSON.
	ipamJSON := fmt.Sprintf(`{"type": "whereabouts", "range": "%s", "gateway": "%s"}`, ipRange, gateway)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithVLAN(vlanID)

	// Set the IPAM directly on the spec.
	networkBuilder.Definition.Spec.IPAM = ipamJSON

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
}

// DeleteSriovNetworks deletes the specified SR-IOV networks by name.
func DeleteSriovNetworks(networkNames ...string) error {
	for _, networkName := range networkNames {
		klog.V(90).Infof("Deleting SR-IOV network %s", networkName)

		network, err := sriov.PullNetwork(APIClient, networkName, NetConfig.SriovOperatorNamespace)
		if err != nil {
			// Network doesn't exist, skip.
			klog.V(90).Infof("SR-IOV network %s not found, skipping", networkName)

			continue
		}

		targetNamespace := TargetNamespaceOf(network)

		err = network.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete SR-IOV network %s: %w", networkName, err)
		}

		err = WaitForNADDeletion(networkName, targetNamespace, tsparams.DefaultTimeout)
		if err != nil {
			return fmt.Errorf("failed waiting for NAD deletion of %s: %w", networkName, err)
		}
	}

	return nil
}

// CreateTestPod creates a test pod with SR-IOV interface.
// isServer: if true, creates a server pod with testcmd listeners; if false, creates a client pod.
// ipAddresses can be a single IP or multiple IPs for dual-stack (e.g., []string{"192.168.1.1/24", "2001::1/64"}).
// Pass nil or empty ipAddresses for whereabouts/dynamic IP assignment.
// Pass empty macAddress for dynamic MAC assignment.
// serverBindIP is the specific IP (without prefix) that testcmd should bind to (server only).
// Pass empty serverBindIP to auto-discover the IP from the net1 interface at runtime.
// mtu is the MTU size for testcmd (server only, ignored for client).
func CreateTestPod(name, nodeName, networkName string, ipAddresses []string, macAddress string,
	isServer bool, serverBindIP string, mtu int) (*pod.Builder, error) {
	if isServer {
		klog.V(90).Infof("Creating server pod %s on node %s with MTU %d, bindIP %s", name, nodeName, mtu, serverBindIP)
	} else {
		klog.V(90).Infof("Creating client pod %s on node %s", name, nodeName)
	}

	secNetwork := []*types.NetworkSelectionElement{
		{
			Name: networkName,
		},
	}

	// Only set MAC if provided (non-empty).
	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	// Only set IPs if provided (non-nil and non-empty).
	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	var podCmd []string

	var containerName string

	if isServer {
		containerName = "server"
		// Use mtu-100 for packet size to match client (accounting for headers).
		packetSize := mtu - 100

		if serverBindIP == "" {
			// Dynamic IP: discover IP from net1 interface at runtime.
			podCmd = []string{"bash", "-c", fmt.Sprintf(
				"sleep 5; "+
					"SERVER_IP=$(ip -o addr show net1 | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
					"echo \"Discovered server IP: $SERVER_IP\"; "+
					"if [[ \"$SERVER_IP\" == *:* ]]; then TCP_IP=\"[$SERVER_IP]\"; else TCP_IP=\"$SERVER_IP\"; fi; "+
					"testcmd -listen -protocol tcp -port 5001 -interface net1 -server $TCP_IP -mtu %d & "+
					"testcmd -listen -protocol udp -port 5002 -interface net1 -server $SERVER_IP -mtu %d & "+
					"testcmd -listen -protocol sctp -port 5003 -interface net1 -server $SERVER_IP -mtu %d & "+
					"sleep infinity",
				packetSize, packetSize, packetSize)}
		} else {
			// Static IP: use provided serverBindIP.
			// TCP needs brackets around IPv6 for port binding.
			tcpBindAddr := serverBindIP
			if strings.Contains(serverBindIP, ":") {
				tcpBindAddr = "[" + serverBindIP + "]"
			}

			podCmd = []string{"bash", "-c", fmt.Sprintf(
				"sleep 5; "+
					"testcmd -listen -protocol tcp -port 5001 -interface net1 -server %s -mtu %d & "+
					"testcmd -listen -protocol udp -port 5002 -interface net1 -server %s -mtu %d & "+
					"testcmd -listen -protocol sctp -port 5003 -interface net1 -server %s -mtu %d & "+
					"sleep infinity",
				tcpBindAddr, packetSize, serverBindIP, packetSize, serverBindIP, packetSize)}
		}
	} else {
		containerName = "test"
		podCmd = []string{"bash", "-c", "sleep infinity"}
	}

	container, err := pod.NewContainerBuilder(containerName, NetConfig.CnfNetTestContainer, podCmd).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	return pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		RedefineDefaultContainer(*container).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
}

// CreateTestClientPod creates a client pod with SR-IOV interface.
// This is a convenience wrapper around CreateTestPod.
func CreateTestClientPod(
	name, nodeName, networkName string, ipAddresses []string, macAddress string) (*pod.Builder, error) {
	return CreateTestPod(name, nodeName, networkName, ipAddresses, macAddress, false, "", 0)
}

// CreateTestServerPod creates a server pod with testcmd listeners for TCP, UDP, and SCTP.
// This is a convenience wrapper around CreateTestPod that also waits for testcmd to be ready.
func CreateTestServerPod(name, nodeName, networkName string, ipAddresses []string, serverBindIP, macAddress string,
	mtu int) (*pod.Builder, error) {
	serverPod, err := CreateTestPod(name, nodeName, networkName, ipAddresses, macAddress, true, serverBindIP, mtu)
	if err != nil {
		return nil, err
	}

	// Wait for testcmd listeners to be ready (they start after sleep 5 in the command).
	if err := WaitForServerReady(serverPod, tsparams.WaitTimeout); err != nil {
		return nil, fmt.Errorf("server pod %s not ready: %w", name, err)
	}

	return serverPod, nil
}

// DeleteTestPods deletes the given test pods.
func DeleteTestPods(pods ...*pod.Builder) error {
	for _, podBuilder := range pods {
		if podBuilder != nil {
			klog.V(90).Infof("Deleting pod %s", podBuilder.Definition.Name)

			_, err := podBuilder.Delete()
			if err != nil {
				return fmt.Errorf("failed to delete pod %s: %w", podBuilder.Definition.Name, err)
			}
		}
	}

	return nil
}

// CleanupTestResources deletes test pods and SR-IOV networks.
// This is a convenience function that combines DeleteTestPods and DeleteSriovNetworks.
func CleanupTestResources(networkNames []string, pods ...*pod.Builder) error {
	klog.V(90).Infof("Cleaning up test resources: %d pods, %d networks", len(pods), len(networkNames))

	if err := DeleteTestPods(pods...); err != nil {
		return fmt.Errorf("failed to delete test pods: %w", err)
	}

	if err := DeleteSriovNetworks(networkNames...); err != nil {
		return fmt.Errorf("failed to delete SR-IOV networks: %w", err)
	}

	return nil
}

// WaitForServerReady waits for the server pod's testcmd listeners to be ready.
func WaitForServerReady(serverPod *pod.Builder, timeout time.Duration) error {
	klog.V(90).Infof("Waiting for server pod %s to be ready", serverPod.Definition.Name)

	return wait.PollUntilContextTimeout(context.TODO(),
		2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := serverPod.ExecCommand([]string{"bash", "-c", "pgrep -f testcmd"})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
}

// GetPodIPFromInterface retrieves the IP address of a specific interface from a pod.
// This is useful for whereabouts IPAM where the IP is assigned dynamically.
// Returns the IP without the prefix (e.g., "2001:100::5" instead of "2001:100::5/64").
func GetPodIPFromInterface(podBuilder *pod.Builder, interfaceName string) (string, error) {
	klog.V(90).Infof("Getting IP from interface %s on pod %s", interfaceName, podBuilder.Definition.Name)

	output, err := podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("ip -o addr show %s | awk '{print $4}' | cut -d'/' -f1 | head -1", interfaceName)})
	if err != nil {
		return "", fmt.Errorf("failed to get IP from interface %s: %w", interfaceName, err)
	}

	ipAddress := strings.TrimSpace(output.String())
	if ipAddress == "" {
		return "", fmt.Errorf("no IP found on interface %s", interfaceName)
	}

	klog.V(90).Infof("Found IP %s on interface %s", ipAddress, interfaceName)

	return ipAddress, nil
}

// CleanupAllSriovResources removes all SR-IOV networks and policies.
func CleanupAllSriovResources(mcpLabel string, timeout time.Duration) error {
	klog.V(90).Infof("Cleaning up all SR-IOV resources")

	err := removeSriovNetworks()
	if err != nil {
		return err
	}

	return removeSriovPoliciesAndWaitForStability(mcpLabel, timeout)
}

// RunTrafficTest runs all traffic type tests (ICMP, TCP, UDP, SCTP) between client and server pods.
// serverIP should be the bare IP address without prefix (e.g., "192.168.10.2" or "2001:100::2").
func RunTrafficTest(clientPod *pod.Builder, serverIP string, mtu int) error {
	serverIPAddress := removePrefix(serverIP)
	packetSize := mtu - 100

	klog.V(90).Infof("Running traffic tests with MTU %d (packet size %d)", mtu, packetSize)

	// 1. ICMP connectivity.
	// ICMPConnectivityCheck expects CIDR notation, so add /32 for IPv4 or /128 for IPv6.
	serverIPWithPrefix := serverIPAddress + "/32"
	if strings.Contains(serverIPAddress, ":") {
		serverIPWithPrefix = serverIPAddress + "/128"
	}

	err := cmd.ICMPConnectivityCheck(clientPod, []string{serverIPWithPrefix})
	if err != nil {
		return fmt.Errorf("ICMP connectivity check failed: %w", err)
	}

	// 2. TCP unicast (port 5001).
	tcpCmd := fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface net1 -server %s -mtu %d",
		serverIPAddress, packetSize)
	klog.V(90).Infof("Running TCP test: %s", tcpCmd)
	tcpOutput, err := clientPod.ExecCommand([]string{"bash", "-c", tcpCmd})
	klog.V(90).Infof("TCP output: %s", tcpOutput.String())

	if err != nil {
		return fmt.Errorf("TCP connectivity check failed (output: %s): %w", tcpOutput.String(), err)
	}

	// 3. UDP unicast (port 5002).
	udpCmd := fmt.Sprintf("testcmd -protocol udp -port 5002 -interface net1 -server %s -mtu %d",
		serverIPAddress, packetSize)
	klog.V(90).Infof("Running UDP test: %s", udpCmd)
	udpOutput, err := clientPod.ExecCommand([]string{"bash", "-c", udpCmd})
	klog.V(90).Infof("UDP output: %s", udpOutput.String())

	if err != nil {
		return fmt.Errorf("UDP connectivity check failed (output: %s): %w", udpOutput.String(), err)
	}

	// 4. SCTP unicast (port 5003).
	sctpCmd := fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface net1 -server %s -mtu %d",
		serverIPAddress, packetSize)
	klog.V(90).Infof("Running SCTP test: %s", sctpCmd)
	sctpOutput, err := clientPod.ExecCommand([]string{"bash", "-c", sctpCmd})
	klog.V(90).Infof("SCTP output: %s", sctpOutput.String())

	if err != nil {
		return fmt.Errorf("SCTP connectivity check failed (output: %s): %w", sctpOutput.String(), err)
	}

	return nil
}

// EnableSCTPOnWorkers loads the SCTP kernel module on all worker nodes.
func EnableSCTPOnWorkers(workerNodes []*nodes.Builder) error {
	for _, node := range workerNodes {
		klog.V(90).Infof("Loading SCTP kernel module on node %s", node.Definition.Name)

		debugPod, err := pod.NewBuilder(
			APIClient, fmt.Sprintf("sctp-enable-%s", node.Definition.Name),
			tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
			DefineOnNode(node.Definition.Name).
			WithPrivilegedFlag().
			WithHostPid(true).
			WithHostNetwork().
			RedefineDefaultCMD([]string{"bash", "-c", "nsenter -t 1 -m -u -n -i modprobe sctp && sleep 5"}).
			CreateAndWaitUntilRunning(netparam.DefaultTimeout)
		if err != nil {
			return fmt.Errorf("failed to create SCTP enabler pod on node %s: %w", node.Definition.Name, err)
		}

		time.Sleep(10 * time.Second)

		_, err = debugPod.DeleteAndWait(netparam.DefaultTimeout)
		if err != nil {
			return fmt.Errorf("failed to delete SCTP enabler pod on node %s: %w", node.Definition.Name, err)
		}
	}

	return nil
}

// removePrefix removes the CIDR prefix from an IP address.
func removePrefix(ipWithPrefix string) string {
	for i, c := range ipWithPrefix {
		if c == '/' {
			return ipWithPrefix[:i]
		}
	}

	return ipWithPrefix
}

func removeSriovNetworks() error {
	klog.V(90).Infof("Removing all SR-IOV networks")

	sriovNetworks, err := sriov.List(APIClient, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to list SR-IOV networks: %w", err)
	}

	for _, network := range sriovNetworks {
		// Get the network name and target namespace before deletion.
		networkName := network.Definition.Name
		targetNamespace := TargetNamespaceOf(network)

		err = network.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete SR-IOV network %s: %w", networkName, err)
		}

		err = WaitForNADDeletion(networkName, targetNamespace, tsparams.DefaultTimeout)
		if err != nil {
			return fmt.Errorf("failed waiting for NAD deletion: %w", err)
		}
	}

	return nil
}

func removeSriovPoliciesAndWaitForStability(mcpLabel string, timeout time.Duration) error {
	klog.V(90).Infof("Removing all SR-IOV policies and waiting for MCP stability")

	policies, err := sriov.ListPolicy(APIClient, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to list SR-IOV policies: %w", err)
	}

	for _, policy := range policies {
		if policy.Definition.Name != "default" {
			err = policy.Delete()
			if err != nil {
				return fmt.Errorf("failed to delete SR-IOV policy %s: %w", policy.Definition.Name, err)
			}
		}
	}

	return cluster.WaitForMcpStable(APIClient, timeout, 1*time.Minute, mcpLabel)
}
