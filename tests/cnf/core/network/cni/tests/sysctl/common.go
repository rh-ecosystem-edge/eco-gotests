package tests

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

func getAllRequestedSriovInterfaceNames() ([]string, error) {
	requestedInterfaceList, err := NetConfig.GetSriovInterfaces(1)
	if err != nil {
		return nil, err
	}

	trimmedInterfaceList := make([]string, 0, len(requestedInterfaceList))
	for _, requestedInterface := range requestedInterfaceList {
		interfaceName := strings.TrimSpace(requestedInterface)
		if interfaceName == "" {
			return nil, fmt.Errorf("empty interface name in ECO_CNF_CORE_NET_SRIOV_INTERFACE_LIST")
		}

		trimmedInterfaceList = append(trimmedInterfaceList, interfaceName)
	}

	return trimmedInterfaceList, nil
}

func getRequestedSriovInterfaceNames(requestedNumber int) ([]string, error) {
	allRequestedInterfaces, err := getAllRequestedSriovInterfaceNames()
	if err != nil {
		return nil, err
	}

	if len(allRequestedInterfaces) < requestedNumber {
		return nil, fmt.Errorf(
			"the number of SR-IOV interfaces is less than %d, check ECO_CNF_CORE_NET_SRIOV_INTERFACE_LIST env var",
			requestedNumber)
	}

	return allRequestedInterfaces[:requestedNumber], nil
}

func findWorkerNodeWithSriovInterface(workerNodeList []*nodes.Builder, sriovInterfaceName string) (string, error) {
	for _, workerNode := range workerNodeList {
		upNICs, err := sriov.NewNetworkNodeStateBuilder(
			APIClient, workerNode.Definition.Name, NetConfig.SriovOperatorNamespace).GetUpNICs()
		if err != nil {
			return "", fmt.Errorf("failed to get SR-IOV devices from node %s: %w", workerNode.Definition.Name, err)
		}

		for _, upNIC := range upNICs {
			if upNIC.Name == sriovInterfaceName {
				return workerNode.Definition.Name, nil
			}
		}
	}

	return "", fmt.Errorf("SR-IOV interface %s was not found on any worker node", sriovInterfaceName)
}

func getValidMacVlanInterfaces(nodeName string, requestNumber int) []nodeInterface {
	By("Select host interface for mac-vlan")

	requestedInterfaceList, err := getAllRequestedSriovInterfaceNames()
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

	if len(validMacVlanInterfaces) < requestNumber {
		Fail(fmt.Sprintf(
			"requested interfaces %v are not present on cluster node %s as mac-vlan capable interfaces",
			requestedInterfaceList, nodeName))
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
		DefRoute: strings.Contains(defaultRoute.String(), interfaceName),
	}

	if !nodeIntf.UP || nodeIntf.Bridge || nodeIntf.DefRoute {
		return nodeIntf, false, nil
	}

	return nodeIntf, true, nil
}

func createSysctlTuningSriovNetwork(networkName string, sysctlFlags map[string]string, withIPAM bool) {
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

	sriovInterfaces, err := NetConfig.GetSriovInterfaces(1)
	Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for sysctl tests")

	workerNodeList, err := nodes.List(
		APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")

	deviceID := discoverSriovDeviceID(sriovInterfaces[0], workerNodeList[0].Definition.Name)
	if deviceID == "1015" {
		networkBuilder = networkBuilder.WithSpoof(false)
	}

	By("Define and create sr-iov sysctl network")

	sriovNetwork, err := networkBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SriovNetwork %s", networkName))

	Eventually(func() error {
		_, pullErr := nad.Pull(APIClient, networkName, tsparams.TestNamespaceName)

		return pullErr
	}, tsparams.NADWaitTimeout, tsparams.RetryInterval).Should(Succeed(),
		fmt.Sprintf("Failed waiting for NAD %s", networkName))

	_ = sriovNetwork
}

func discoverSriovDeviceID(sriovInterfaceName, workerNodeName string) string {
	sriovNICs, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	Expect(err).ToNot(HaveOccurred(), "Failed to discover SR-IOV node interfaces")

	for _, sriovNIC := range sriovNICs {
		if sriovNIC.Name == sriovInterfaceName {
			return sriovNIC.DeviceID
		}
	}

	return ""
}

func verifySysctlKernelParametersConfiguredOnPodInterface(
	podUnderTest *pod.Builder, sysctlPluginConfig map[string]string, interfaceName string) {
	for key, value := range sysctlPluginConfig {
		sysctlKernelParam := strings.Replace(key, "IFNAME", interfaceName, 1)

		By(fmt.Sprintf("Validate sysctl flag: %s has the right value in pod's interface: %s",
			sysctlKernelParam, interfaceName))

		cmdBuffer, err := podUnderTest.ExecCommand([]string{"sysctl", "-n", sysctlKernelParam})
		Expect(err).ToNot(HaveOccurred(), "Failed to execute sysctl command on the pod")
		Expect(strings.TrimSpace(cmdBuffer.String())).To(BeIdenticalTo(value),
			"sysctl kernel param is not in expected state")
	}
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

	createdPod, err := podBuilder.CreateAndWaitUntilRunning(tsparams.PodWaitingTime)
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

	verifySysctlKernelParametersConfiguredOnPodInterface(runningClientPod, sysctlConfig, intName)
	retryPing(runningClientPod, dstAddr, intName, 3, negative)
	checkRouteToDst(runningClientPod, dstAddr, negative)
}

func retryPing(runningClientPod *pod.Builder, desIP, intName string, retry int, negative bool) {
	attempt := 0

	Eventually(func() bool {
		err := pingIPViaInterface(runningClientPod, intName, desIP, negative)
		if attempt == retry && err == nil {
			return true
		}

		if attempt > retry {
			Fail("connectivity test failed")
		}

		attempt++

		return false
	}, tsparams.PodWaitingTime, tsparams.RetryInterval).Should(BeTrue())
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

	err = namespace.NewBuilder(APIClient, NetConfig.SriovOperatorNamespace).CleanObjects(
		tsparams.DefaultTimeout, sriov.GetSriovNetworksGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to clean SR-IOV networks")

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

func setHostIPForwardingQuiet(nodeName, interfaceName string, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}

	_, err := cmd.RunCommandOnHostNetworkPod(nodeName, tsparams.TestNamespaceName, fmt.Sprintf(
		"echo %s > /proc/sys/net/ipv4/conf/%s/forwarding", value, interfaceName))

	return err
}
