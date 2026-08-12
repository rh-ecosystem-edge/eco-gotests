package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netstatus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
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

var (
	sysctlMacVlanDiscoveryOnce sync.Once
	sysctlWorkerNodeName       string
	sysctlMacVlanInterfaces    []nodeInterface
	errSysctlMacVlanDiscovery  error
)

// ensureSysctlMacVlanSetup discovers a worker node and macvlan-capable interface once per suite
// run so api and e2e Describes do not each pay for a hostNetwork probe pod.
// Discovery failures are stored and re-asserted outside Once so a failed first caller
// (with ContinueOnFailure) does not turn a later Describe into Skip on empty results.
func ensureSysctlMacVlanSetup() (string, []nodeInterface) {
	sysctlMacVlanDiscoveryOnce.Do(func() {
		By("Collect node list based on worker label")

		workerNodeList, err := nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		if err != nil {
			errSysctlMacVlanDiscovery = fmt.Errorf("failed to list worker nodes: %w", err)

			return
		}

		if len(workerNodeList) == 0 {
			errSysctlMacVlanDiscovery = fmt.Errorf("cluster has no worker nodes for sysctl tests")

			return
		}

		sysctlWorkerNodeName = workerNodeList[0].Definition.Name

		By("Collect node valid interface for macvlan configuration")

		sysctlMacVlanInterfaces, errSysctlMacVlanDiscovery = getValidMacVlanInterfaces(sysctlWorkerNodeName, 1)
	})

	Expect(errSysctlMacVlanDiscovery).ToNot(HaveOccurred(), "sysctl macvlan discovery failed")

	if len(sysctlMacVlanInterfaces) < 1 {
		Skip("cluster doesn't have secondary interfaces available for sysctl test")
	}

	return sysctlWorkerNodeName, sysctlMacVlanInterfaces
}

func getValidMacVlanInterfaces(nodeName string, requestNumber int) ([]nodeInterface, error) {
	By("Select host interface for mac-vlan")

	requestedInterfaceList, err := NetConfig.GetSriovInterfaces(requestNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read requested SR-IOV interfaces from environment: %w", err)
	}

	hostPod, err := pod.NewBuilder(
		APIClient, hostNetworkPodName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithHostNetwork().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create host network pod on node %s: %w", nodeName, err)
	}

	defer func() {
		_, _ = hostPod.DeleteAndWait(tsparams.DefaultTimeout)
	}()

	var validMacVlanInterfaces []nodeInterface

	for _, interfaceName := range requestedInterfaceList {
		nodeIntf, isMacVlanCapable, checkErr := macVlanCapableInterface(hostPod, interfaceName)
		if checkErr != nil {
			return nil, fmt.Errorf("failed to inspect interface %s on node %s: %w", interfaceName, nodeName, checkErr)
		}

		if isMacVlanCapable {
			validMacVlanInterfaces = append(validMacVlanInterfaces, nodeIntf)

			if len(validMacVlanInterfaces) >= requestNumber {
				break
			}
		}
	}

	return validMacVlanInterfaces, nil
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

func copySysctlMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func createStaticIpamNad(nadName, macVlanInterfaceName string) {
	masterPlugin, err := define.MasterNadPlugin(
		nadName, "bridge", nad.IPAMStatic(), macVlanInterfaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to define macvlan master plugin")

	_, err = nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPlugin).
		Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create NAD %s", nadName))

	assertSysctlNADConfig(nadName, macVlanInterfaceName, false, nil)
}

func createSysctlTuningNad(nadName string, sysctlConfig map[string]string, macVlanIf string) {
	plugins := []nad.Plugin{
		{
			Type:   "macvlan",
			Master: macVlanIf,
			Mode:   "bridge",
			Ipam:   nad.IPAMStatic(),
		},
		*nad.TuningSysctlPlugin(false, sysctlConfig),
	}

	_, err := nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithPlugins(nadName, &plugins).
		Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create sysctl NAD %s", nadName))

	assertSysctlNADConfig(nadName, macVlanIf, true, sysctlConfig)
}

// assertSysctlNADConfig pulls the NAD and unmarshals Spec.Config to verify CNI finished writing
// a valid JSON object with the expected macvlan (and optional tuning) plugins.
func assertSysctlNADConfig(
	nadName, expectedMaster string, expectTuning bool, expectedSysctl map[string]string) {
	By(fmt.Sprintf("Verifying NAD %s Spec.Config JSON after CNI config write", nadName))

	pulled, err := nad.Pull(APIClient, nadName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull NAD %s", nadName)
	Expect(pulled.Definition.Spec.Config).NotTo(BeEmpty(), "NAD %s has empty Spec.Config", nadName)

	var cfg map[string]interface{}
	Expect(json.Unmarshal([]byte(pulled.Definition.Spec.Config), &cfg)).
		To(Succeed(), "Failed to unmarshal NAD %s Spec.Config: %s",
			nadName, pulled.Definition.Spec.Config)

	if expectTuning {
		Expect(cfg).To(HaveKey("plugins"), "NAD %s missing plugins list", nadName)

		plugins, ok := cfg["plugins"].([]interface{})
		Expect(ok).To(BeTrue(), "NAD %s plugins is not a list", nadName)
		Expect(plugins).ToNot(BeEmpty(), "NAD %s plugins list is empty", nadName)

		var foundMacvlan, foundTuning bool

		for _, pluginEntry := range plugins {
			plugin, ok := pluginEntry.(map[string]interface{})
			Expect(ok).To(BeTrue(), "NAD %s plugin entry is not an object", nadName)

			switch plugin["type"] {
			case "macvlan":
				foundMacvlan = true

				assertMacvlanPluginFields(nadName, plugin, expectedMaster)
			case "tuning":
				foundTuning = true

				assertTuningSysctlFields(nadName, plugin, expectedSysctl)
			}
		}

		Expect(foundMacvlan).To(BeTrue(), "NAD %s missing macvlan plugin", nadName)
		Expect(foundTuning).To(BeTrue(), "NAD %s missing tuning plugin", nadName)

		return
	}

	assertMacvlanPluginFields(nadName, cfg, expectedMaster)
}

func assertMacvlanPluginFields(nadName string, plugin map[string]interface{}, expectedMaster string) {
	Expect(plugin["type"]).To(Equal("macvlan"), "NAD %s type mismatch", nadName)
	Expect(plugin["mode"]).To(Equal("bridge"), "NAD %s mode mismatch", nadName)
	Expect(plugin["master"]).To(Equal(expectedMaster), "NAD %s master mismatch", nadName)

	ipam, ok := plugin["ipam"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "NAD %s missing ipam object", nadName)
	Expect(ipam["type"]).To(Equal("static"), "NAD %s ipam type mismatch", nadName)
}

// verifySysctlNetworkStatus asserts k8s.v1.cni.cncf.io/network-status after CNI ADD includes
// the secondary interface with the expected network name and IP.
func verifySysctlNetworkStatus(
	podBuilder *pod.Builder, networkName, interfaceName, expectedIP string) {
	ifaces, _, err := netstatus.ParseByInterface(APIClient, podBuilder)
	Expect(err).ToNot(HaveOccurred(), "Failed to parse network-status for pod %s",
		podBuilder.Definition.Name)

	Expect(ifaces).To(HaveKey(interfaceName),
		"network-status missing interface %s", interfaceName)

	status := ifaces[interfaceName]
	Expect(status.Name).To(ContainSubstring(networkName),
		"network-status name %q should reference network %s", status.Name, networkName)
	Expect(status.Mac).NotTo(BeEmpty(), "network-status missing MAC on %s", interfaceName)

	gotIPs := make([]string, 0, len(status.IPs))
	for _, ipWithPrefix := range status.IPs {
		gotIPs = append(gotIPs, ipaddr.RemovePrefix(ipWithPrefix))
	}

	Expect(gotIPs).To(ContainElement(expectedIP),
		"network-status IPs %v missing expected %s", gotIPs, expectedIP)
}

func defineCreatePodWithNetworksAndWaitUntilPending(podNetworks []*types.NetworkSelectionElement) {
	Expect(sysctlWorkerNodeName).NotTo(BeEmpty(), "sysctl worker node was not discovered")

	createdPod, err := pod.NewBuilder(
		APIClient, "sysctl-pending", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(sysctlWorkerNodeName).
		WithSecondaryNetwork(podNetworks).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create pod with secondary networks")

	err = createdPod.WaitUntilInStatus(corev1.PodPending, tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Pod is not in Pending phase")
}

func waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(sysctlFlag string) {
	expectedSysctlFailedMessage := fmt.Sprintf(
		"Sysctl %s is not allowed. Only the following sysctls are allowed", sysctlFlag)

	Eventually(func() bool {
		eventList, err := APIClient.Events(tsparams.TestNamespaceName).List(
			context.TODO(), metav1.ListOptions{FieldSelector: "reason=FailedCreatePodSandBox"})
		Expect(err).ToNot(HaveOccurred(), "Failed to collect events")

		for _, event := range eventList.Items {
			if strings.Contains(event.Message, expectedSysctlFailedMessage) {
				return true
			}
		}

		return false
	}, tsparams.DefaultTimeout, time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to detect FailedCreatePodSandBox event for sysctl %s", sysctlFlag))
}

func assertTuningSysctlFields(nadName string, plugin map[string]interface{}, expectedSysctl map[string]string) {
	if len(expectedSysctl) == 0 {
		return
	}

	sysctlRaw, ok := plugin["sysctl"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "NAD %s tuning plugin missing sysctl object", nadName)

	for key, value := range expectedSysctl {
		Expect(sysctlRaw).To(HaveKeyWithValue(key, value),
			"NAD %s tuning sysctl missing %s=%s", nadName, key, value)
	}
}

func defineClientNetCfg(networkName string) []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(networkName, []string{tsparams.ClientIPv4CIDR})
}

func defineServerNetCfg() []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(tsparams.NetworkWithoutSysctlMutation, []string{tsparams.ServerIPv4CIDR})
}

func defineRedirectNetCfg() []*types.NetworkSelectionElement {
	return pod.StaticIPAnnotation(tsparams.NetworkWithoutSysctlMutation, []string{tsparams.RedirectIPv4CIDR})
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

	Expect(sysctlWorkerNodeName).NotTo(BeEmpty(), "sysctl worker node was not discovered")

	podBuilder := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(sysctlWorkerNodeName).
		WithAdditionalInitContainer(initContainer).
		RedefineDefaultContainer(*mainContainer).
		WithSecondaryNetwork(podNetworks)

	createdPod, err := podBuilder.CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
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
	}, tsparams.DefaultTimeout, time.Second).Should(Succeed())
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
