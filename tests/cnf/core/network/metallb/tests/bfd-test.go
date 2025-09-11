package tests

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	mlbcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
)

var (
	secondServiceIPPool         = []string{"192.168.255.1", "192.168.255.2"}
	secondServicePoolName       = "second-bgp-adv-and-addr-pool"
	secondServiceAppLabel       = "second-app"
	secondServiceNginxPodPrefix = "second-nginx-pod-"
)

var _ = Describe("BFD", Ordered, Label(tsparams.LabelBFDTestCases), ContinueOnFailure, func() {

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Creating a new instance of MetalLB Speakers on workers")
		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to create/recreate metalLb daemonset")

		err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")
	})

	Context("single hop", Label("singlehop"), func() {
		BeforeEach(func() {
			By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
				"worker nodes.")
			frrk8sPods := verifyAndCreateFRRk8sPodList()

			By("Creating BFD profile.")
			bfdProfile := createBFDProfileAndVerifyIfItsReady(frrk8sPods)

			By("Creating BGP peer config.")
			createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], bfdProfile.Definition.Name,
				tsparams.RemoteBGPASN, false, 0, frrk8sPods)

			By("Creating MetalLb configMap")
			bfdConfigMap := createConfigMap(tsparams.RemoteBGPASN, ipv4NodeAddrList, false, true)

			By("Creating static ip annotation")
			staticIPAnnotation := pod.StaticIPAnnotation(
				frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0],
					netparam.IPSubnet24)})

			By("Creating FRR Pod with network and IP address")
			frrPod := createFrrPod(
				masterNodeList[0].Object.Name, bfdConfigMap.Object.Name, []string{}, staticIPAnnotation)

			By("Checking that BGP and BFD sessions are established and up")
			verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(frrPod, ipv4NodeAddrList)
		})

		It("basic functionality should provide fast link failure detection", reportxml.ID("47188"), func() {
			testBFDFailOver()
			testBFDFailBack()
		})

		It("provides Prometheus BFD metrics", reportxml.ID("47187"), func() {
			mlbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to pull %s namespace", NetConfig.MlbOperatorNamespace))
			_, err = mlbNs.WithLabel(tsparams.PrometheusMonitoringLabel, "true").Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to redefine %s namespace with the label %s",
				NetConfig.MlbOperatorNamespace, tsparams.PrometheusMonitoringLabel))

			By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
				"worker nodes.")
			frrk8sPods := verifyAndCreateFRRk8sPodList()

			prometheusPods, err := pod.List(APIClient, NetConfig.PrometheusOperatorNamespace, metav1.ListOptions{
				LabelSelector: tsparams.PrometheusMonitoringPodLabel,
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list prometheus pods")

			verifyMetricPresentInPrometheus(frrk8sPods, prometheusPods[0], "frrk8s_bfd_")
		})

		It("Verify BGP over BFD session remains stable during service change", reportxml.ID("76013"), func() {
			By("Getting existing FRR pod created by BeforeEach")
			frrPod, err := pod.Pull(APIClient, tsparams.FRRContainerName, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull FRR test pod")

			By("Verifying initial BFD session stability before service operations")
			verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)

			By("Creating initial MetalLB service")
			createCompleteMetalLBService(tsparams.BGPAdvAndAddressPoolName, tsparams.LBipv4Range,
				tsparams.MetallbServiceName, tsparams.LabelValue1, tsparams.MLBNginxPodName,
				workerNodeList[0].Definition.Name)
			verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)

			By("Creating additional MetalLB service to test service addition")
			createCompleteMetalLBService(secondServicePoolName, secondServiceIPPool,
				tsparams.MetallbServiceName2, secondServiceAppLabel, secondServiceNginxPodPrefix,
				workerNodeList[0].Definition.Name)
			verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)

			By("Testing endpoint modification by adding second service backend pod")
			if len(workerNodeList) > 1 {
				By("Creating additional nginx pod on second worker to modify endpoints")
				setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[1].Definition.Name,
					workerNodeList[1].Definition.Name, tsparams.LabelValue1)
				verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)
			}

			By("Testing BFD stability during service deletion")
			deleteMetalLBService(tsparams.MetallbServiceName2)
			verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)

			By("Deleting primary MetalLB service to complete cleanup")
			deleteMetalLBService(tsparams.MetallbServiceName)
			verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod, ipv4NodeAddrList)
		})

		AfterEach(func() {
			By("Removing custom nft table if exists")
			removeNFTTable(workerNodeList[0].Object.Name)

			By("Cleaning MetalLb operator namespace")
			metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
			err = metalLbNs.CleanObjects(tsparams.DefaultTimeout, metallb.GetBGPPeerGVR(), metallb.GetBFDProfileGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).
				CleanObjects(tsparams.DefaultTimeout, pod.GetGVR(), configmap.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean objects from test namespace")

		})
	})

	Context("multihop", Label("multihop"), func() {
		var err error
		speakerRoutesMap := make(map[string]string)

		BeforeEach(func() {
			By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
				"worker nodes.")
			frrk8sPods := verifyAndCreateFRRk8sPodList()

			speakerRoutesMap, err = buildRoutesMap(frrk8sPods, ipv4metalLbIPList)
			Expect(err).ToNot(HaveOccurred(), "Failed to build speaker route map")

			By("Configuring Local GW mode")
			setLocalGWMode(true)
		})

		AfterEach(func() {
			By("Removing custom nft table if exists")
			removeNFTTable(workerNodeList[0].Object.Name)

			By("Cleaning MetalLb operator namespace")
			metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
			err = metalLbNs.CleanObjects(
				tsparams.DefaultTimeout,
				metallb.GetBGPPeerGVR(),
				metallb.GetBFDProfileGVR(),
				metallb.GetBGPPeerGVR(),
				metallb.GetBGPAdvertisementGVR(),
				metallb.GetIPAddressPoolGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

			By("Removing static routes from the speakers")
			frrk8sPods := verifyAndCreateFRRk8sPodList()
			for _, frrk8sPod := range frrk8sPods {
				out, err := netenv.SetStaticRoute(frrk8sPod, "del", "172.16.0.1",
					frrconfig.ContainerName, speakerRoutesMap)
				Expect(err).ToNot(HaveOccurred(), out)
			}

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				tsparams.DefaultTimeout,
				pod.GetGVR(),
				service.GetGVR(),
				configmap.GetGVR(),
				nad.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		DescribeTable("should provide fast link failure detection", reportxml.ID("47186"),
			func(bgpProtocol, ipStack string, externalTrafficPolicy corev1.ServiceExternalTrafficPolicyType) {
				err := define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

				By("Verifying that speaker route map is not empty")
				Expect(speakerRoutesMap).ToNot(BeNil(), "Speaker route map is empty")

				By("Setting test iteration parameters")
				masterClientPodIP, subMast, mlbAddressList, nodeAddrList, addressPool, frrMasterIPs, err :=
					metallbenv.DefineIterationParams(
						ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, ipStack)

				if err != nil {
					Skip(err.Error())
				}

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")
				frrk8sPods := verifyAndCreateFRRk8sPodList()

				bfdProfile := createBFDProfileAndVerifyIfItsReady(frrk8sPods)

				neighbourASN := uint32(tsparams.LocalBGPASN)
				var eBgpMultiHop bool
				if bgpProtocol == tsparams.EBGPProtocol {
					neighbourASN = tsparams.RemoteBGPASN
					eBgpMultiHop = true
				}
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, masterClientPodIP, bfdProfile.Definition.Name,
					neighbourASN, eBgpMultiHop, 0, frrk8sPods)

				prefixLen := netparam.IPSubnetInt32
				if ipStack == netparam.IPV6Family {
					prefixLen = 128
				}

				By("Creating an IPAddressPool and BGPAdvertisement for bfd tests")
				ipAddressPool := setupBgpAdvertisementAndIPAddressPool(
					tsparams.BGPAdvAndAddressPoolName, addressPool, prefixLen)

				By("Creating a MetalLB service")
				setupMetalLbService(
					tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipAddressPool, externalTrafficPolicy)

				By("Creating nginx test pod on worker node")
				setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
					workerNodeList[0].Definition.Name,
					tsparams.LabelValue1)

				By("Creating internal NAD")
				masterBridgePlugin, err := nad.NewMasterBridgePlugin("internalnad", "br0").
					WithIPAM(nad.IPAMStatic()).GetMasterPluginConfig()
				Expect(err).ToNot(HaveOccurred(), "Failed to create master bridge plugin setting")
				bridgeNad, err := nad.NewBuilder(APIClient, "internal", tsparams.TestNamespaceName).
					WithMasterPlugin(masterBridgePlugin).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create internal NAD")

				By("Creating FRR pod one on master node")
				createFrrPodOnMasterNodeAndWaitUntilRunning("frronmaster1",
					mlbAddressList[0], subMast, frrMasterIPs[0], bridgeNad.Definition.Name,
					masterNodeList[0].Object.Name, addressPool[0], nodeAddrList[0])

				By("Creating FRR pod two on master node")
				createFrrPodOnMasterNodeAndWaitUntilRunning("frronmaster2",
					mlbAddressList[1], subMast, frrMasterIPs[1], bridgeNad.Definition.Name,
					masterNodeList[0].Object.Name, addressPool[0], nodeAddrList[1])

				By("Creating client pod config map")
				masterConfigMap := createConfigMap(int(neighbourASN), nodeAddrList, eBgpMultiHop, true)

				By("Creating FRR pod in the test namespace")
				frrPod := createFrrPod(
					masterNodeList[0].Object.Name,
					masterConfigMap.Object.Name,
					[]string{},
					pod.StaticIPAnnotation(bridgeNad.Definition.Name, []string{fmt.Sprintf("%s/%s", masterClientPodIP, subMast)}))

				// Add static routes from client towards Speaker via router internal IPs
				for index, workerAddress := range netcmd.RemovePrefixFromIPList(nodeAddrList) {
					buffer, err := mlbcmd.SetRouteOnPod(frrPod, workerAddress, frrMasterIPs[index])
					Expect(err).ToNot(HaveOccurred(), buffer.String())
				}
				By("Adding static routes to the speakers")
				for _, frrk8sPod := range frrk8sPods {
					out, err := netenv.SetStaticRoute(frrk8sPod, "add", masterClientPodIP,
						frrconfig.ContainerName, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

				By("Checking that BGP and BFD sessions are established and up")
				verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(nodeAddrList))

				By("Running http check")
				httpOutput, err := mlbcmd.Curl(frrPod, masterClientPodIP, addressPool[0], ipStack, tsparams.FRRSecondContainerName)
				Expect(err).ToNot(HaveOccurred(), httpOutput)

				testBFDFailOver()

				By("Running http check after fail-over")
				httpOutput, err = mlbcmd.Curl(frrPod, masterClientPodIP, addressPool[0], ipStack, tsparams.FRRSecondContainerName)
				// If externalTrafficPolicy is Local, the server pod should be unreachable.
				switch externalTrafficPolicy {
				case corev1.ServiceExternalTrafficPolicyTypeLocal:
					Expect(err).To(HaveOccurred(), httpOutput)
				case corev1.ServiceExternalTrafficPolicyTypeCluster:
					Expect(err).ToNot(HaveOccurred(), httpOutput)
				}
				testBFDFailBack()
			},

			Entry("", tsparams.IBPGPProtocol, netparam.IPV4Family, corev1.ServiceExternalTrafficPolicyTypeCluster,
				reportxml.SetProperty("BGPPeer", tsparams.IBPGPProtocol),
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("TrafficPolicy", "Cluster")),
			Entry("", tsparams.IBPGPProtocol, netparam.IPV4Family, corev1.ServiceExternalTrafficPolicyTypeLocal,
				reportxml.SetProperty("BGPPeer", tsparams.IBPGPProtocol),
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("TrafficPolicy", "Local")),
			Entry("", tsparams.EBGPProtocol, netparam.IPV4Family, corev1.ServiceExternalTrafficPolicyTypeCluster,
				reportxml.SetProperty("BGPPeer", tsparams.EBGPProtocol),
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("TrafficPolicy", "Custer")),
			Entry("", tsparams.EBGPProtocol, netparam.IPV4Family, corev1.ServiceExternalTrafficPolicyTypeLocal,
				reportxml.SetProperty("BGPPeer", tsparams.EBGPProtocol),
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("TrafficPolicy", "Local")),
		)

	})

	AfterAll(func() {
		By("Removing custom nft table if exists")
		removeNFTTable(workerNodeList[0].Object.Name)

		if len(cnfWorkerNodeList) > 2 {
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}
		By("Cleaning Metallb namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb namespace")
		err = metalLbNs.CleanObjects(tsparams.DefaultTimeout, metallb.GetMetalLbIoGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean metalLb operator namespace")

		By("Cleaning test namespace")
		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, pod.GetGVR(), nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")

		By("Reverting Local GW mode")
		setLocalGWMode(false)
	})
})

func createFrrPodOnMasterNodeAndWaitUntilRunning(
	name, metalLbAddr, subMask, internalFrrIP, bridgeNadName, masterNodeName, mlbPoolIP, nodeAddr string) {
	By("Creating static ip annotation for FRR pod two on master node")

	podMasterOneNetCfg := pod.StaticIPAnnotation(
		frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", metalLbAddr, subMask)})
	podMasterOneNetCfg = append(podMasterOneNetCfg, pod.StaticIPAnnotation(
		bridgeNadName, []string{fmt.Sprintf("%s/%s", internalFrrIP, subMask)})...)

	By("Creating FRR pod on master node")
	createFrrPod(
		masterNodeName,
		"",
		mlbcmd.DefineRouteAndSleep(mlbPoolIP, ipaddr.RemovePrefix(nodeAddr)),
		podMasterOneNetCfg,
		name,
	)
}
func testBFDFailOver() {
	By("Checking that BGP and BFD sessions are established and up")

	frrPod, err := pod.Pull(APIClient, tsparams.FRRContainerName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull frr test pod")

	verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(frrPod, ipv4NodeAddrList)

	firstWorkerNode, err := nodes.Pull(APIClient, workerNodeList[0].Object.Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull worker node object")

	secondWorkerNode, err := nodes.Pull(APIClient, workerNodeList[1].Object.Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull compute node object")
	secondWorkerIP, err := secondWorkerNode.ExternalIPv4Network()
	Expect(err).ToNot(HaveOccurred(), "Failed to collect external node ip")

	By("Blocking BGP and BFD ports on a first compute node via nft rules")
	blockBFDBGPPortsViaNFT(workerNodeList[0].Object.Name)

	// Sleep until BFD timeout
	time.Sleep(1200 * time.Millisecond)

	bpgUp, err := frr.BGPNeighborshipHasState(frrPod, ipaddr.RemovePrefix(secondWorkerIP), "Established")
	Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp state from FRR router")
	Expect(bpgUp).Should(BeTrue(), "BGP is not in expected established state")
	Expect(netenv.BFDHasStatus(frrPod, ipaddr.RemovePrefix(secondWorkerIP), "up")).Should(BeNil(),
		"BFD is not in expected up state")

	By("Verifying that FRR pod lost BFD and BGP session with one of the MetalLb speakers")

	firstWorkerNodeIP, err := firstWorkerNode.ExternalIPv4Network()
	Expect(err).ToNot(HaveOccurred(), "Failed to collect external node ip")
	bpgUp, err = frr.BGPNeighborshipHasState(frrPod, ipaddr.RemovePrefix(firstWorkerNodeIP), "Established")
	Expect(err).ToNot(HaveOccurred(), "Failed to collect BGP state")
	Expect(bpgUp).Should(BeFalse(), "BGP is not in expected down state")
	Expect(netenv.BFDHasStatus(frrPod, ipaddr.RemovePrefix(firstWorkerNodeIP), "up")).
		Should(HaveOccurred(), "BFD is not expected to be in Up state")
}

func testBFDFailBack() {
	By("Removing created nft table on a first compute node")
	removeNFTTable(workerNodeList[0].Object.Name)

	By("Checking that BGP and BFD sessions are established and up")

	frrPod, err := pod.Pull(APIClient, tsparams.FRRContainerName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull frr test pod")
	verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(frrPod, ipv4NodeAddrList)
}

func createBFDProfileAndVerifyIfItsReady(frrk8sPods []*pod.Builder) *metallb.BFDBuilder {
	By("Creating BFD profile")

	bfdProfile, err := metallb.NewBFDBuilder(APIClient, "bfdprofile", NetConfig.MlbOperatorNamespace).
		WithRcvInterval(300).WithPassiveMode(false).WithMinimumTTL(5).
		WithMultiplier(3).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BFD profile")
	Expect(bfdProfile.Exists()).To(BeTrue(), "BFD profile doesn't exist")

	for _, frrk8sPod := range frrk8sPods {
		Eventually(frr.IsProtocolConfigured,
			time.Minute, tsparams.DefaultRetryInterval).WithArguments(frrk8sPod, "bfd").
			Should(BeTrue(), "BFD is not configured on the Speakers")
	}

	return bfdProfile
}

func setLocalGWMode(status bool) {
	By(fmt.Sprintf("Configuring GW mode %v", status))

	clusterNetwork, err := cluster.GetOCPNetworkOperatorConfig(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to collect network.operator object")

	clusterNetwork, err = clusterNetwork.SetLocalGWMode(status, 20*time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to set local GW mode %v", status))

	network, err := clusterNetwork.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to collect network.operator object")
	Expect(network.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig.RoutingViaHost).To(BeEquivalentTo(status),
		"Failed network.operator object is not in expected state")
}

func verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(frrPod *pod.Builder, peerAddrList []string) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Eventually(frr.BGPNeighborshipHasState,
			time.Minute*4, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "Established").Should(
			BeTrue(), "Failed to receive BGP status UP")
		Eventually(netenv.BFDHasStatus,
			time.Minute, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "up").
			ShouldNot(HaveOccurred(), "Failed to receive BFD status UP")
	}
}

func verifyMetalLbBFDAndBGPSessionsRemainStable(frrPod *pod.Builder, peerAddrList []string) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Consistently(frr.BGPNeighborshipHasState,
			30*time.Second, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "Established").Should(
			BeTrue(), "BGP session did not remain stable")
		Consistently(netenv.BFDHasStatus,
			30*time.Second, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, "up").
			ShouldNot(HaveOccurred(), "BFD session did not remain stable")
	}
}

func buildRoutesMap(podList []*pod.Builder, nextHopList []string) (map[string]string, error) {
	if len(podList) == 0 {
		return nil, fmt.Errorf("pod list is empty")
	}

	if len(nextHopList) == 0 {
		return nil, fmt.Errorf("nexthop IP addresses list is empty")
	}

	if len(nextHopList) < len(podList) {
		return nil, fmt.Errorf("number of speaker IP addresses[%d] is less then number of pods[%d]",
			len(nextHopList), len(podList))
	}

	routesMap := make(map[string]string)

	for num, pod := range podList {
		routesMap[pod.Definition.Spec.NodeName] = nextHopList[num]
	}

	return routesMap, nil
}

func blockBFDBGPPortsViaNFT(nodeName string) {
	commands := []string{
		"nft add table inet my_table",
		"nft add chain inet my_table my_chain { type filter hook input priority 1 \\; policy accept \\; }",
		"nft add rule inet my_table my_chain tcp dport 179 drop",
		"nft add rule inet my_table my_chain tcp sport 179 drop",
		"nft add rule inet my_table my_chain udp dport 3784 drop",
		"nft add rule inet my_table my_chain udp sport 3784 drop",
		"nft add rule inet my_table my_chain udp dport 4784 drop",
		"nft add rule inet my_table my_chain udp sport 4784 drop",
	}

	for _, command := range commands {
		output, err := cluster.ExecCmdWithStdout(
			APIClient, command, metav1.ListOptions{LabelSelector: corev1.LabelHostname + "=" + nodeName})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command %s: %s", command, output))
	}
}

func removeNFTTable(nodeName string) {
	_, err := cluster.ExecCmdWithStdout(
		APIClient, "nft list table inet my_table",
		metav1.ListOptions{LabelSelector: corev1.LabelHostname + "=" + nodeName})

	// If table doesn't exist, skip deletion
	if err != nil && strings.Contains(err.Error(), "failed executing command") {
		By(fmt.Sprintf("nft table already deleted on node %s, skipping\n", nodeName))

		return
	}

	_, err = cluster.ExecCmdWithStdout(
		APIClient, "nft delete table inet my_table",
		metav1.ListOptions{LabelSelector: corev1.LabelHostname + "=" + nodeName})
	Expect(err).ToNot(HaveOccurred(), "Failed to delete nft table")
}

func createCompleteMetalLBService(poolName string, ipPool []string, serviceName, appLabel, podNamePrefix,
	workerNode string) {
	ipAddressPool := setupBgpAdvertisementAndIPAddressPool(poolName, ipPool, netparam.IPSubnetInt32)

	setupMetalLbService(serviceName, netparam.IPV4Family, appLabel, ipAddressPool,
		corev1.ServiceExternalTrafficPolicyTypeCluster)

	setupNGNXPod(podNamePrefix+workerNode, workerNode, appLabel)
}

func deleteMetalLBService(serviceName string) {
	By(fmt.Sprintf("Testing service deletion - removing service '%s'", serviceName))
	svc, err := service.Pull(APIClient, serviceName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull MetalLB service '%s'", serviceName))
	err = svc.Delete()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete MetalLB service '%s'", serviceName))
}
