package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/sriovhelper"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const (
	sysctlServerPodName   = "sysctl-server"
	sysctlRedirectPodName = "sysctl-redirect"
	sysctlClientPodName   = "sysctl-client"
	hostNetworkPodName    = "sysctl-host-ifdisc"
)

type nodeInterface struct {
	Name     string
	Physical bool
	UP       bool
	Bridge   bool
	DefRoute bool
}

func getValidMacVlanInterfaces(nodeName string, requestNumber int) []nodeInterface {
	By("Select host interface for mac-vlan")

	requestedInterfaceList, err := NetConfig.GetSriovInterfaces(requestNumber)
	Expect(err).ToNot(HaveOccurred(), "Failed to read requested SR-IOV interfaces from environment")

	hostPod, err := pod.NewBuilder(
		APIClient, hostNetworkPodName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithHostNetwork().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create host network pod on node %s", nodeName))

	defer func() {
		_, _ = hostPod.DeleteAndWait(tsparams.DefaultTimeout)
	}()

	var validMacVlanInterfaces []nodeInterface

	for _, interfaceName := range requestedInterfaceList {
		nodeIntf, isMacVlanCapable, checkErr := macVlanCapableInterface(hostPod, interfaceName)
		Expect(checkErr).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to inspect interface %s on node %s", interfaceName, nodeName))

		if isMacVlanCapable {
			validMacVlanInterfaces = append(validMacVlanInterfaces, nodeIntf)

			if len(validMacVlanInterfaces) >= requestNumber {
				break
			}
		}
	}

	return validMacVlanInterfaces
}

func macVlanCapableInterface(hostPod *pod.Builder, interfaceName string) (nodeInterface, bool, error) {
	_, err := hostPod.ExecCommand([]string{"test", "-e", "/sys/class/net/" + interfaceName})
	if err != nil {
		return nodeInterface{}, false, nil
	}

	operstateOutput, err := hostPod.ExecCommand([]string{"cat", "/sys/class/net/" + interfaceName + "/operstate"})
	if err != nil {
		return nodeInterface{}, false, fmt.Errorf("failed to get operstate for %s: %w", interfaceName, err)
	}

	linkOutput, err := hostPod.ExecCommand([]string{"ip", "-o", "link", "show", "dev", interfaceName})
	if err != nil {
		return nodeInterface{}, false, fmt.Errorf("failed to get link status for %s: %w", interfaceName, err)
	}

	defaultRoute, err := hostPod.ExecCommand([]string{"ip", "route", "show", "0.0.0.0/0"})
	if err != nil {
		return nodeInterface{}, false, fmt.Errorf("failed to get default route on node: %w", err)
	}

	linkStatus := linkOutput.String()
	nodeIntf := nodeInterface{
		Name:     interfaceName,
		Physical: true,
		UP:       strings.TrimSpace(operstateOutput.String()) == "up",
		Bridge:   strings.Contains(linkStatus, "master"),
		DefRoute: isDefaultRouteForInterface(defaultRoute.String(), interfaceName),
	}

	if !nodeIntf.UP || nodeIntf.Bridge || nodeIntf.DefRoute {
		return nodeIntf, false, nil
	}

	return nodeIntf, true, nil
}

func isDefaultRouteForInterface(defaultRouteOut string, iface string) bool {
	for _, line := range strings.Split(strings.TrimSpace(defaultRouteOut), "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field == "dev" && i+1 < len(fields) && fields[i+1] == iface {
				return true
			}
		}
	}

	return false
}

func createSysctlTuningSriovNetwork(
	networkName, workerNodeName, sriovInterfaceName string,
	sysctlFlags map[string]string, withIPAM bool) {
	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, networkName, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, tsparams.ResourceNameSysctl).
		WithMacAddressSupport().
		WithLogLevel(netparam.LogLevelDebug).
		WithLinkState("enable")

	if withIPAM {
		networkBuilder = networkBuilder.WithStaticIpam()
	}

	if len(sysctlFlags) > 0 {
		pluginJSON, err := json.Marshal(nad.TuningSysctlPlugin(false, sysctlFlags))
		Expect(err).ToNot(HaveOccurred(), "Failed to marshal sysctl meta plugin config")

		networkBuilder.Definition.Spec.MetaPluginsConfig = string(pluginJSON)
	}

	deviceID := sriovhelper.DiscoverInterfaceUnderTestDeviceID(sriovInterfaceName, workerNodeName)
	if deviceID == "1015" {
		networkBuilder = networkBuilder.WithSpoof(false)
	}

	By("Define and create sr-iov sysctl network")

	err := sriovhelper.CreateSriovNetworkAndWaitForNADCreation(networkBuilder, 5*time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SriovNetwork %s", networkName))
}

func defineClientNetCfg(networkName string) []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(networkName, []string{"10.100.100.210/24"})
}

func defineServerNetCfg() []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(tsparams.NetworkWithoutSysctlMutation, []string{"10.100.100.200/24"})
}

func defineRedirectNetCfg() []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(tsparams.NetworkWithoutSysctlMutation, []string{"10.100.100.1/24"})
}

func createServerPod() {
	createSysctlPod(sysctlServerPodName, defineServerNetCfg(), tsparams.SrvInitCMD, nil)
}

func createRedirectPod() {
	createSysctlPod(sysctlRedirectPodName, defineRedirectNetCfg(), tsparams.RdrInitCMD, nil)
}

func createClientPod(clientNetCfg []*types.NetworkSelectionElement) *pod.Builder {
	return createSysctlPod(sysctlClientPodName, clientNetCfg, tsparams.ClientInitCMD, []string{"NET_RAW"})
}

func createSysctlPod(
	name string,
	podNetworks []*types.NetworkSelectionElement,
	initCmd string,
	containerCapabilities []string,
) *pod.Builder {
	initContainer, err := pod.NewContainerBuilder(
		"init1", NetConfig.CnfNetTestContainer, []string{"bash", "-c", initCmd}).
		WithSecurityCapabilities([]string{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to define init container")

	privileged := true
	initContainer.SecurityContext.Privileged = &privileged

	containerBuilder := pod.NewContainerBuilder(
		"test", NetConfig.CnfNetTestContainer, []string{"/bin/bash", "-c", "sleep INF"})
	if len(containerCapabilities) > 0 {
		containerBuilder = containerBuilder.WithSecurityCapabilities(containerCapabilities, true)
	}

	mainContainer, err := containerBuilder.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to define main container")

	podBuilder := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithAdditionalInitContainer(initContainer).
		RedefineDefaultContainer(*mainContainer).
		WithSecondaryNetwork(podNetworks)

	createdPod, err := podBuilder.CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create pod %s", name))

	return createdPod
}

func recreateClientPod(
	runningClientPod *pod.Builder, clientNetCfg []*types.NetworkSelectionElement) *pod.Builder {
	By("Remove pod")

	_, err := runningClientPod.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove client pod")

	return createClientPod(clientNetCfg)
}

func checkRouteToDst(client *pod.Builder, destAddress string, negative bool) {
	logs, err := client.ExecCommand([]string{"ip", "route", "get", destAddress})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get route to %s", destAddress))

	if negative {
		Expect(logs.String()).ToNot(ContainSubstring("<redirected>"),
			"pod's route table has redirected route")
	} else {
		Expect(logs.String()).To(ContainSubstring("<redirected>"),
			"pod's route table doesn't have redirected route")
	}
}

func testIcmpRouteSysctlFlag(runningClientPod *pod.Builder, dstAddr, intName string, negative bool) {
	sysctlConfig := tsparams.SingleAcceptRedirectSysctlFlag
	if negative {
		sysctlConfig = tsparams.SingleSysctlFlag
	}

	Expect(cmd.VerifySysctlKernelParametersConfiguredOnPodInterface(runningClientPod, sysctlConfig, intName)).
		To(Succeed(), "sysctl kernel params are not in expected state")
	retryPing(runningClientPod, dstAddr, intName, negative)
	checkRouteToDst(runningClientPod, dstAddr, negative)
}

// retryPing verifies ICMP redirect connectivity via testcmd. When negative is true, testcmd is
// invoked with --negative and a zero exit means the ping correctly failed (accept_redirects=0).
func retryPing(runningClientPod *pod.Builder, desIP, intName string, negative bool) {
	Eventually(func() error {
		return pingIPViaInterface(runningClientPod, intName, desIP, negative)
	}, 2*time.Minute, netparam.DefaultRetryInterval).Should(Succeed())
}

func pingIPViaInterface(clientPod *pod.Builder, interfaceName, destIPAddr string, negative bool) error {
	command := []string{"testcmd", "-interface", interfaceName, "-server", destIPAddr, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := clientPod.ExecCommand(command)

	return err
}

func cleanSysctlTestNamespace() {
	By("Clean pods from the namespace")

	err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
		tsparams.DefaultTimeout, pod.GetGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to remove pods from test namespace")

	By("Clean SR-IOV networks from the operator namespace")

	for _, networkName := range []string{
		tsparams.NetworkWithoutSysctlMutation,
		tsparams.NetworkWithSysctlMutation,
	} {
		err = sriov.NewNetworkBuilder(
			APIClient, networkName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, tsparams.ResourceNameSysctl).
			DeleteAndWait(tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete SriovNetwork %s", networkName))
	}

	By("Clean NADs from the namespace")

	err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
		tsparams.DefaultTimeout, nad.GetGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to clean NetworkAttachmentDefinitions")
}

func setHostIPForwarding(nodeName, interfaceName string, enabled bool) {
	err := setHostIPForwardingQuiet(nodeName, interfaceName, enabled)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to set IP forwarding on interface %s", interfaceName))
}

func getHostIPForwardingQuiet(nodeName, interfaceName string) (bool, error) {
	output, err := cmd.RunCommandOnHostNetworkPod(nodeName, tsparams.TestNamespaceName, fmt.Sprintf(
		"cat /proc/sys/net/ipv4/conf/%s/forwarding", interfaceName))
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(output) == "1", nil
}

func setHostIPForwardingQuiet(nodeName, interfaceName string, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}

	_, err := cmd.RunCommandOnHostNetworkPod(nodeName, tsparams.TestNamespaceName, fmt.Sprintf(
		"echo %s > /proc/sys/net/ipv4/conf/%s/forwarding", value, interfaceName))

	return err
}
