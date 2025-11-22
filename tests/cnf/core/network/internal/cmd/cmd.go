package cmd

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"k8s.io/klog/v2"
)

// ICMPConnectivityCheck checks ping against provided IPs from the client pod.
func ICMPConnectivityCheck(clientPod *pod.Builder, destIPAddresses []string, ifName ...string) error {
	klog.V(90).Infof("Checking ping against %v from the client pod %s",
		destIPAddresses, clientPod.Definition.Name)

	for _, destIPAddress := range destIPAddresses {
		ipAddress, _, err := net.ParseCIDR(destIPAddress)
		if err != nil {
			return fmt.Errorf("invalid IP address: %s", destIPAddress)
		}

		TestCmdIcmpCommand := fmt.Sprintf("ping %s -c 5", ipAddress.String())
		if ifName != nil {
			TestCmdIcmpCommand = fmt.Sprintf("ping -I %s %s -c 5", ifName[0], ipAddress.String())
		}

		if ipAddress.To4() == nil {
			TestCmdIcmpCommand = fmt.Sprintf("ping -6 %s -c 5", ipAddress.String())
			if ifName != nil {
				TestCmdIcmpCommand = fmt.Sprintf("ping -6 -I %s %s -c 5", ifName[0], ipAddress.String())
			}
		}

		output, err := clientPod.ExecCommand([]string{"bash", "-c", TestCmdIcmpCommand})
		if err != nil {
			return fmt.Errorf("ICMP connectivity failed: %s/nerror: %w", output.String(), err)
		}
	}

	return nil
}

// RunCommandOnHostNetworkPod creates hostNetwork pod  and executes given command on it.
// The Pod will be removed at the end.
func RunCommandOnHostNetworkPod(nodeName, namespace, command string) (string, error) {
	klog.V(90).Infof("Running command %s on the host network pod on node %s",
		command, nodeName)

	testPod, err := pod.NewBuilder(APIClient, "hostnetworkpod", namespace, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).WithPrivilegedFlag().WithHostNetwork().CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	if err != nil {
		return "", err
	}

	output, err := testPod.ExecCommand([]string{"bash", "-c", command})
	if err != nil {
		return "", err
	}

	_, err = testPod.DeleteAndWait(netparam.DefaultTimeout)
	if err != nil {
		return output.String(), err
	}

	return output.String(), nil
}

// GetSrIovPf returns SR-IOV PF name for given SR-IOV VF.
func GetSrIovPf(vfInterfaceName, namespace, nodeName string) (string, error) {
	klog.V(90).Infof("Getting PF interface name for VF %s on node %s", vfInterfaceName, nodeName)

	pfName, err := RunCommandOnHostNetworkPod(nodeName, namespace,
		fmt.Sprintf("ls /sys/class/net/%s/device/physfn/net/", vfInterfaceName))
	if err != nil {
		return "", err
	}

	return strings.TrimRight(pfName, "\r\n"), nil
}

// RxTrafficOnClientPod verifies the incoming packets on the dpdk client pod from the dpdk server.
func RxTrafficOnClientPod(clientPod *pod.Builder, clientRxCmd string) error {
	timeoutError := "command terminated with exit code 137"

	klog.V(90).Infof("Checking dpdk-pmd traffic command %s from the client pod %s",
		clientRxCmd, clientPod.Definition.Name)

	err := clientPod.WaitUntilRunning(time.Minute)
	if err != nil {
		return fmt.Errorf("failed to wait until pod is running with error %w", err)
	}

	clientOut, err := clientPod.ExecCommand([]string{"/bin/bash", "-c", clientRxCmd})

	if err.Error() != timeoutError {
		return fmt.Errorf("failed to run the dpdk-pmd command on the client pod %s with output %s and %w",
			clientRxCmd, clientOut.String(), err)
	}

	// Parsing output from the DPDK application
	klog.V(90).Infof("Processing testpdm output from client pod \n%s", clientOut.String())
	outPutTrue := checkRxOnly(clientOut.String())

	if !outPutTrue {
		return fmt.Errorf("failed to parse the output from RxTrafficOnClientPod")
	}

	return nil
}

// checkRxOnly checks the number of incoming packets.
func checkRxOnly(out string) bool {
	lines := strings.Split(out, "\n")
	for index, line := range lines {
		if strings.Contains(line, "NIC statistics for port") {
			if len(lines[index+1]) < 3 {
				klog.V(90).Info("Fail: line list contains less than 3 elements")

				return false
			}

			if len(lines) > index && getNumberOfPackets(lines[index+1], "RX") > 0 {
				return true
			}
		}
	}

	return false
}

// getNumberOfPackets counts the number of packets in the lines with RX output.
func getNumberOfPackets(line, firstFieldSubstr string) int {
	splitLine := strings.Fields(line)

	if !strings.Contains(splitLine[0], firstFieldSubstr) {
		klog.V(90).Infof("Failed to find expected substring %s", firstFieldSubstr)

		return 0
	}

	if len(splitLine) != 6 {
		klog.V(90).Info("the slice doesn't contain 6 elements")

		return 0
	}

	numberOfPackets, err := strconv.Atoi(splitLine[1])
	if err != nil {
		klog.V(90).Infof("failed to convert string to integer %s", err)

		return 0
	}

	return numberOfPackets
}

// ValidateTCPTraffic runs the testcmd with tcp and specified interface, port and destination.
// The receiving client needs to be listening to the specified port.
func ValidateTCPTraffic(clientPod *pod.Builder, destIPAddrs []string, interfaceName,
	containerName string, portNum int) error {
	for _, destIPAddr := range RemovePrefixFromIPList(destIPAddrs) {
		klog.V(90).Infof("Validate tcp traffic to %d to destination server IP %s", portNum, destIPAddr)

		command := fmt.Sprintf("testcmd -interface %s -protocol tcp -port %d -server %s -mtu 1000", interfaceName,
			portNum, destIPAddr)
		_, err := clientPod.ExecCommand([]string{"bash", "-c", command}, containerName)

		return err
	}

	return nil
}

// RemovePrefixFromIPList removes the prefix from a list of IP addresses with prefixes.
func RemovePrefixFromIPList(ipAddressList []string) []string {
	var ipAddressListWithoutPrefix []string

	for _, ipaddress := range ipAddressList {
		klog.V(90).Infof("Remove the network prefix from IP address %s", ipaddress)
		ipAddressListWithoutPrefix = append(ipAddressListWithoutPrefix, ipaddr.RemovePrefix(ipaddress))
	}

	return ipAddressListWithoutPrefix
}
