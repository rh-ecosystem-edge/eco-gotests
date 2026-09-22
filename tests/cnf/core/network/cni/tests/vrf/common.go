package tests

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

const (
	vrfTCPPort       = 8080
	vrfBlueInterface = "net1"
	vrfRedInterface  = "net2"
	vrfBlueName      = "blue"
	vrfRedName       = "red"

	vrfClientIPAddress = "192.168.10.10"
	vrfServerIPAddress = "192.168.10.11"

	vrfBlueClientIPv4          = "10.255.255.1"
	vrfBlueServerIPv4          = "10.255.255.2"
	vrfRedClientIPv4Overlap    = "10.255.255.3"
	vrfRedServerIPv4Overlap    = "10.255.255.4"
	vrfRedClientIPv4NonOverlap = "192.168.255.3"
	vrfRedServerIPv4NonOverlap = "192.168.255.4"

	vrfBlueClientIPv6          = "2001:100::1"
	vrfBlueServerIPv6          = "2001:100::2"
	vrfRedClientIPv6Overlap    = "2001:100::3"
	vrfRedServerIPv6Overlap    = "2001:100::4"
	vrfRedClientIPv6NonOverlap = "2001:200::3"
	vrfRedServerIPv6NonOverlap = "2001:200::4"

	vrfClientMacAddressBlue = "20:04:0f:f1:88:A1"
	vrfClientMacAddressRed  = "20:04:0f:f1:88:B2"
	vrfServerMacAddressBlue = "20:04:0f:f1:88:A3"
	vrfServerMacAddressRed  = "20:04:0f:f1:88:B4"

	dhcpServerIPSameNode = "10.255.255.201"
	dhcpServerIPDiffNode = "10.255.255.202"
	dhcpVolumeName       = "dhcp"

	dummyDhcpNetworkName = "dummy-dhcp-network"
	dhcpDaemonName       = "dhcp-daemon"
)

// vrfNetConfig holds per-VRF network configuration for validation and connectivity checks.
type vrfNetConfig struct {
	VrfInterface string
	VrfName      string
	IPAddr       string
	NetPrefix    string
	Mac          string
}

// ensureDhcpDaemon configures the cluster network operator to deploy the CNI DHCP IPAM daemon.
// cnf-gotests clusters are provisioned with dummy-dhcp-network via setup-test-cluster.sh;
// without it the dhcp IPAM plugin cannot dial /run/cni/dhcp.sock on the node.
// Returns true if it added the dummy-dhcp-network, false if it already existed.
func ensureDhcpDaemon() bool {
	By("Ensuring CNI DHCP IPAM daemon is deployed on cluster nodes")

	clusterNetwork, err := cluster.GetOCPNetworkOperatorConfig(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to get network.operator config")

	addedShimNetwork := false

	if !hasDhcpShimNetwork(clusterNetwork.Object) {
		clusterNetwork.Definition.Spec.AdditionalNetworks = append(
			clusterNetwork.Definition.Spec.AdditionalNetworks,
			operatorv1.AdditionalNetworkDefinition{
				Name: dummyDhcpNetworkName,
				Type: operatorv1.NetworkTypeSimpleMacvlan,
				SimpleMacvlanConfig: &operatorv1.SimpleMacvlanConfig{
					Master: "lo",
					Mode:   operatorv1.MacvlanModeBridge,
					MTU:    1500,
					IPAMConfig: &operatorv1.IPAMConfig{
						Type: operatorv1.IPAMTypeDHCP,
					},
				},
			},
		)

		_, err = clusterNetwork.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to patch network.operator with dummy-dhcp-network")

		err = clusterNetwork.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeAvailable, tsparams.DefaultTimeout, operatorv1.ConditionTrue)
		Expect(err).ToNot(HaveOccurred(), "network.operator not available after dhcp shim patch")

		addedShimNetwork = true
	}

	By("Waiting for dhcp-daemon DaemonSet to be created")

	var dhcpDs *daemonset.Builder

	Eventually(func() error {
		dhcpDs, err = daemonset.Pull(APIClient, dhcpDaemonName, NetConfig.MultusNamesapce)

		return err
	}, tsparams.DefaultTimeout, 5*time.Second).Should(Succeed(),
		"dhcp-daemon DaemonSet was not created in openshift-multus")

	By("Waiting for dhcp-daemon to become ready on all nodes")

	Expect(dhcpDs.IsReady(tsparams.DefaultTimeout)).To(BeTrue(), "dhcp-daemon is not ready on all nodes")

	return addedShimNetwork
}

// removeDhcpShimNetwork removes the dummy-dhcp-network from the cluster network operator configuration.
// This should only be called if ensureDhcpDaemon returned true, indicating this test suite added it.
func removeDhcpShimNetwork() {
	By("Removing dummy-dhcp-network from cluster network operator")

	clusterNetwork, err := cluster.GetOCPNetworkOperatorConfig(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to get network.operator config")

	additionalNetworks := clusterNetwork.Definition.Spec.AdditionalNetworks[:0]
	for _, additionalNetwork := range clusterNetwork.Definition.Spec.AdditionalNetworks {
		if additionalNetwork.Name != dummyDhcpNetworkName {
			additionalNetworks = append(additionalNetworks, additionalNetwork)
		}
	}

	clusterNetwork.Definition.Spec.AdditionalNetworks = additionalNetworks

	_, err = clusterNetwork.Update()
	Expect(err).ToNot(HaveOccurred(), "Failed to remove dummy-dhcp-network")

	err = clusterNetwork.WaitUntilInCondition(
		operatorv1.OperatorStatusTypeAvailable, tsparams.DefaultTimeout, operatorv1.ConditionTrue)
	Expect(err).ToNot(HaveOccurred(), "network.operator not available after dhcp shim removal")
}

// hasDhcpShimNetwork checks if the cluster network operator has configured any
// AdditionalNetwork with DHCP IPAM, which is required for the DHCP daemon to function.
// It checks for: (1) dummy-dhcp-network, (2) RawCNIConfig with "type":"dhcp", or
// (3) SimpleMacvlanConfig with IPAMTypeDHCP.
func hasDhcpShimNetwork(network *operatorv1.Network) bool {
	for _, additionalNetwork := range network.Spec.AdditionalNetworks {
		if additionalNetwork.Name == dummyDhcpNetworkName {
			return true
		}

		if additionalNetwork.Type == operatorv1.NetworkTypeRaw &&
			strings.Contains(additionalNetwork.RawCNIConfig, `"type": "dhcp"`) {
			return true
		}

		if additionalNetwork.SimpleMacvlanConfig != nil &&
			additionalNetwork.SimpleMacvlanConfig.IPAMConfig != nil &&
			additionalNetwork.SimpleMacvlanConfig.IPAMConfig.Type == operatorv1.IPAMTypeDHCP {
			return true
		}
	}

	return false
}

// WithVRFDhcpIpam returns an option that sets DHCP IPAM on the SriovNetwork.
// This overwrites any existing IPAM configuration. Requires the CNI DHCP daemon
// to be deployed on cluster nodes (see ensureDhcpDaemon).
func WithVRFDhcpIpam() sriov.NetworkAdditionalOptions {
	return func(builder *sriov.NetworkBuilder) (*sriov.NetworkBuilder, error) {
		builder.Definition.Spec.IPAM = `{ "type": "dhcp" }`

		return builder, nil
	}
}

// WithVRFMetaPlugin returns an option that sets VRF meta plugin config on the SriovNetwork.
func WithVRFMetaPlugin(vrfName string) sriov.NetworkAdditionalOptions {
	return func(builder *sriov.NetworkBuilder) (*sriov.NetworkBuilder, error) {
		if vrfName == "" {
			return builder, fmt.Errorf("vrfName cannot be empty")
		}

		builder.Definition.Spec.MetaPluginsConfig = fmt.Sprintf(`{"type":"vrf","vrfName":"%s"}`, vrfName)

		return builder, nil
	}
}

func buildVRFServerCommand() string {
	return fmt.Sprintf(
		"testcmd --protocol=tcp --listen --mtu=100 --port=%d --interface=eth0 & "+
			"testcmd --protocol=tcp --listen --mtu=100 --port=%d --interface=%s & "+
			"testcmd --protocol=tcp --listen --mtu=100 --port=%d --interface=%s & sleep INF",
		vrfTCPPort, vrfTCPPort, vrfBlueInterface, vrfTCPPort, vrfRedInterface)
}

func buildOverlapTempPodCommand() []string {
	return []string{
		"testcmd", "--protocol=tcp", "--interface=eth0", "--listen", "--mtu=100",
		fmt.Sprintf("--port=%d", vrfTCPPort),
	}
}

func podHasCorrectVrfConfig(testPod *pod.Builder, vrfNetConfigs []vrfNetConfig) {
	for _, vrfMapConfig := range vrfNetConfigs {
		validateVrfIPAddrCommand := []string{"ip", "addr", "show", vrfMapConfig.VrfInterface}
		validateVRFRouteTableCommand := []string{"ip", "route", "show", "vrf", vrfMapConfig.VrfName}

		if strings.Contains(vrfMapConfig.IPAddr, ":") {
			validateVrfIPAddrCommand = appendAtIndex(validateVrfIPAddrCommand, "-6", 1)
			validateVRFRouteTableCommand = appendAtIndex(validateVRFRouteTableCommand, "-6", 1)
		}

		Eventually(func() bool {
			output, _ := testPod.ExecCommand(validateVrfIPAddrCommand)

			return strings.Contains(output.String(), vrfMapConfig.IPAddr)
		}, tsparams.DefaultTimeout, 5*time.Second).Should(BeTrue(), "VRF interface is not present")

		var ipnet *net.IPNet

		if strings.Contains(vrfMapConfig.IPAddr, ":") {
			var parseErr error

			_, ipnet, parseErr = net.ParseCIDR(vrfMapConfig.IPAddr + "/" + vrfMapConfig.NetPrefix)
			Expect(parseErr).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to parse CIDR %s/%s", vrfMapConfig.IPAddr, vrfMapConfig.NetPrefix))
		}

		Eventually(func() bool {
			output, _ := testPod.ExecCommand(validateVRFRouteTableCommand)

			if ipnet != nil {
				return strings.Contains(output.String(), ipnet.String())
			}

			return strings.Contains(output.String(), vrfMapConfig.IPAddr)
		}, tsparams.DefaultTimeout, 5*time.Second).Should(BeTrue(),
			fmt.Sprintf("VRF %s route table is not present", vrfMapConfig.VrfName))
	}
}

func connectivityViaIcmpAndHTTP(clientPod *pod.Builder, serverNetConfig []vrfNetConfig, negative bool) {
	By("Validating client/server ICMP and TCP VRF connectivity")

	for _, netConfig := range serverNetConfig {
		err := pingIPViaVRF(clientPod, netConfig.VrfName, netConfig.IPAddr, negative)
		Expect(err).ToNot(HaveOccurred(), "icmp test failed")

		err = httpViaVRF(clientPod, netConfig.VrfName, netConfig.IPAddr, negative)
		Expect(err).ToNot(HaveOccurred(), "http test failed")
	}
}

func connectivityViaEth0(clientPod *pod.Builder, destIP string, negative bool) {
	err := pingIPViaVRF(clientPod, "eth0", destIP, negative)
	Expect(err).ToNot(HaveOccurred(), "icmp test via eth0 failed")

	err = httpViaVRF(clientPod, "eth0", destIP, negative)
	Expect(err).ToNot(HaveOccurred(), "http test via eth0 failed")
}

func pingIPViaVRF(clientPod *pod.Builder, vrfName, destIP string, negative bool) error {
	command := []string{"testcmd", "-interface", vrfName, "-server", destIP, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := clientPod.ExecCommand(command)

	return err
}

func httpViaVRF(clientPod *pod.Builder, vrfName, destIP string, negative bool) error {
	command := []string{
		"testcmd",
		fmt.Sprintf("--interface=%s", vrfName),
		fmt.Sprintf("--server=%s", destIP),
		"--protocol=tcp",
		"--mtu=100",
		fmt.Sprintf("--port=%d", vrfTCPPort),
	}
	if negative {
		command = append(command, "--negative")
	}

	_, err := clientPod.ExecCommand(command)

	return err
}

func appendAtIndex(slice []string, element string, index int) []string {
	slice = append(slice[:index+1], slice[index:]...)
	slice[index] = element

	return slice
}

func netPrefixForIP(ipAddr string) string {
	if strings.Contains(ipAddr, ":") {
		return "64"
	}

	return "24"
}

func defineVRFIPConfig(vrfInterface, vrfName, ipAddr string) vrfNetConfig {
	return vrfNetConfig{
		VrfInterface: vrfInterface,
		VrfName:      vrfName,
		IPAddr:       ipAddr,
		NetPrefix:    netPrefixForIP(ipAddr),
	}
}

func defineClientServerVRFsIPConfig(ipOverlap bool, ipStack string) ([]vrfNetConfig, []vrfNetConfig) {
	var blueClientIP, blueServerIP, redClientIP, redServerIP string

	if ipStack == netparam.IPV6Family {
		blueClientIP = vrfBlueClientIPv6
		blueServerIP = vrfBlueServerIPv6
		redClientIP = vrfRedClientIPv6Overlap
		redServerIP = vrfRedServerIPv6Overlap

		if !ipOverlap {
			redClientIP = vrfRedClientIPv6NonOverlap
			redServerIP = vrfRedServerIPv6NonOverlap
		}
	} else {
		blueClientIP = vrfBlueClientIPv4
		blueServerIP = vrfBlueServerIPv4
		redClientIP = vrfRedClientIPv4Overlap
		redServerIP = vrfRedServerIPv4Overlap

		if !ipOverlap {
			redClientIP = vrfRedClientIPv4NonOverlap
			redServerIP = vrfRedServerIPv4NonOverlap
		}
	}

	clientNetConfig := []vrfNetConfig{
		defineVRFIPConfig(vrfBlueInterface, vrfBlueName, blueClientIP),
		defineVRFIPConfig(vrfRedInterface, vrfRedName, redClientIP),
	}
	serverNetConfig := []vrfNetConfig{
		defineVRFIPConfig(vrfBlueInterface, vrfBlueName, blueServerIP),
		defineVRFIPConfig(vrfRedInterface, vrfRedName, redServerIP),
	}

	return clientNetConfig, serverNetConfig
}

func defineClientServerVRFsIPOverlapWithStaticMac() ([]vrfNetConfig, []vrfNetConfig) {
	clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, netparam.IPV4Family)
	clientNetConfig[0].Mac = vrfClientMacAddressBlue
	clientNetConfig[1].Mac = vrfClientMacAddressRed
	serverNetConfig[0].Mac = vrfServerMacAddressBlue
	serverNetConfig[1].Mac = vrfServerMacAddressRed

	return clientNetConfig, serverNetConfig
}

func buildVRFNetworksWithMac(
	blueNetName, redNetName string, netConfig []vrfNetConfig,
) []*types.NetworkSelectionElement {
	return []*types.NetworkSelectionElement{
		{
			Name:       blueNetName,
			MacRequest: netConfig[0].Mac,
		},
		{
			Name:       redNetName,
			MacRequest: netConfig[1].Mac,
		},
	}
}

func buildVRFNetworks(blueNetName, redNetName string, netConfig []vrfNetConfig) []*types.NetworkSelectionElement {
	return []*types.NetworkSelectionElement{
		{
			Name:      blueNetName,
			IPRequest: []string{fmt.Sprintf("%s/%s", netConfig[0].IPAddr, netConfig[0].NetPrefix)},
		},
		{
			Name:      redNetName,
			IPRequest: []string{fmt.Sprintf("%s/%s", netConfig[1].IPAddr, netConfig[1].NetPrefix)},
		},
	}
}

func defineClientServerVRFsIPOverlapConfig(
	workerNodeList []*nodes.Builder,
	sameNode bool,
) ([]vrfNetConfig, []vrfNetConfig) {
	clientPodNode := workerNodeList[0].Object.Name
	serverPodNode := clientPodNode

	if !sameNode {
		if len(workerNodeList) < 2 {
			Skip("not enough nodes to run DiffNode test")
		}

		serverPodNode = workerNodeList[1].Object.Name
	}

	redNetPrefix := "23"
	if !sameNode {
		redNetPrefix = "8"
	}

	By("Create 2 pods and fetch their IPs")

	tempPod1, err := pod.NewBuilder(
		APIClient,
		"temp-pod1",
		tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).
		WithCommand(buildOverlapTempPodCommand()).
		WithNodeSelector(map[string]string{corev1.LabelHostname: clientPodNode}).
		CreateAndWaitUntilRunning(tsparams.PodStartTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create temp pod1")

	// Note: Leaving temp pods running to keep cluster IPs assigned for DiffNode eth0 checks

	tempPod2, err := pod.NewBuilder(
		APIClient,
		"temp-pod2",
		tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).
		WithCommand(buildOverlapTempPodCommand()).
		WithNodeSelector(map[string]string{corev1.LabelHostname: serverPodNode}).
		CreateAndWaitUntilRunning(tsparams.PodStartTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create temp pod2")

	// Note: Leaving temp pods running to keep cluster IPs assigned for DiffNode eth0 checks

	Expect(tempPod1.Exists()).To(BeTrue(), "temp pod1 disappeared after creation")
	Expect(tempPod2.Exists()).To(BeTrue(), "temp pod2 disappeared after creation")

	Expect(tempPod1.Object.Status.PodIP).ToNot(BeEmpty(), "Temp pod1 has no assigned IP")
	tempPod1IP := tempPod1.Object.Status.PodIP

	Expect(tempPod2.Object.Status.PodIP).ToNot(BeEmpty(), "Temp pod2 has no assigned IP")
	tempPod2IP := tempPod2.Object.Status.PodIP

	clientNetConfig := []vrfNetConfig{
		{VrfInterface: vrfBlueInterface, VrfName: vrfBlueName, IPAddr: vrfClientIPAddress, NetPrefix: "24"},
		{VrfInterface: vrfRedInterface, VrfName: vrfRedName, IPAddr: tempPod1IP, NetPrefix: redNetPrefix},
	}
	serverNetConfig := []vrfNetConfig{
		{VrfInterface: vrfBlueInterface, VrfName: vrfBlueName, IPAddr: vrfServerIPAddress, NetPrefix: "24"},
		{VrfInterface: vrfRedInterface, VrfName: vrfRedName, IPAddr: tempPod2IP, NetPrefix: redNetPrefix},
	}

	return clientNetConfig, serverNetConfig
}

// networkBuilderFunc is a function type that builds network selection elements for VRF pods.
type networkBuilderFunc func(blueNetName, redNetName string, netConfig []vrfNetConfig) []*types.NetworkSelectionElement

// runVRFScenario runs a VRF scenario with optional DHCP server setup.
// If setupDhcp is true, it will run the DHCP server before creating pods and use MAC-based network attachments.
// If setupDhcp is false, it will use static IP-based network attachments.
func runVRFScenario(
	workerNodeList []*nodes.Builder,
	sameNode bool,
	redNetName, blueNetName string,
	clientNetConfig, serverNetConfig []vrfNetConfig,
	setupDhcp bool,
) *pod.Builder {
	clientPodNode := workerNodeList[0].Object.Name
	serverPodNode := clientPodNode

	if !sameNode {
		if len(workerNodeList) < 2 {
			Skip("not enough nodes to run DiffNode test")
		}

		serverPodNode = workerNodeList[1].Object.Name
	}

	// Determine which network builder function to use
	var networkBuilder networkBuilderFunc

	if setupDhcp {
		runDHCPServer(clientNetConfig, serverNetConfig, sameNode, clientPodNode, serverPodNode)

		networkBuilder = buildVRFNetworksWithMac
	} else {
		networkBuilder = buildVRFNetworks
	}

	By("Creating a client pod")

	clientPod, err := pod.NewBuilder(APIClient, "client-pod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(networkBuilder(blueNetName, redNetName, clientNetConfig)).
		WithPrivilegedFlag().
		WithNodeSelector(map[string]string{corev1.LabelHostname: clientPodNode}).
		CreateAndWaitUntilRunning(tsparams.PodStartTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	By("Creating a server pod")

	serverPod, err := pod.NewBuilder(APIClient, "server-pod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(networkBuilder(blueNetName, redNetName, serverNetConfig)).
		WithPrivilegedFlag().
		RedefineDefaultCMD([]string{"/bin/bash", "-c", buildVRFServerCommand()}).
		WithNodeSelector(map[string]string{corev1.LabelHostname: serverPodNode}).
		CreateAndWaitUntilRunning(tsparams.PodStartTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(clientPod, clientNetConfig)
	podHasCorrectVrfConfig(serverPod, serverNetConfig)

	connectivityViaIcmpAndHTTP(clientPod, serverNetConfig, false)

	if serverNetConfig[1].NetPrefix == "8" {
		By("Validating client/server ICMP and HTTP via eth0")
		connectivityViaEth0(clientPod, serverNetConfig[1].IPAddr, false)
	}

	_, err = serverPod.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "failed to remove server pod")

	By("Validating client/server ICMP and HTTP negative test")
	connectivityViaIcmpAndHTTP(clientPod, serverNetConfig, true)

	return clientPod
}

func runDHCPServer(
	clientNetConfig, serverNetConfig []vrfNetConfig,
	sameNode bool,
	clientNode, serverNode string,
) {
	By("Running DHCP server pods")

	addressMap := map[string]string{
		vrfClientMacAddressBlue: clientNetConfig[0].IPAddr,
		vrfServerMacAddressBlue: serverNetConfig[0].IPAddr,
		vrfClientMacAddressRed:  clientNetConfig[1].IPAddr,
		vrfServerMacAddressRed:  serverNetConfig[1].IPAddr,
	}

	macVlanInterface, err := getMacVlanInterfaceForDHCP(clientNode)
	Expect(err).ToNot(HaveOccurred(), "Failed to discover macvlan interface for DHCP server")

	err = createDHCPServerOnNode(clientNode, macVlanInterface, dhcpServerIPSameNode, addressMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to create DHCP server on client node")

	if !sameNode {
		err = createDHCPServerOnNode(serverNode, macVlanInterface, dhcpServerIPDiffNode, addressMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DHCP server on server node")
	}
}

func getMacVlanInterfaceForDHCP(nodeName string) (string, error) {
	interfaces, err := NetConfig.GetSriovInterfaces(1)
	if err != nil {
		return "", fmt.Errorf("failed to read SR-IOV interfaces from environment: %w", err)
	}

	if len(interfaces) == 0 {
		return "", fmt.Errorf("no SR-IOV interfaces configured for DHCP server")
	}

	hostPod, err := pod.NewBuilder(
		APIClient, "dhcp-host-ifdisc", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithHostNetwork().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to create host network pod on node %s: %w", nodeName, err)
	}

	defer func() {
		_, _ = hostPod.DeleteAndWait(tsparams.DefaultTimeout)
	}()

	output, err := hostPod.ExecCommand([]string{"ip", "-o", "link", "show", interfaces[0]})
	if err != nil {
		return "", fmt.Errorf("failed to verify interface %s on node %s: %w", interfaces[0], nodeName, err)
	}

	if !strings.Contains(output.String(), interfaces[0]) {
		return "", fmt.Errorf("interface %s is not present on node %s", interfaces[0], nodeName)
	}

	return interfaces[0], nil
}

// createDHCPServerOnNode creates a DHCP server pod on the specified node with a dedicated macvlan NAD.
// The NAD is created in the test namespace and will be cleaned up automatically when the namespace
// is deleted in AfterSuite. NAD names are derived from the DHCP server IP to ensure uniqueness.
func createDHCPServerOnNode(nodeName, macVlanInterface, serverIP string, addressMap map[string]string) error {
	// Create unique NAD name based on server IP (e.g., "dhcp-10-255-255-201")
	nadName := fmt.Sprintf("dhcp-%s", strings.ReplaceAll(serverIP, ".", "-"))

	// Build macvlan master plugin with static IPAM for DHCP server NAD
	macvlanPlugin := nad.NewMasterMacVlanPlugin("macvlan-vrf").
		WithMasterInterface(macVlanInterface).
		WithMode("bridge").
		WithIPAM(nad.IPAMStatic())

	masterPluginConfig, err := macvlanPlugin.GetMasterPluginConfig()
	if err != nil {
		return fmt.Errorf("failed to build macvlan plugin config: %w", err)
	}

	// Create NAD using eco-goinfra builder
	dhcpNAD, err := nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPluginConfig).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create DHCP NAD: %w", err)
	}

	ipAddr, subnet, err := net.ParseCIDR(fmt.Sprintf("%s/24", serverIP))
	if err != nil {
		return fmt.Errorf("failed to parse DHCP server IP %s: %w", serverIP, err)
	}

	broadcastAddr, err := lastIPv4Addr(subnet)
	if err != nil {
		return err
	}

	_, err = createDHCPServerPod(dhcpNAD.Object.Name, nodeName, ipAddr, subnet, broadcastAddr, addressMap)
	if err != nil {
		return fmt.Errorf("failed to create DHCP server pod: %w", err)
	}

	return nil
}

func createDHCPServerPod(
	networkName, nodeName string,
	serverIP net.IP, subnet *net.IPNet, broadcastAddr net.IP,
	addressMap map[string]string,
) (*pod.Builder, error) {
	hosts := ""
	index := 0

	for macAddr, ipAddr := range addressMap {
		hosts += fmt.Sprintf(
			" host test%d \"{hardware ethernet %s; fixed-address %s; max-lease-time 7200;}\"",
			index, macAddr, ipAddr)
		index++
	}

	initCommand := fmt.Sprintf(
		"echo subnet %s netmask %s \"{option broadcast-address %s; default-lease-time 3600; "+
			"allow duplicates; max-lease-time 7200; interface net1;}\"%s > /etc/dhcp/dhcpd.conf",
		subnet.IP, net.IP(subnet.Mask).String(), broadcastAddr, hosts)

	initContainer, err := pod.NewContainerBuilder(
		"initcontainer", NetConfig.CnfNetTestContainer, []string{"bash", "-c", initCommand}).
		WithSecurityCapabilities([]string{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true).
		WithVolumeMount(corev1.VolumeMount{Name: dhcpVolumeName, MountPath: "/etc/dhcp/"}).
		GetContainerCfg()
	if err != nil {
		return nil, err
	}

	privileged := true
	initContainer.SecurityContext.Privileged = &privileged

	mainContainer, err := pod.NewContainerBuilder(
		"test", NetConfig.CnfNetTestContainer,
		[]string{"/bin/bash", "-c", "/usr/sbin/dhcpd -cf /etc/dhcp/dhcpd.conf && sleep infinity"}).
		WithVolumeMount(corev1.VolumeMount{Name: dhcpVolumeName, MountPath: "/etc/dhcp/"}).
		GetContainerCfg()
	if err != nil {
		return nil, err
	}

	mainContainer.SecurityContext = &corev1.SecurityContext{Privileged: &privileged}

	dhcpPod, err := pod.NewBuilder(
		APIClient,
		fmt.Sprintf("dhcp-server-%s", serverIP.String()),
		tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithAdditionalInitContainer(initContainer).
		RedefineDefaultContainer(*mainContainer).
		WithVolume(corev1.Volume{
			Name: dhcpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}).
		WithSecondaryNetwork(pod.StaticIPAnnotation(networkName, []string{fmt.Sprintf("%s/24", serverIP)})).
		WithPrivilegedFlag().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return nil, err
	}

	return dhcpPod, nil
}

func lastIPv4Addr(network *net.IPNet) (net.IP, error) {
	if network.IP.To4() == nil {
		return net.IP{}, fmt.Errorf("lastIPv4Addr does not support IPv6 addresses (got %s)", network.IP)
	}

	ip := make(net.IP, len(network.IP.To4()))
	binary.BigEndian.PutUint32(
		ip, binary.BigEndian.Uint32(network.IP.To4())|^binary.BigEndian.Uint32(net.IP(network.Mask).To4()))

	return ip, nil
}
