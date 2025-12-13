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

// CreateTestClientPod creates a client pod with SR-IOV interface.
// ipAddresses can be a single IP or multiple IPs for dual-stack (e.g., []string{"192.168.1.1/24", "2001::1/64"}).
func CreateTestClientPod(
	name, nodeName, networkName string, ipAddresses []string, macAddress string) (*pod.Builder, error) {
	klog.V(90).Infof("Creating client pod %s on node %s", name, nodeName)

	secNetwork := []*types.NetworkSelectionElement{
		{
			Name:       networkName,
			MacRequest: macAddress,
			IPRequest:  ipAddresses,
		},
	}

	// Use a proper sleep command (default uses "sleep INF" which is invalid).
	clientCmd := []string{"bash", "-c", "sleep infinity"}

	clientContainer, err := pod.NewContainerBuilder("test", NetConfig.CnfNetTestContainer, clientCmd).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create client container config: %w", err)
	}

	return pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		RedefineDefaultContainer(*clientContainer).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
}

// CreateTestServerPod creates a server pod with testcmd listeners for TCP, UDP, and SCTP.
// ipAddresses can be a single IP or multiple IPs for dual-stack (e.g., []string{"192.168.1.1/24", "2001::1/64"}).
// serverBindIP is the specific IP (without prefix) that testcmd should bind to for listening.
func CreateTestServerPod(name, nodeName, networkName string, ipAddresses []string, serverBindIP, macAddress string,
	mtu int) (*pod.Builder, error) {
	klog.V(90).Infof("Creating server pod %s on node %s with MTU %d, bindIP %s", name, nodeName, mtu, serverBindIP)

	// Use mtu-100 for packet size to match client (accounting for headers).
	packetSize := mtu - 100
	secNetwork := []*types.NetworkSelectionElement{
		{
			Name:       networkName,
			MacRequest: macAddress,
			IPRequest:  ipAddresses,
		},
	}

	// Run all listeners in one container using background processes.
	// sleep 5 gives network time to be ready before starting listeners.
	// For IPv6, we use [ip] format for binding. For IPv4, just the IP.
	bindAddr := serverBindIP
	if strings.Contains(serverBindIP, ":") {
		// IPv6 address - wrap in brackets for testcmd
		bindAddr = "[" + serverBindIP + "]"
	}

	serverCmd := []string{"bash", "-c", fmt.Sprintf(
		"sleep 5; "+
			"testcmd --listen --protocol=tcp --port=5001 --interface=net1 --server=%s --mtu=%d & "+
			"testcmd --listen --protocol=udp --port=5002 --interface=net1 --server=%s --mtu=%d & "+
			"testcmd --listen --protocol=sctp --port=5003 --interface=net1 --server=%s --mtu=%d & "+
			"sleep infinity",
		bindAddr, packetSize, bindAddr, packetSize, bindAddr, packetSize)}

	serverContainer, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, serverCmd).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create server container config: %w", err)
	}

	return pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		RedefineDefaultContainer(*serverContainer).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
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
func RunTrafficTest(clientPod *pod.Builder, serverIP string, mtu int) error {
	serverIPAddress := removePrefix(serverIP)
	packetSize := mtu - 100

	klog.V(90).Infof("Running traffic tests with MTU %d (packet size %d)", mtu, packetSize)

	// 1. ICMP connectivity.
	err := cmd.ICMPConnectivityCheck(clientPod, []string{serverIP})
	if err != nil {
		return fmt.Errorf("ICMP connectivity check failed: %w", err)
	}

	// 2. TCP unicast (port 5001).
	_, err = clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("testcmd --protocol=tcp --port=5001 --interface=net1 --server=%s --mtu=%d",
			serverIPAddress, packetSize)})
	if err != nil {
		return fmt.Errorf("TCP connectivity check failed: %w", err)
	}

	// 3. UDP unicast (port 5002).
	_, err = clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("testcmd --protocol=udp --port=5002 --interface=net1 --server=%s --mtu=%d",
			serverIPAddress, packetSize)})
	if err != nil {
		return fmt.Errorf("UDP connectivity check failed: %w", err)
	}

	// 4. SCTP unicast (port 5003).
	_, err = clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("testcmd --protocol=sctp --port=5003 --interface=net1 --server=%s --mtu=%d",
			serverIPAddress, packetSize)})
	if err != nil {
		return fmt.Errorf("SCTP connectivity check failed: %w", err)
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
