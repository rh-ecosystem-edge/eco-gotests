package tests

import (
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BGP", Ordered, Label(tsparams.LabelBGPTestCases), ContinueOnFailure, func() {
	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Creating a new instance of MetalLB Speakers on workers")
		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		resetOperatorAndTestNS()
	})

	AfterEach(func() {
		By("Cleaning MetalLb operator namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetBGPPeerGVR(),
			metallb.GetBFDProfileGVR(),
			metallb.GetBGPAdvertisementGVR(),
			metallb.GetIPAddressPoolGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

		By("Cleaning test namespace")
		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout,
			pod.GetGVR(),
			service.GetGVR(),
			configmap.GetGVR(),
			nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	Context("functionality", func() {
		DescribeTable("Creating AddressPool with bgp-advertisement", reportxml.ID("47174"),
			func(ipStack string, prefixLen int) {

				validateIPFamilySupport(ipStack)

				_, extFrrPod, _ := setupTestEnv(ipStack, prefixLen, false)

				By("Validating BGP route prefix")
				validatePrefix(
					extFrrPod, ipStack, prefixLen, removePrefixFromIPList(nodeAddrList[ipStack]), tsparams.LBipRange1[ipStack])
			},

			Entry("", netparam.IPV4Family, 32,
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet32)),
			Entry("", netparam.IPV4Family, 28,
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet28)),
			Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, 128,
				reportxml.SetProperty("IPStack", netparam.IPV6Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet128)),
			Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, 64,
				reportxml.SetProperty("IPStack", netparam.IPV6Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet64)),
		)

		It("provides Prometheus BGP metrics", reportxml.ID("47202"), func() {
			frrk8sPods, _, _ := setupTestEnv(ipv4, 32, false)

			By("Label namespace")
			testNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred())
			_, err = testNs.WithLabel(tsparams.PrometheusMonitoringLabel, "true").Update()
			Expect(err).ToNot(HaveOccurred())

			By("Listing prometheus pods")
			prometheusPods, err := pod.List(APIClient, NetConfig.PrometheusOperatorNamespace, metav1.ListOptions{
				LabelSelector: tsparams.PrometheusMonitoringPodLabel,
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list prometheus pods")

			verifyMetricPresentInPrometheus(
				frrk8sPods, prometheusPods[0], "frrk8s_bgp_", tsparams.MetalLbBgpMetrics)
		})

		DescribeTable("Verify external FRR BGP Peer cannot propagate routes to Speaker",
			reportxml.ID("47203"),
			func(ipStack string) {

				validateIPFamilySupport(ipStack)

				frrk8sPods, extFrrPod, _ := setupTestEnv(ipStack, defaultAggLen[ipStack], true)

				By("Verify external FRR is advertising prefixes")
				advRoutes, err := frr.GetBGPAdvertisedRoutes(extFrrPod, netcmd.RemovePrefixFromIPList(nodeAddrList[ipStack]))
				Expect(err).ToNot(HaveOccurred(), "Failed to get BGP Advertised routes")
				Expect(len(advRoutes)).To(BeNumerically(">", 0), "BGP Advertised routes should not be empty")

				By("Verify MetalLB FRR pod is not receiving routes from External FRR Pod")
				recRoutes, err := frr.VerifyBGPReceivedRoutesOnFrrNodes(frrk8sPods)
				Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")
				Expect(recRoutes).ShouldNot(SatisfyAny(
					ContainSubstring(tsparams.ExtFrrConnectedPools[ipStack][0]),
					ContainSubstring(tsparams.ExtFrrConnectedPools[ipStack][1])),
					"Received routes validation failed")
			},
			Entry("", netparam.IPV4Family,
				reportxml.SetProperty("IPStack", netparam.IPV4Family)),
			Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family,
				reportxml.SetProperty("IPStack", netparam.IPV6Family)),
		)
	})

	Context("Log Level Feature", func() {
		It("Verify frrk8s pod default Info logs", reportxml.ID("49810"), func() {
			By("Fetch speaker pods from metallb-system namespace")
			speakerPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace,
				metav1.ListOptions{LabelSelector: tsparams.SpeakerLabel})
			Expect(err).ToNot(HaveOccurred(), "Failed to list speaker pods")
			Expect(len(speakerPods)).Should(BeNumerically(">", 0), "Speaker Pods List should not be empty")

			By("Verify loglevel in speaker pod logs")
			for _, speakerPod := range speakerPods {
				podLogs, err := speakerPod.GetFullLog("speaker")
				Expect(err).ToNot(HaveOccurred(), "Failed to get speaker pod logs")
				Expect(podLogs).Should(SatisfyAll(ContainSubstring("info"), Not(ContainSubstring("debug"))),
					"Pods logs should contain info logs only")
			}
		})

		It("Verify frrk8s debug logs", reportxml.ID("49812"), func() {
			By("Creating a new instance of MetalLB with Log level set to debug")
			err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap, "debug")
			Expect(err).ToNot(HaveOccurred(), "Failed to create a new instance of MetalLB with Log level set to debug")

			By("Fetch speaker pods from metallb-system namespace")
			speakerPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace,
				metav1.ListOptions{LabelSelector: tsparams.SpeakerLabel})
			Expect(err).ToNot(HaveOccurred(), "Failed to list speaker pods")
			Expect(len(speakerPods)).Should(BeNumerically(">", 0), "Speaker Pods List should not be empty")

			By("Verify loglevel in speaker pod logs")
			for _, speakerPod := range speakerPods {
				podLogs, err := speakerPod.GetFullLog("speaker")
				Expect(err).ToNot(HaveOccurred(), "Failed to get speaker pod logs")
				Expect(podLogs).Should(SatisfyAll(ContainSubstring("debug"), ContainSubstring("info")),
					"Pods logs should contain both info and debug logs")
			}
		})
	})

	Context("Updates", func() {
		DescribeTable("Verify bgp-advertisement updates", reportxml.ID("47178"),
			func(ipStack string, prefixLen int) {

				validateIPFamilySupport(ipStack)

				_, extFrrPod, bgpAdv := setupTestEnv(ipStack, prefixLen, false)

				By("Validating BGP route prefix")
				validatePrefix(
					extFrrPod, ipStack, prefixLen, removePrefixFromIPList(nodeAddrList[ipStack]), tsparams.LBipRange1[ipStack])

				By("Validate BGP Community is received on the External FRR Pod")
				bgpStatus, err := frr.GetBGPCommunityStatus(extFrrPod, tsparams.NoAdvertiseCommunity, strings.ToLower(ipStack))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(BeNumerically(">", 0),
					"Failed to fetch BGP routes with required Community")

				By("Validate BGP Local Preference received on External FRR Pod")
				bgpStatus, err = frr.GetBGPStatus(extFrrPod, strings.ToLower(ipStack))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp command output")
				for _, frrRoute := range bgpStatus.Routes {
					Expect(frrRoute[0].LocalPref).To(Equal(uint32(100)))
				}

				By("Update BGP Advertisements")
				switch ipStack {
				case ipv4:
					_, err = bgpAdv.
						WithLocalPref(200).
						WithAggregationLength4(28).
						WithCommunities([]string{tsparams.CustomCommunity}).
						Update(false)
				case ipv6:
					_, err = bgpAdv.
						WithLocalPref(200).
						WithAggregationLength6(64).
						WithCommunities([]string{tsparams.CustomCommunity}).
						Update(false)
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to update BGPAdvertisement")

				By("Validating updated BGP route prefix")
				var subnet *net.IPNet
				switch ipStack {
				case ipv4:
					_, subnet, err = net.ParseCIDR(tsparams.LBipRange1[ipStack][0] + "/28")
				case ipv6:
					_, subnet, err = net.ParseCIDR(tsparams.LBipRange1[ipStack][0] + "/64")
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to parse CIDR")

				Eventually(func() (map[string][]frr.Route, error) {
					bgpStatus, err := frr.GetBGPStatus(extFrrPod, strings.ToLower(ipStack), "test")
					if err != nil {
						return nil, err
					}

					return bgpStatus.Routes, nil
				}, time.Minute, tsparams.DefaultRetryInterval).Should(HaveKey(subnet.String()))

				By("Validate BGP Community received on External FRR Pod")
				bgpStatus, err = frr.GetBGPCommunityStatus(extFrrPod, tsparams.CustomCommunity, strings.ToLower(ipStack))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(BeNumerically(">", 0),
					"Failed to fetch BGP routes with required Community")

				By("Validate BGP Local Preference on External FRR Pod")
				bgpStatus, err = frr.GetBGPStatus(extFrrPod, strings.ToLower(ipStack))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp command output")
				for _, frrRoute := range bgpStatus.Routes {
					Expect(frrRoute[0].LocalPref).To(Equal(uint32(200)))
				}
			},
			Entry("", netparam.IPV4Family, 32,
				reportxml.SetProperty("IPStack", netparam.IPV4Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet32)),
			Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, 128,
				reportxml.SetProperty("IPStack", netparam.IPV6Family),
				reportxml.SetProperty("PrefixLength", netparam.IPSubnet128)),
		)

		It("BGP Timer update", reportxml.ID("47180"), func() {
			frrk8sPods, extFrrPod, _ := setupTestEnv(ipv4, 32, false)

			By("Verify BGP Timers of neighbors in external FRR Pod")
			verifyBGPTimer(extFrrPod, nodeAddrList[ipv4], 180000, 60000)

			By("Update BGP Timers")
			bgpPeer, err := metallb.PullBGPPeer(APIClient, tsparams.BgpPeerName1, NetConfig.MlbOperatorNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to fetch BGP peer")

			_, err = bgpPeer.WithHoldTime(metav1.Duration{Duration: 30000 * time.Millisecond}).
				WithKeepalive(metav1.Duration{Duration: 10000 * time.Millisecond}).
				Update(false)
			Expect(err).NotTo(HaveOccurred(), "Failed to update BGP peer")

			By("Verify Timers updated in frrk8s pods")
			for _, frrk8sPod := range frrk8sPods {
				Eventually(frr.CheckFRRConfigLine,
					time.Minute, tsparams.DefaultRetryInterval).WithArguments(frrk8sPod, " timers 10 30").
					Should(BeTrue(), "BFD is not configured on the Speakers")
			}

			By("Verify BGP Timers of neighbors in external FRR Pod are updated")
			err = frr.ResetBGPConnection(extFrrPod)
			Expect(err).NotTo(HaveOccurred(), "Failed to reset BGP connection")

			verifyBGPTimer(extFrrPod, nodeAddrList[ipv4], 30000, 10000)
		})
	})
})

// Deploys a basic test environment with a single BGPPeer and a single IPAddressPool
// and creates a BGPAdvertisement with the given prefix length and communities.
// ipStack is the IP family to use for the test. ipStack can be ipv4 or ipv6 only.
func setupTestEnv(ipStack string, prefixLen int, extFrrAdv bool) (
	[]*pod.Builder,
	*pod.Builder,
	*metallb.BGPAdvertisementBuilder,
) {
	By("Fetching frrk8s pods list running on the worker nodes selected for metallb tests")

	frrk8sPods := verifyAndCreateFRRk8sPodList()

	By("Creating BGPPeer with external FRR Pod")
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1,
		metallbAddrList[ipStack][0], "", tsparams.LocalBGPASN, false, 0, frrk8sPods)

	By("Creating an IPAddressPool")

	ipAddressPool, err := metallb.NewIPAddressPoolBuilder(
		APIClient,
		"address-pool",
		NetConfig.MlbOperatorNamespace,
		[]string{fmt.Sprintf("%s-%s", tsparams.LBipRange1[ipStack][0], tsparams.LBipRange1[ipStack][1])}).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool")

	By("Creating a BGPAdvertisement")

	bgpAdvertisement := metallb.
		NewBGPAdvertisementBuilder(APIClient, "bgpadvertisement", NetConfig.MlbOperatorNamespace).
		WithIPAddressPools([]string{ipAddressPool.Definition.Name}).
		WithCommunities([]string{tsparams.NoAdvertiseCommunity}).
		WithLocalPref(100)

	if ipStack == netparam.IPV6Family {
		bgpAdvertisement = bgpAdvertisement.WithAggregationLength6(int32(prefixLen))
	} else {
		bgpAdvertisement = bgpAdvertisement.WithAggregationLength4(int32(prefixLen))
	}

	bgpAdvertisement, err = bgpAdvertisement.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement")

	By("Deploy nginx on single worker with LB service")
	setupNGNXPod("nginxpod1", workerNodeList[0].Object.Name, tsparams.LabelValue1)
	setupMetalLbService(tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipAddressPool,
		corev1.ServiceExternalTrafficPolicyTypeCluster)

	By("Creating configMap with selected worker nodes as BGP Peers for external FRR Pod")

	var masterConfigMap *configmap.Builder

	if extFrrAdv {
		masterConfigMap = createConfigMapWithNetwork(
			ipStack, tsparams.LocalBGPASN, nodeAddrList[ipStack], tsparams.ExtFrrConnectedPools[ipStack])
	} else {
		masterConfigMap = createConfigMap(
			tsparams.LocalBGPASN, nodeAddrList[ipStack], false, false)
	}

	By("Creating macvlan NAD for external FRR Pod")

	err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a macvlan NAD")

	By("Creating external FRR Pod with configMap mount and external NAD")

	extFrrPod := createFrrPod(masterNodeList[0].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", metallbAddrList[ipStack][0], frrPodSubnet[ipStack])}))

	By("Checking that BGP session is established on external FRR Pod")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod, nodeAddrList[ipStack])

	return frrk8sPods, extFrrPod, bgpAdvertisement
}

func verifyBGPTimer(frrPod *pod.Builder, peerAddrList []string, hTimer, aTimer int) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Eventually(frr.VerifyBGPNeighborTimer,
			time.Minute*3, tsparams.DefaultRetryInterval).
			WithArguments(frrPod, peerAddress, hTimer, aTimer).Should(
			BeTrue(), "Failed to verify BGP Timer on peer")
	}
}

func createConfigMapWithNetwork(
	ipStack string,
	bgpAsn int,
	nodeAddrList, externalAdvertisedRoutes []string) *configmap.Builder {
	var frrBFDConfig string

	if ipStack == ipv6 {
		frrBFDConfig = frr.DefineBGPConfigWithIPv6Network(
			bgpAsn,
			tsparams.LocalBGPASN,
			externalAdvertisedRoutes,
			netcmd.RemovePrefixFromIPList(nodeAddrList),
			false,
			false,
		)
	} else {
		frrBFDConfig = frr.DefineBGPConfigWithIPv4Network(
			bgpAsn,
			tsparams.LocalBGPASN,
			externalAdvertisedRoutes,
			netcmd.RemovePrefixFromIPList(nodeAddrList),
			false,
			false,
		)
	}

	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrBFDConfig, "")

	masterConfigMap, err := configmap.NewBuilder(APIClient, "frr-master-node-config", tsparams.TestNamespaceName).
		WithData(configMapData).
		Create()

	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")

	return masterConfigMap
}
