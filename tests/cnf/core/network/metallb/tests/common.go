package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	typesGomega "github.com/onsi/gomega/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypesv1beta2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreparams"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/prometheus"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// test cases variables that are accessible across entire package.
var (
	clusterIPVersion  string
	ipv4metalLbIPList []string
	ipv4NodeAddrList  []string
	ipv6metalLbIPList []string
	ipv6NodeAddrList  []string
	nodeAddrList      map[string][]string
	metallbAddrList   map[string][]string
	cnfWorkerNodeList []*nodes.Builder
	workerNodeList    []*nodes.Builder
	masterNodeList    []*nodes.Builder
	workerLabelMap    map[string]string
)

var (
	metalLbTestsLabel    = map[string]string{"metallb": "metallbtests"}
	frrK8WebHookServer   = "frr-k8s-webhook-server"
	ovnExternalAddresses = "k8s.ovn.org/node-primary-ifaddr"
	frrPodSubnet         = map[string]string{
		netparam.IPV4Family: netparam.IPSubnet24,
		netparam.IPV6Family: netparam.IPSubnet64,
	}
	defaultAggLen = map[string]int{
		netparam.IPV4Family: netparam.IPSubnetInt32,
		netparam.IPV6Family: netparam.IPSubnetInt128,
	}
)

const (
	ipv4 = netparam.IPV4Family
	ipv6 = netparam.IPV6Family
)

func clusterSupportsIPv6() bool {
	return clusterIPVersion == netparam.IPV6Family || clusterIPVersion == netparam.DualIPFamily
}

func clusterSupportsIPv4() bool {
	return clusterIPVersion == netparam.IPV4Family || clusterIPVersion == netparam.DualIPFamily
}

func getClusterIPVersion() (string, error) {
	nodesList, err := nodes.List(APIClient)
	if err != nil {
		return "", err
	}

	if len(nodesList) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	var ipVersion string

	for nodeIndex, node := range nodesList {
		ipVersion, _, err = getNodeExternalIP(node)
		if err != nil {
			return "", err
		}

		if nodeIndex == 0 {
			clusterIPVersion = ipVersion

			continue
		}

		if ipVersion != clusterIPVersion {
			return "", fmt.Errorf("mixed IP families detected: %s and %s", clusterIPVersion, ipVersion)
		}
	}

	return clusterIPVersion, nil
}

func getNodeExternalIP(nodeBuilder *nodes.Builder) (string, nodes.ExternalNetworks, error) {
	var extNetwork nodes.ExternalNetworks

	raw := nodeBuilder.Object.Annotations[ovnExternalAddresses]
	if raw == "" {
		return "", nodes.ExternalNetworks{}, fmt.Errorf(
			"node %q missing %q annotation",
			nodeBuilder.Object.Name,
			ovnExternalAddresses,
		)
	}

	err := json.Unmarshal([]byte(raw), &extNetwork)
	if err != nil {
		return "", nodes.ExternalNetworks{}, err
	}

	switch {
	case extNetwork.IPv4 != "" && extNetwork.IPv6 == "":
		return netparam.IPV4Family, extNetwork, nil
	case extNetwork.IPv4 == "" && extNetwork.IPv6 != "":
		return netparam.IPV6Family, extNetwork, nil
	case extNetwork.IPv4 != "" && extNetwork.IPv6 != "":
		return netparam.DualIPFamily, extNetwork, nil
	default:
		return "", nodes.ExternalNetworks{}, fmt.Errorf("no IPv4 or IPv6 nodes found")
	}
}

// Initializes and validates Vars:
// ipv4metalLbIPList, ipv6metalLbIPList,
// ipv4NodeAddrList, ipv6NodeAddrList,
// nodeAddrList, metallbAddrList,
// cnfWorkerNodeList, workerLabelMap,
// workerNodeList, masterNodeList.
func validateEnvVarAndGetNodeList() {
	var err error
	// Initialize maps before assignment to avoid nil map panic
	nodeAddrList = make(map[string][]string)
	metallbAddrList = make(map[string][]string)

	By("Getting cluster IP version")

	clusterIPVersion, err = getClusterIPVersion()
	Expect(err).ToNot(HaveOccurred(), "Failed to get cluster IP version")

	By("Fetching IPv4 and IPv6 IPs from ENV VAR to be used for External FRR Pod")

	ipv4metalLbIPList, ipv6metalLbIPList, err = metallbenv.GetMetalLbIPByIPStack()
	Expect(err).ToNot(HaveOccurred(), tsparams.MlbAddressListError)

	if clusterSupportsIPv4() {
		Expect(len(ipv4metalLbIPList)).To(BeNumerically(">=", 2))
	}

	if clusterSupportsIPv6() {
		Expect(len(ipv6metalLbIPList)).To(BeNumerically(">=", 2))
	}

	By("Selecting Worker nodes for the test")

	cnfWorkerNodeList, err = nodes.List(APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

	workerLabelMap, workerNodeList = setWorkerNodeListAndLabelForMlbTests(cnfWorkerNodeList, metalLbTestsLabel)

	By("Validating whether the IPv4 addresses of ENV VAR are in the same subnet as Worker Nodes external IPv4 range")

	if clusterSupportsIPv4() {
		ipv4NodeAddrList, err = nodes.ListExternalIPv4Networks(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(workerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to collect external nodes ipv4 addresses")

		err = metallbenv.IsEnvVarMetalLbIPinNodeExtNetRange(ipv4NodeAddrList, ipv4metalLbIPList, nil)
		Expect(err).ToNot(HaveOccurred(), "Failed to validate metalLb exported ipv4 address")

		nodeAddrList[ipv4] = ipv4NodeAddrList
		metallbAddrList[ipv4] = ipv4metalLbIPList
	}

	By("Validating whether the IPv6 addresses of ENV VAR are in the same subnet as Worker Nodes external IPv6 range")

	if clusterSupportsIPv6() {
		ipv6NodeAddrList, err = nodes.ListExternalIPv6Networks(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(workerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to collect external nodes ipv6 addresses")

		err = metallbenv.IsEnvVarMetalLbIPinNodeExtNetRange(ipv6NodeAddrList, nil, ipv6metalLbIPList)
		Expect(err).ToNot(HaveOccurred(), "Failed to validate metalLb exported ipv6 address")

		nodeAddrList[ipv6] = ipv6NodeAddrList
		metallbAddrList[ipv6] = ipv6metalLbIPList
	}

	By("Listing Master Nodes")

	masterNodeList, err = nodes.List(APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Fail to list master nodes")
	Expect(len(masterNodeList)).To(BeNumerically(">=", 1))
}

func setWorkerNodeListAndLabelForMlbTests(
	workerNodeList []*nodes.Builder, nodeSelector map[string]string) (map[string]string, []*nodes.Builder) {
	if len(workerNodeList) > 2 {
		By(fmt.Sprintf(
			"Worker node number is greater than 2. Limit worker nodes for bfd test using label %v", nodeSelector))
		addNodeLabel(workerNodeList[:2], nodeSelector)

		return metalLbTestsLabel, workerNodeList[:2]
	}

	return NetConfig.WorkerLabelMap, workerNodeList
}

func removeNodeLabel(workerNodeList []*nodes.Builder, nodeSelector map[string]string) {
	updateNodeLabel(workerNodeList, nodeSelector, true)
}

func addNodeLabel(workerNodeList []*nodes.Builder, nodeSelector map[string]string) {
	updateNodeLabel(workerNodeList, nodeSelector, false)
}

func updateNodeLabel(workerNodeList []*nodes.Builder, nodeLabel map[string]string, removeLabel bool) {
	for _, worker := range workerNodeList {
		worker, err := nodes.Pull(APIClient, worker.Definition.Name)
		Expect(err).ToNot(HaveOccurred(), "Fail to pull latest worker %s object", worker.Definition.Name)

		if removeLabel {
			worker.RemoveLabel(netenv.MapFirstKeyValue(nodeLabel))
		} else {
			worker.RemoveLabel(netenv.MapFirstKeyValue(nodeLabel))
			worker.WithNewLabel(netenv.MapFirstKeyValue(nodeLabel))
		}

		_, err = worker.Update()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to update node's labels %s", worker.Definition.Name))
	}
}

func createConfigMap(
	bgpAsn int, nodeAddrList []string, enableMultiHop, enableBFD bool) *configmap.Builder {
	frrBFDConfig := frr.DefineBGPConfigWithIPv4AndIPv6(
		bgpAsn, tsparams.LocalBGPASN, netcmd.RemovePrefixFromIPList(nodeAddrList), enableMultiHop, enableBFD)
	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrBFDConfig, "")
	masterConfigMap, err := configmap.NewBuilder(APIClient, "frr-master-node-config", tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")

	return masterConfigMap
}

func createHubConfigMap(name string) *configmap.Builder {
	frrBFDConfig := frr.DefineBGPConfig(
		tsparams.LocalBGPASN, tsparams.LocalBGPASN, []string{"10.10.0.10"}, false, false)
	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrBFDConfig, "")
	hubConfigMap, err := configmap.NewBuilder(APIClient, name, tsparams.TestNamespaceName).WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create hub config map")

	return hubConfigMap
}

func createExternalNad(name string) {
	By("Creating external BR-EX NetworkAttachmentDefinition")

	macVlanPlugin, err := define.MasterNadPlugin(coreparams.OvnExternalBridge, "bridge", nad.IPAMStatic())
	Expect(err).ToNot(HaveOccurred(), "Failed to define master nad plugin")
	externalNad, err := nad.NewBuilder(APIClient, name, tsparams.TestNamespaceName).
		WithMasterPlugin(macVlanPlugin).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create external NetworkAttachmentDefinition")
	Expect(externalNad.Exists()).To(BeTrue(), "Failed to detect external NetworkAttachmentDefinition")
}

func createExternalNadWithMasterInterface(name, masterInterface string) {
	By("Creating external BR-EX NetworkAttachmentDefinition")

	macVlanPlugin, err := define.MasterNadPlugin(name, "bridge", nad.IPAMStatic(), masterInterface)
	Expect(err).ToNot(HaveOccurred(), "Failed to define master nad plugin")
	externalNad, err := nad.NewBuilder(APIClient, name, tsparams.TestNamespaceName).
		WithMasterPlugin(macVlanPlugin).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create external NetworkAttachmentDefinition")
	Expect(externalNad.Exists()).To(BeTrue(), "Failed to detect external NetworkAttachmentDefinition")
}

func createBGPPeerAndVerifyIfItsReady(
	name, peerIP, bfdProfileName string, remoteAsn uint32, eBgpMultiHop bool, connectTime int,
	frrk8sPods []*pod.Builder) {
	By("Creating BGP Peer")

	bgpPeer := metallb.NewBPGPeerBuilder(APIClient, name, NetConfig.MlbOperatorNamespace,
		peerIP, tsparams.LocalBGPASN, remoteAsn).WithPassword(tsparams.BGPPassword).WithEBGPMultiHop(eBgpMultiHop)

	if bfdProfileName != "" {
		bgpPeer.WithBFDProfile(bfdProfileName)
	}

	if connectTime != 0 {
		// Convert connectTime int to time.Duration in seconds
		bgpPeer.WithConnectTime(metav1.Duration{Duration: time.Duration(connectTime) * time.Second})
	}

	_, err := bgpPeer.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGP peer")

	By("Verifying if BGP protocol configured")

	for _, frrk8sPod := range frrk8sPods {
		Eventually(frr.IsProtocolConfigured,
			time.Minute, tsparams.DefaultRetryInterval).WithArguments(frrk8sPod, "router bgp").
			Should(BeTrue(), "BGP is not configured on the Speakers")
	}
}

//nolint:unparam
func createBGPPeerUnnumberedAndVerifyIfItsReady(
	name, dynamicASN, interfaceName, bfdProfileName string, localAS, remoteAsn uint32, eBgpMultiHop bool, connectTime int,
	frrk8sPod *pod.Builder, nodeSelector map[string]string) {
	By("Creating BGP Peer")

	bgpPeer := metallb.NewBGPPeerBuilder(APIClient, name, NetConfig.MlbOperatorNamespace, localAS, remoteAsn).
		WithPassword(tsparams.BGPPassword).WithEBGPMultiHop(eBgpMultiHop).WithIPUnnumbered(interfaceName).
		WithNodeSelector(nodeSelector).WithHoldTime(metav1.Duration{
		Duration: 90 * time.Second,
	}).WithKeepalive(metav1.Duration{Duration: 30 * time.Second})

	if dynamicASN != "" {
		bgpPeer.WithDynamicASN(mlbtypesv1beta2.DynamicASNMode(dynamicASN))
	}

	if bfdProfileName != "" {
		bgpPeer.WithBFDProfile(bfdProfileName)
	}

	if connectTime != 0 {
		// Convert connectTime int to time.Duration in seconds
		bgpPeer.WithConnectTime(metav1.Duration{Duration: time.Duration(connectTime) * time.Second})
	}

	_, err := bgpPeer.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGP peer")

	By("Verifying if BGP protocol configured")

	Eventually(frr.IsProtocolConfigured,
		time.Minute, tsparams.DefaultRetryInterval).WithArguments(frrk8sPod, "router bgp").
		Should(BeTrue(), "BGP is not configured on the Speakers")
}

func setupBgpAdvertisementAndIPAddressPool(
	name string, addressPool []string, prefixLen int) *metallb.IPAddressPoolBuilder {
	ipAddressPool, err := metallb.NewIPAddressPoolBuilder(
		APIClient,
		name,
		NetConfig.MlbOperatorNamespace,
		[]string{fmt.Sprintf("%s-%s", addressPool[0], addressPool[1])}).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool")

	_, err = metallb.
		NewBGPAdvertisementBuilder(APIClient, name, NetConfig.MlbOperatorNamespace).
		WithIPAddressPools([]string{ipAddressPool.Definition.Name}).
		WithCommunities([]string{"65535:65282"}).
		WithLocalPref(100).
		WithAggregationLength4(int32(prefixLen)).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement")

	return ipAddressPool
}

func setupBgpAdvertisement(
	name,
	communities,
	ipAddressPool string,
	localPreference uint32,
	bgpPeers []string,
	nodeSelectors []metav1.LabelSelector) {
	builder := metallb.NewBGPAdvertisementBuilder(APIClient, name, NetConfig.MlbOperatorNamespace).
		WithIPAddressPools([]string{ipAddressPool}).
		WithCommunities([]string{communities}).
		WithLocalPref(localPreference).
		WithAggregationLength4(32)

	if len(nodeSelectors) > 0 {
		builder = builder.WithNodeSelector(nodeSelectors)
	}

	if len(bgpPeers) > 0 {
		builder = builder.WithPeers(bgpPeers)
	}

	_, err := builder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement")
}

func updateBgpAdvertisementWithNodeSelector(
	name string,
	nodeSelectors []metav1.LabelSelector) {
	builder, err := metallb.PullBGPAdvertisement(APIClient, name, NetConfig.MlbOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull existing BGPAdvertisement")
	_, err = builder.WithNodeSelector(nodeSelectors).Update(true)
	Expect(err).ToNot(HaveOccurred(), "Failed to update BGPAdvertisement")
}

func setupL2Advertisement(addressPool []string) *metallb.IPAddressPoolBuilder {
	ipAddressPool, err := metallb.NewIPAddressPoolBuilder(
		APIClient,
		"l2address-pool",
		NetConfig.MlbOperatorNamespace,
		[]string{fmt.Sprintf("%s-%s", addressPool[0], addressPool[1])}).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool")

	_, err = metallb.
		NewL2AdvertisementBuilder(APIClient, "l2advertisement", NetConfig.MlbOperatorNamespace).
		WithIPAddressPools([]string{ipAddressPool.Definition.Name}).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement")

	return ipAddressPool
}

func verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod *pod.Builder, peerAddrList []string) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Eventually(frr.BGPNeighborshipHasState,
			time.Minute*4, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "Established").Should(
			BeTrue(), "Failed to receive BGP status UP")
	}
}

func verifyMetalLbBGPSessionsAreDownOnFrrPod(frrPod *pod.Builder, peerAddrList []string) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Eventually(func() bool {
			neighborState, _ := frr.BGPNeighborshipHasState(frrPod, peerAddress, "Established")

			return neighborState
		}, 30*time.Second, 5*time.Second).Should(BeFalse(),
			fmt.Sprintf("BGP session to %s should not be Established, but it is", peerAddress))

		Consistently(frr.BGPNeighborshipHasState,
			time.Minute, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "Established").Should(
			Not(BeTrue()), fmt.Sprintf("BGP session to %s unexpectedly reached Established state", peerAddress))
	}
}

func createFrrPod(
	nodeName string,
	configmapName string,
	defaultCMD []string,
	secondaryNetConfig []*types.NetworkSelectionElement,
	podName ...string) *pod.Builder {
	name := tsparams.FRRContainerName

	if len(podName) > 0 {
		name = podName[0]
	}

	frrPod := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.FrrImage).
		DefineOnNode(nodeName).
		WithTolerationToMaster().
		WithSecondaryNetwork(secondaryNetConfig).
		RedefineDefaultCMD(defaultCMD)

	By("Creating FRR container")

	if configmapName != "" {
		frrContainer := pod.NewContainerBuilder(
			tsparams.FRRSecondContainerName,
			NetConfig.CnfNetTestContainer,
			[]string{"/bin/bash", "-c",
				"while ip -4 route del default 2>/dev/null; do :; done; " +
					"while ip -6 route del default 2>/dev/null; do :; done; sleep INF"}).
			WithSecurityCapabilities([]string{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true)

		frrCtr, err := frrContainer.GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to get container configuration")
		frrPod.WithAdditionalContainer(frrCtr).WithLocalVolume(configmapName, "/etc/frr")
	}

	By("Creating FRR pod in the test namespace")

	frrPod, err := frrPod.WithPrivilegedFlag().CreateAndWaitUntilRunning(time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create FRR test pod")

	return frrPod
}

func createFrrHubPod(name, nodeName, configmapName string, defaultCMD []string,
	secondaryNetConfig []*types.NetworkSelectionElement) *pod.Builder {
	frrPod := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.FrrImage).
		DefineOnNode(nodeName).
		WithTolerationToMaster().
		WithSecondaryNetwork(secondaryNetConfig).
		RedefineDefaultCMD(defaultCMD)

	By("Creating FRR container")

	frrContainer := pod.NewContainerBuilder(
		tsparams.FRRSecondContainerName, NetConfig.CnfNetTestContainer, tsparams.SleepCMD).
		WithSecurityCapabilities([]string{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true)

	frrCtr, err := frrContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container configuration")
	frrPod.WithAdditionalContainer(frrCtr).WithLocalVolume(configmapName, "/etc/frr")

	By("Creating FRR pod in the test namespace")

	frrPod, err = frrPod.WithPrivilegedFlag().CreateAndWaitUntilRunning(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create FRR test pod")

	return frrPod
}

func setupMetalLbService(
	name,
	ipStack,
	labelValue string,
	ipAddressPool *metallb.IPAddressPoolBuilder,
	extTrafficPolicy corev1.ServiceExternalTrafficPolicyType) {
	servicePort, err := service.DefineServicePort(80, 80, "TCP")
	Expect(err).ToNot(HaveOccurred(), "Failed to define service port")
	_, err = service.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		map[string]string{"app": labelValue}, *servicePort).
		WithExternalTrafficPolicy(extTrafficPolicy).
		WithIPFamily([]corev1.IPFamily{corev1.IPFamily(ipStack)}, corev1.IPFamilyPolicySingleStack).
		WithAnnotation(map[string]string{"metallb.universe.tf/address-pool": ipAddressPool.Definition.Name}).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create MetalLB Service")
}

//nolint:unparam
func setupLoadBalancerService(
	name,
	ipStack,
	labelValue string,
	ipAddressPool *metallb.IPAddressPoolBuilder,
	extTrafficPolicy corev1.ServiceExternalTrafficPolicyType,
	protocol corev1.Protocol,
	port, targetPort int32) {
	servicePort, err := service.DefineServicePort(port, targetPort, protocol)
	Expect(err).ToNot(HaveOccurred(), "Failed to define service port")

	var (
		ipFamily       []corev1.IPFamily
		ipFamilyPolicy corev1.IPFamilyPolicyType
	)

	if ipStack == netparam.DualIPFamily {
		ipFamily = []corev1.IPFamily{corev1.IPFamily(netparam.IPV4Family), corev1.IPFamily(netparam.IPV6Family)}
		ipFamilyPolicy = corev1.IPFamilyPolicyRequireDualStack
	} else {
		ipFamily = []corev1.IPFamily{corev1.IPFamily(ipStack)}
		ipFamilyPolicy = corev1.IPFamilyPolicySingleStack
	}

	_, err = service.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		map[string]string{"app": labelValue}, *servicePort).
		WithExternalTrafficPolicy(extTrafficPolicy).
		WithIPFamily(ipFamily, ipFamilyPolicy).
		WithAnnotation(map[string]string{"metallb.universe.tf/address-pool": ipAddressPool.Definition.Name}).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create MetalLB Service")
}

func setupNGNXPod(podName, nodeName, labelValue string) {
	_, err := pod.NewBuilder(
		APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithLabel("app", labelValue).
		RedefineDefaultCMD([]string{"/bin/bash", "-c", "nginx && sleep INF"}).
		WithPrivilegedFlag().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create nginx test pod")
}

//nolint:unparam
func setupNGNXPodAndSCTPServer(podName, nodeName, labelValue string, sctpIPv6 ...bool) {
	var sctpIPv6Cmd string

	if len(sctpIPv6) > 0 && sctpIPv6[0] {
		sctpIPv6Cmd = "--server ::"
	}

	_, err := pod.NewBuilder(
		APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithLabel("app", labelValue).
		RedefineDefaultCMD([]string{"/bin/bash", "-c",
			fmt.Sprintf("nginx && /usr/bin/testcmd -listen -interface eth0 -port 50000 -protocol sctp %s", sctpIPv6Cmd)}).
		WithPrivilegedFlag().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create nginx test pod")
}

func verifyMetricPresentInPrometheus(
	frrk8sPods []*pod.Builder, prometheusPod *pod.Builder, metricPrefix string, expectedMetrics ...[]string) {
	By("Verifying if metrics are present in Prometheus database")

	for _, frrk8sPod := range frrk8sPods {
		var (
			metricsFromSpeaker []string
			err                error
		)

		Eventually(func() error {
			metricsFromSpeaker, err = frr.GetMetricsByPrefix(frrk8sPod, metricPrefix)

			return err
		}, time.Minute, tsparams.DefaultRetryInterval).ShouldNot(HaveOccurred(),
			"Failed to collect metrics from speaker pods")

		if len(expectedMetrics) > 0 {
			By("Verifying if metrics match expected list of metrics")
			Expect(expectedMetrics[0]).To(ContainElements(metricsFromSpeaker))
		}

		Eventually(
			prometheus.PodMetricsPresentInDB, 5*time.Minute, tsparams.DefaultRetryInterval).WithArguments(
			prometheusPod, frrk8sPod.Definition.Name, metricsFromSpeaker).Should(
			BeTrue(), "Failed to match metric in prometheus")
	}
}

func metalLbDaemonSetShouldMatchConditionAndBeInReadyState(
	expectedCondition typesGomega.GomegaMatcher, errorMessage string) {
	metalLbDs, err := daemonset.Pull(APIClient, tsparams.MetalLbDsName, NetConfig.MlbOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb speaker daemonSet")

	Eventually(func() int32 {
		if metalLbDs.Exists() {
			return metalLbDs.Object.Status.NumberAvailable
		}

		return 0
	}, tsparams.DefaultTimeout, tsparams.DefaultRetryInterval).Should(expectedCondition, errorMessage)
	Expect(metalLbDs.IsReady(120*time.Second)).To(BeTrue(), "MetalLb daemonSet is not Ready")
}

func resetOperatorAndTestNS() {
	By("Cleaning MetalLb and openshift-frr-k8s operator namespaces")

	metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
	err = metalLbNs.CleanObjects(
		tsparams.DefaultTimeout,
		metallb.GetBGPPeerGVR(),
		metallb.GetBFDProfileGVR(),
		metallb.GetL2AdvertisementGVR(),
		metallb.GetBGPAdvertisementGVR(),
		metallb.GetIPAddressPoolGVR(),
		metallb.GetMetalLbIoGVR(),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

	frrk8sNs, err := namespace.Pull(APIClient, NetConfig.Frrk8sNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull openshift-frr-k8s operator namespace")
	err = frrk8sNs.CleanObjects(
		tsparams.DefaultTimeout,
		metallb.GetFrrConfigurationGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

	By("Cleaning test namespace")

	err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
		tsparams.DefaultTimeout,
		pod.GetGVR(),
		service.GetGVR(),
		configmap.GetGVR(),
		nad.GetGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
}

func validatePrefix(
	masterNodeFRRPod *pod.Builder,
	ipProtoVersion string,
	prefix int,
	workerNodesAddresses,
	addressPool []string,
	noRouteCheck ...bool, // Optional boolean argument
) {
	var nextHopAddresses []string

	_, subnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", addressPool[0], prefix))
	Expect(err).ToNot(HaveOccurred(), "Failed to parse CIDR")

	if len(noRouteCheck) > 0 && noRouteCheck[0] {
		Eventually(func() bool {
			bgpStatus, err := frr.GetBGPStatus(masterNodeFRRPod, strings.ToLower(ipProtoVersion), "test")
			if err != nil {
				return false
			}

			_, exists := bgpStatus.Routes[subnet.String()]

			return !exists
		}, time.Minute, tsparams.DefaultRetryInterval).
			Should(BeTrue(), "Expected BGP status to not contain prefix")
	} else {
		Eventually(func() bool {
			bgpStatus, err := frr.GetBGPStatus(masterNodeFRRPod, strings.ToLower(ipProtoVersion), "test")
			if err != nil {
				return false
			}

			return len(bgpStatus.Routes) > 0
		}, time.Minute, tsparams.DefaultRetryInterval).
			Should(BeTrue(), "Expected BGP status to contain routes, but it was empty")

		bgpStatus, err := frr.GetBGPStatus(masterNodeFRRPod, strings.ToLower(ipProtoVersion), "test")

		Expect(err).NotTo(HaveOccurred(), "Failed to get BGP status")
		Expect(bgpStatus.Routes).To(HaveKey(subnet.String()), "Failed to verify subnet in bgp status output")

		for _, route := range bgpStatus.Routes[subnet.String()] {
			Expect(route.PrefixLen).To(BeNumerically("==", prefix),
				"Failed prefix length is not in expected value")

			for _, nHop := range route.Nexthops {
				nextHopAddresses = append(nextHopAddresses, nHop.IP)
			}
		}

		Expect(nextHopAddresses).To(ContainElements(workerNodesAddresses),
			"Failed next hop address is not in node addresses list")
	}
}

func removePrefixFromIPList(ipAddressList []string) []string {
	var ipAddressListWithoutPrefix []string
	for _, ipaddress := range ipAddressList {
		ipAddressListWithoutPrefix = append(ipAddressListWithoutPrefix, ipaddr.RemovePrefix(ipaddress))
	}

	return ipAddressListWithoutPrefix
}

func verifyAndCreateFRRk8sPodList() []*pod.Builder {
	frrk8sWebhookDeployment, err := deployment.Pull(
		APIClient, frrK8WebHookServer, NetConfig.Frrk8sNamespace)
	Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
	Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
		"frr-k8s-webhook-server deployment is not ready")

	frrk8sPods := []*pod.Builder{}

	for _, node := range workerNodeList {
		var frrk8sPodList []*pod.Builder

		Eventually(func() error {
			pods, err := pod.List(APIClient, NetConfig.Frrk8sNamespace, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Definition.Name),
				LabelSelector: tsparams.FRRK8sDefaultLabel,
			})
			if err != nil {
				return err
			}
			if len(pods) == 0 {
				return fmt.Errorf("no FRR k8s pod found on node %s", node.Definition.Name)
			}

			frrk8sPodList = pods

			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "Failed to find FRR k8s pod on node %s", node.Definition.Name)

		frrk8sPods = append(frrk8sPods, frrk8sPodList[0])
	}

	return frrk8sPods
}
