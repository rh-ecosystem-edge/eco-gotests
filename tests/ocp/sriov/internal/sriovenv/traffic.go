package sriovenv

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const networkStatusAnnotation = "k8s.v1.cni.cncf.io/network-status"

type networkStatusEntry struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	IPs       []string `json:"ips"`
	Mac       string   `json:"mac"`
}

// RemovePrefix removes the CIDR prefix from an IP address.
func RemovePrefix(ipAddr string) string {
	return strings.Split(ipAddr, "/")[0]
}

// GetPodIPFromInterface retrieves an IP address of a specific interface from a pod's network-status annotation.
// ipFamily should be "ipv4" or "ipv6". For dual-stack, call this function twice with each family.
func GetPodIPFromInterface(podBuilder *pod.Builder, interfaceName, ipFamily string) (string, error) {
	klog.V(90).Infof("Getting %s from interface %s on pod %s",
		ipFamily, interfaceName, podBuilder.Definition.Name)

	pulled, err := pod.Pull(APIClient, podBuilder.Definition.Name, podBuilder.Definition.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull pod %s: %w", podBuilder.Definition.Name, err)
	}

	annotation := pulled.Object.Annotations[networkStatusAnnotation]
	if annotation == "" {
		return "", fmt.Errorf("no network-status annotation on pod %s", pulled.Definition.Name)
	}

	var statuses []networkStatusEntry
	if err := json.Unmarshal([]byte(annotation), &statuses); err != nil {
		return "", fmt.Errorf("failed to parse network-status annotation for pod %s: %w",
			pulled.Definition.Name, err)
	}

	var ips []string

	for _, status := range statuses {
		if status.Interface == interfaceName {
			ips = status.IPs

			break
		}
	}

	if ips == nil {
		return "", fmt.Errorf("interface %s not found in network-status annotation", interfaceName)
	}

	for _, ipAddr := range ips {
		ipClean := RemovePrefix(ipAddr)
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
	clientName,
	serverName,
	clientNode,
	serverNode,
	clientNetwork,
	serverNetwork,
	serverBindIP,
	clientMAC,
	serverMAC string,
	clientIPs,
	serverIPs []string,
	mtu int,
) (*pod.Builder, *pod.Builder, error) {
	klog.V(90).Infof("Creating client pod %s and server pod %s", clientName, serverName)

	client, err := CreateTestClientPod(clientName, clientNode, clientNetwork, clientMAC, clientIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client pod: %w", err)
	}

	server, err := CreateTestServerPod(
		serverName, serverNode, serverNetwork, serverBindIP, serverMAC, serverIPs, mtu)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server pod: %w", err)
	}

	return client, server, nil
}

// CreateTestClientPod creates a client pod with SR-IOV interface.
func CreateTestClientPod(
	name,
	nodeName,
	networkName,
	macAddress string,
	ipAddresses []string,
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

	container, err := pod.NewContainerBuilder("test", SriovOcpConfig.OcpSriovTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	podBuilder, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*container).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return nil, err
	}

	return podBuilder, nil
}

// CreateTestServerPod creates a server pod with testcmd listeners for TCP, UDP, SCTP, and multicast.
func CreateTestServerPod(
	name,
	nodeName,
	networkName,
	serverBindIP,
	macAddress string,
	ipAddresses []string,
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

	container, err := pod.NewContainerBuilder("server", SriovOcpConfig.OcpSriovTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	serverPod, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*container).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return nil, err
	}

	if err := WaitForServerReady(serverPod, tsparams.DefaultTimeout); err != nil {
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
			// Match process name only. pgrep -f also matches the parent bash -c
			// command line, which already contains "testcmd" during the startup sleep.
			_, execErr := serverPod.ExecCommand([]string{
				"bash", "-c", `n=$(pgrep testcmd 2>/dev/null | wc -l); [ "$n" -ge 4 ]`,
			})
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

func getIPv4MulticastConfig(mtu int) (group, mac string) {
	if mtu > 1500 {
		return tsparams.MulticastIPv4GroupLargeMTU, tsparams.MulticastIPv4MACLargeMTU
	}

	return tsparams.MulticastIPv4Group, tsparams.MulticastIPv4MAC
}

func buildTestcmdListeners(interfaceName, serverIP, mcastGroup string, packetSize int, holdOpen bool) string {
	listeners := fmt.Sprintf(
		"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
			"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port 5003 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & ",
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, serverIP, packetSize,
		interfaceName, mcastGroup, packetSize)

	if holdOpen {
		return listeners + "sleep infinity"
	}

	return listeners
}

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

func buildDynamicServerStartScript(interfaceName string, mtu, packetSize int) string {
	discoverIP := fmt.Sprintf(
		"for _ in $(seq 1 10); do "+
			"SERVER_IP=$(ip -4 -o addr show %s 2>/dev/null | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
			"[ -n \"$SERVER_IP\" ] && break; "+
			"SERVER_IP=$(ip -6 -o addr show %s 2>/dev/null | grep -v fe80 | awk '{print $4}' | "+
			"cut -d'/' -f1 | head -1); "+
			"[ -n \"$SERVER_IP\" ] && break; "+
			"sleep 1; done; "+
			"[ -n \"$SERVER_IP\" ] || { echo 'Failed to discover server IP'; exit 1; }; "+
			"echo \"Discovered server IP: $SERVER_IP\"; ",
		interfaceName, interfaceName)

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

	listeners := buildTestcmdListeners(interfaceName, "$SERVER_IP", "$MCAST_GROUP", packetSize, false)

	return discoverIP + setupMulticast + listeners
}

func buildDynamicIPServerCommand(interfaceName string, mtu, packetSize int) []string {
	script := buildDynamicServerStartScript(interfaceName, mtu, packetSize)

	return []string{"bash", "-c", script + "sleep infinity"}
}

func buildStaticIPServerCommand(serverBindIP, interfaceName string, mtu, packetSize int) []string {
	isIPv6 := strings.Contains(serverBindIP, ":")
	multicastSetup, multicastGroup := buildMulticastSetup(isIPv6, interfaceName, mtu)

	listeners := buildTestcmdListeners(interfaceName, serverBindIP, multicastGroup, packetSize, true)

	return []string{"bash", "-c", multicastSetup + "sleep 5; " + listeners}
}

// RunTrafficTestsForBothMTUs runs traffic tests for two different MTU configurations.
// mtuSmall is typically 500 for IPv4 or 1280 for IPv6, mtuLarge is typically 9000.
func RunTrafficTestsForBothMTUs(
	clientSmallMTU,
	clientLargeMTU *pod.Builder,
	serverIP1,
	serverIP2 string,
	mtuSmall,
	mtuLarge int,
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

// RunTrafficTest runs all traffic type tests (ICMP, TCP, UDP, SCTP, multicast) between client and server pods.
// Optional interfaceName defaults to tsparams.Net1Interface when omitted or empty (e.g. bond tests use bond0).
func RunTrafficTest(clientPod *pod.Builder, serverIP string, mtu int, interfaceName ...string) error {
	iface := tsparams.Net1Interface
	if len(interfaceName) > 0 && interfaceName[0] != "" {
		iface = interfaceName[0]
	}

	klog.V(90).Infof("Running traffic tests against %s with MTU %d on interface %s", serverIP, mtu, iface)
	serverIPAddress := RemovePrefix(serverIP)

	// Subtract header overhead to fit within MTU.
	// Accounts for IP headers, protocol headers, and testcmd overhead.
	packetSize := mtu - 100

	var failedProtocols []string

	serverIPWithPrefix := serverIPAddress + "/32"
	if strings.Contains(serverIPAddress, ":") {
		serverIPWithPrefix = serverIPAddress + "/128"
	}

	if err := sriovocpenv.ICMPConnectivityCheck(
		clientPod, []string{serverIPWithPrefix}, iface); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("ICMP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			iface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("TCP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			iface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("UDP: %v", err))
	}

	if err := RunProtocolTest(clientPod, "SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface %s -server %s -mtu %d",
			iface, serverIPAddress, packetSize)); err != nil {
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
			iface, multicastGroup, packetSize)); err != nil {
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
