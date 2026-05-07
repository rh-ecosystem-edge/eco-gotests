package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// waitForDualStackServerReady waits for all 6 dual-stack testcmd listeners to be ready.
// It requires at least 6 listeners; more are accepted (e.g. an extra process from the container image).
func waitForDualStackServerReady(serverPod *pod.Builder, timeout time.Duration) error {
	klog.V(90).Infof("Waiting for dual-stack server pod %s to be ready", serverPod.Definition.Name)

	const minListeners = 6

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.RetryInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			output, execErr := serverPod.ExecCommand([]string{"bash", "-c",
				"pgrep -c -f 'testcmd -listen'"})
			if execErr != nil {
				klog.V(90).Infof("Listeners not ready on pod %s: %v", serverPod.Definition.Name, execErr)

				return false, nil
			}

			countStr := strings.TrimSpace(output.String())

			var count int

			if _, parseErr := fmt.Sscanf(countStr, "%d", &count); parseErr != nil {
				klog.V(90).Infof("Invalid listener count %q on pod %s", countStr, serverPod.Definition.Name)

				return false, nil
			}

			if count < minListeners {
				klog.V(90).Infof("Only %d/%d testcmd listeners ready on pod %s",
					count, minListeners, serverPod.Definition.Name)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("dual-stack listeners not ready on pod %s: %w", serverPod.Definition.Name, err)
	}

	return nil
}

// buildDualStackServerCommand builds the server command for dual-stack pods that have both IPv4 and IPv6 addresses.
// It starts listeners for both families: TCP/UDP are shared, SCTP and multicast use separate ports per family.
func buildDualStackServerCommand(ipv4BindIP, ipv6BindIP, interfaceName string, mtu int) []string {
	klog.V(90).Infof("Building dual-stack server command for interface %s with MTU %d, ipv4=%q, ipv6=%q",
		interfaceName, mtu, ipv4BindIP, ipv6BindIP)

	packetSize := mtu - 100

	if ipv4BindIP == "" && ipv6BindIP == "" {
		return buildDynamicDualStackServerCommand(interfaceName, mtu, packetSize)
	}

	ipv4McastSetup, ipv4McastGroup := sriovenv.BuildMulticastSetup(false, interfaceName, mtu)
	ipv6McastSetup, ipv6McastGroup := sriovenv.BuildMulticastSetup(true, interfaceName, mtu)

	listeners := fmt.Sprintf(
		"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
			"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port 5003 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port %d -interface %s -server %s -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port %d -interface %s -server %s -mtu %d & "+
			"sleep infinity",
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, ipv4BindIP, packetSize,
		interfaceName, ipv4McastGroup, packetSize,
		tsparams.DualStackSCTPv6Port, interfaceName, ipv6BindIP, packetSize,
		tsparams.DualStackMulticastV6Port, interfaceName, ipv6McastGroup, packetSize)

	return []string{"bash", "-c", ipv4McastSetup + ipv6McastSetup + "sleep 5; " + listeners}
}

func buildDynamicDualStackServerCommand(interfaceName string, mtu, packetSize int) []string {
	ipv4McastSetup, ipv4McastGroup := sriovenv.BuildMulticastSetup(false, interfaceName, mtu)
	ipv6McastSetup, ipv6McastGroup := sriovenv.BuildMulticastSetup(true, interfaceName, mtu)

	discoverIPs := fmt.Sprintf(
		"for _ in $(seq 1 10); do "+
			"IPV4=$(ip -4 -o addr show %s 2>/dev/null | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
			"IPV6=$(ip -6 -o addr show %s 2>/dev/null | grep -v fe80 | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
			"[ -n \"$IPV4\" ] && [ -n \"$IPV6\" ] && break; "+
			"sleep 1; done; "+
			"[ -n \"$IPV4\" ] || { echo 'Failed to discover IPv4'; exit 1; }; "+
			"[ -n \"$IPV6\" ] || { echo 'Failed to discover IPv6'; exit 1; }; "+
			"echo \"Discovered IPv4: $IPV4, IPv6: $IPV6\"; ",
		interfaceName, interfaceName)

	listeners := fmt.Sprintf(
		"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
			"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port 5003 -interface %s -server $IPV4 -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port %d -interface %s -server $IPV6 -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port %d -interface %s -server %s -mtu %d & "+
			"sleep infinity",
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, ipv4McastGroup, packetSize,
		tsparams.DualStackSCTPv6Port, interfaceName, packetSize,
		tsparams.DualStackMulticastV6Port, interfaceName, ipv6McastGroup, packetSize)

	waitForDAD := "sleep 3; "

	return []string{"bash", "-c", discoverIPs + ipv4McastSetup + ipv6McastSetup + waitForDAD + listeners}
}

// RunDualStackTrafficTest runs all traffic tests for both IPv4 and IPv6 against a dual-stack server pod.
func RunDualStackTrafficTest(clientPod *pod.Builder, serverIPv4, serverIPv6 string, mtu int) error {
	klog.V(90).Infof("Running dual-stack traffic tests against IPv4=%s, IPv6=%s with MTU %d",
		serverIPv4, serverIPv6, mtu)

	ipv4Addr := ipaddr.RemovePrefix(serverIPv4)
	ipv6Addr := ipaddr.RemovePrefix(serverIPv6)
	packetSize := mtu - 100

	var failedProtocols []string

	if err := cmd.ICMPConnectivityCheck(
		clientPod, []string{ipv4Addr + "/32"}, tsparams.Net1Interface); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv4 ICMP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv4 TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv4Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv4 TCP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv4 UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv4Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv4 UDP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv4 SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv4Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv4 SCTP: %v", err))
	}

	ipv4McastGroup := tsparams.MulticastIPv4Group

	if mtu == 9000 {
		ipv4McastGroup = tsparams.MulticastIPv4GroupLargeMTU
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv4 multicast",
		fmt.Sprintf("testcmd -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv4McastGroup, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv4 multicast: %v", err))
	}

	if err := cmd.ICMPConnectivityCheck(
		clientPod, []string{ipv6Addr + "/128"}, tsparams.Net1Interface); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv6 ICMP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv6 TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv6Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv6 TCP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv6 UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, ipv6Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv6 UDP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv6 SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port %d -interface %s -server %s -mtu %d",
			tsparams.DualStackSCTPv6Port, tsparams.Net1Interface, ipv6Addr, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv6 SCTP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "IPv6 multicast",
		fmt.Sprintf("testcmd -multicast -protocol udp -port %d -interface %s -server %s -mtu %d",
			tsparams.DualStackMulticastV6Port, tsparams.Net1Interface, tsparams.MulticastIPv6Group, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("IPv6 multicast: %v", err))
	}

	if len(failedProtocols) > 0 {
		return fmt.Errorf("dual-stack traffic tests failed: %s", strings.Join(failedProtocols, "; "))
	}

	return nil
}

// RunDualStackTrafficTestsForBothMTUs runs dual-stack traffic tests for two MTU configurations.
func RunDualStackTrafficTestsForBothMTUs(
	clientSmallMTU,
	clientLargeMTU *pod.Builder,
	serverIPv4Small,
	serverIPv6Small,
	serverIPv4Large,
	serverIPv6Large string,
	mtuSmall,
	mtuLarge int,
) error {
	klog.V(90).Infof("Running dual-stack traffic tests with MTU %d", mtuSmall)

	if err := RunDualStackTrafficTest(clientSmallMTU, serverIPv4Small, serverIPv6Small, mtuSmall); err != nil {
		return fmt.Errorf("dual-stack traffic tests failed for MTU %d: %w", mtuSmall, err)
	}

	klog.V(90).Infof("Running dual-stack traffic tests with MTU %d", mtuLarge)

	if err := RunDualStackTrafficTest(clientLargeMTU, serverIPv4Large, serverIPv6Large, mtuLarge); err != nil {
		return fmt.Errorf("dual-stack traffic tests failed for MTU %d: %w", mtuLarge, err)
	}

	return nil
}

// CreateDualStackPodPair creates a client and server pod pair for dual-stack traffic testing.
func CreateDualStackPodPair(
	clientName,
	serverName,
	clientNode,
	serverNode,
	clientNetwork,
	serverNetwork,
	ipv4ServerBindIP,
	ipv6ServerBindIP,
	clientMAC,
	serverMAC string,
	clientIPs,
	serverIPs []string,
	mtu int,
) (*pod.Builder, *pod.Builder, error) {
	klog.V(90).Infof("Creating dual-stack client pod %s and server pod %s", clientName, serverName)

	client, err := sriovenv.CreateTestClientPod(clientName, clientNode, clientNetwork, clientMAC, clientIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client pod: %w", err)
	}

	server, err := createDualStackServerPod(
		serverName, serverNode, serverNetwork, serverMAC,
		ipv4ServerBindIP, ipv6ServerBindIP, serverIPs, mtu)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server pod: %w", err)
	}

	return client, server, nil
}

func createDualStackServerPod(
	name,
	nodeName,
	networkName,
	macAddress,
	ipv4BindIP,
	ipv6BindIP string,
	ipAddresses []string,
	mtu int,
) (*pod.Builder, error) {
	klog.V(90).Infof("Creating dual-stack server pod %s on node %s", name, nodeName)

	secNetwork := []*types.NetworkSelectionElement{{Name: networkName}}

	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	command := buildDualStackServerCommand(ipv4BindIP, ipv6BindIP, tsparams.Net1Interface, mtu)

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

	if err := waitForDualStackServerReady(serverPod, tsparams.WaitTimeout); err != nil {
		return nil, fmt.Errorf("server pod %s not ready: %w", name, err)
	}

	return serverPod, nil
}
