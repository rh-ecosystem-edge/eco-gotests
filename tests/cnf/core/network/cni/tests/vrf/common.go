package tests

import (
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
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
)

// vrfNetConfig holds per-VRF network configuration for validation and connectivity checks.
type vrfNetConfig struct {
	VrfInterface string
	VrfName      string
	IPAddr       string
	NetPrefix    string
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
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create temp pod1")

	tempPod2, err := pod.NewBuilder(
		APIClient,
		"temp-pod2",
		tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).
		WithCommand(buildOverlapTempPodCommand()).
		WithNodeSelector(map[string]string{corev1.LabelHostname: serverPodNode}).
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create temp pod2")

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

func runStaticVRFScenario(
	workerNodeList []*nodes.Builder,
	sameNode bool,
	redNetName, blueNetName string,
	clientNetConfig, serverNetConfig []vrfNetConfig,
) *pod.Builder {
	clientPodNode := workerNodeList[0].Object.Name
	serverPodNode := clientPodNode

	if !sameNode {
		if len(workerNodeList) < 2 {
			Skip("not enough nodes to run DiffNode test")
		}

		serverPodNode = workerNodeList[1].Object.Name
	}

	By("Creating a client pod")

	clientPod, err := pod.NewBuilder(APIClient, "client-pod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(buildVRFNetworks(blueNetName, redNetName, clientNetConfig)).
		WithPrivilegedFlag().
		WithNodeSelector(map[string]string{corev1.LabelHostname: clientPodNode}).
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	By("Creating a server pod")

	serverPod, err := pod.NewBuilder(APIClient, "server-pod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(buildVRFNetworks(blueNetName, redNetName, serverNetConfig)).
		WithPrivilegedFlag().
		RedefineDefaultCMD([]string{"/bin/bash", "-c", buildVRFServerCommand()}).
		WithNodeSelector(map[string]string{corev1.LabelHostname: serverPodNode}).
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(clientPod, clientNetConfig)
	podHasCorrectVrfConfig(serverPod, serverNetConfig)

	connectivityViaIcmpAndHTTP(clientPod, serverNetConfig, false)

	_, err = serverPod.DeleteAndWait(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "failed to remove server pod")

	By("Validating client/server ICMP and HTTP negative test")
	connectivityViaIcmpAndHTTP(clientPod, serverNetConfig, true)

	if serverNetConfig[1].NetPrefix == "8" {
		By("Validating client/server ICMP and HTTP via eth0")
		connectivityViaEth0(clientPod, serverNetConfig[1].IPAddr, false)
	}

	return clientPod
}
