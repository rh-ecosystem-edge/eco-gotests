package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MetalLB NodeSelector", Ordered, Label(tsparams.LabelBGPTestCases), ContinueOnFailure, func() {
	var (
		nodeAddrList       []string
		addressPool        []string
		frrk8sPods         []*pod.Builder
		frrPod0            *pod.Builder
		frrPod1            *pod.Builder
		ipAddressPool      *metallb.IPAddressPoolBuilder
		worker0NodeLabel   []metav1.LabelSelector
		worker1NodeLabel   []metav1.LabelSelector
		err                error
		metalLbTestsLabel2 = map[string]string{"metallb2": "metallbtests"}
	)

	const (
		ipaddressPoolName1    = "ipaddresspool1"
		ipaddressPoolName2    = "ipaddresspool2"
		bgpAdvertisementName1 = "bgpadvertisement1"
		bgpAdvertisementName2 = "bgpadvertisement2"
	)

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Collecting information before test")
		frrk8sPods, err = pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
			LabelSelector: tsparams.LabelFRRNode,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to list frrk8s pods")

		By("Setting test iteration parameters")
		_, _, _, nodeAddrList, addressPool, _, err =
			metallbenv.DefineIterationParams(
				ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
		Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")

		worker0NodeLabel = []metav1.LabelSelector{
			{MatchLabels: map[string]string{corev1.LabelHostname: workerNodeList[0].Definition.Name}},
		}

		worker1NodeLabel = []metav1.LabelSelector{
			{MatchLabels: map[string]string{corev1.LabelHostname: workerNodeList[1].Definition.Name}},
		}
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}
	})

	AfterEach(func() {
		By("Clean metallb operator and test namespaces")
		resetOperatorAndTestNS()
	})

	Context("Single IPAddressPool", func() {
		BeforeEach(func() {
			By("Creating a new instance of MetalLB Speakers on workers")
			err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

			By("Create a single IPAddressPool")
			ipAddressPool = createIPAddressPool(ipaddressPoolName1, addressPool)

			By("Setup test case with services, test pods and bgppeers")
			frrPod0, frrPod1 = setupTestCase(ipAddressPool, ipAddressPool, frrk8sPods)
		})

		It("Advertise a single IPAddressPool with different attributes using the node selector option",
			reportxml.ID("53987"), func() {

				By(fmt.Sprintf("Creating a BGPAdvertisement with nodeSelector for bgpPeer1 and LocalPref set to "+
					"200 and community %s", tsparams.NoAdvertiseCommunity))

				setupBgpAdvertisement(bgpAdvertisementName1, tsparams.NoAdvertiseCommunity, ipaddressPoolName1, 200,
					[]string{tsparams.BgpPeerName1}, worker0NodeLabel)

				By(fmt.Sprintf("Creating a BGPAdvertisement with nodeSelector for bgpPeer2 and LocalPref set "+
					"to 100 and community to %s", tsparams.CustomCommunity))

				setupBgpAdvertisement(bgpAdvertisementName2, tsparams.CustomCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName2}, worker1NodeLabel)

				verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1, nodeAddrList, addressPool, addressPool)

				By(fmt.Sprintf("Validate BGP Custom Community %s exists with the node selector",
					tsparams.CustomCommunity))
				bgpStatus, err := frr.GetBGPCommunityStatus(frrPod0, tsparams.NoAdvertiseCommunity,
					strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(Equal(2))

				By(fmt.Sprintf("Validate BGP Custom Community %s exists with the node selector",
					tsparams.CustomCommunity))
				bgpStatus, err = frr.GetBGPCommunityStatus(frrPod1, tsparams.CustomCommunity,
					strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(Equal(2))
			})

		It("Update the node selector option with a label that does not exist",
			reportxml.ID("61336"), func() {

				By("Create a bgpadvertisement for BGPPeer1 and IP AddressPool1.")
				setupBgpAdvertisement(bgpAdvertisementName1, tsparams.NoAdvertiseCommunity, ipaddressPoolName1, 100,
					[]string{tsparams.BgpPeerName1}, worker0NodeLabel)

				By("Create a bgpadvertisement for BGPPeer2 and IP AddressPool1.")
				setupBgpAdvertisement(bgpAdvertisementName2, tsparams.CustomCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName2}, worker1NodeLabel)

				By("Verify BGP Establishement and service route advertisement.")
				verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1, nodeAddrList, addressPool, addressPool)

				By("Create a non existing label to use in test case.")
				nonExistingLabel := []metav1.LabelSelector{
					{MatchLabels: map[string]string{corev1.LabelHostname: "non-existing-label"}},
				}

				By("Update bgpadvertisement2 node selector option with a non existing node label.")
				updateBgpAdvertisementWithNodeSelector(bgpAdvertisementName2, nonExistingLabel)

				verifyBGPStatusAndRouteAfterLabelUpdate(frrPod0, frrPod1, removePrefixFromIPList(ipv4NodeAddrList),
					addressPool)
			})

		It("Remove from node label used in the node selector option",
			reportxml.ID("53991"), func() {

				By("Adding test label to compute nodes")
				addNodeLabel(workerNodeList, metalLbTestsLabel2)

				testMetallbNodeLabel := []metav1.LabelSelector{
					{MatchLabels: metalLbTestsLabel2},
				}

				By("Create a bgpadvertisement for BGPPeer1 and IP AddressPool1.")
				setupBgpAdvertisement(bgpAdvertisementName1, tsparams.NoAdvertiseCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName1}, worker0NodeLabel)

				By("Create a bgpadvertisement for BGPPeer2 and IP AddressPool1.")
				setupBgpAdvertisement(bgpAdvertisementName2, tsparams.CustomCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName2}, testMetallbNodeLabel)

				By("Verify BGP Establishement and service route advertisement.")
				verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1, nodeAddrList, addressPool, addressPool)

				By("Remove custom metallb test label from nodes")
				removeNodeLabel(workerNodeList, metalLbTestsLabel2)

				verifyBGPStatusAndRouteAfterLabelUpdate(frrPod0, frrPod1, removePrefixFromIPList(ipv4NodeAddrList),
					addressPool)
			})
	})

	Context("Dual IPAddressPools", func() {
		var addressPool2 = []string{"4.4.4.1", "4.4.4.240"}
		BeforeEach(func() {
			By("Creating a new instance of MetalLB Speakers on workers")
			err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

			By("Create two IPAddressPools")

			ipAddressPool1 := createIPAddressPool(ipaddressPoolName1, addressPool)
			ipAddressPool2 := createIPAddressPool(ipaddressPoolName2, addressPool2)

			By("Setup test case with services, test pods and bgppeers")
			frrPod0, frrPod1 = setupTestCase(ipAddressPool1, ipAddressPool2, frrk8sPods)
		})

		It("Advertise separate IPAddressPools using the node selector",
			reportxml.ID("53986"), func() {

				By("Creating a BGPAdvertisement with the nodeSelector to bgppeer1")
				setupBgpAdvertisement(bgpAdvertisementName1, tsparams.NoAdvertiseCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName1}, worker0NodeLabel)

				By("Creating a BGPAdvertisement with the nodeSelector to bgppeer2")
				setupBgpAdvertisement(bgpAdvertisementName2, tsparams.CustomCommunity, ipaddressPoolName2,
					200, []string{tsparams.BgpPeerName2}, worker1NodeLabel)

				verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1, nodeAddrList, addressPool, addressPool2)

				By("Validate Local Preference from Frr node0")
				err = frr.ValidateLocalPref(frrPod0, 100, strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Fail to validate local preference")

				By("Validate Local Preference from Frr node1")
				err = frr.ValidateLocalPref(frrPod1, 200, strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Fail to validate local preference")

				By(fmt.Sprintf("Validate BGP Community %s exists on received route prefix",
					tsparams.NoAdvertiseCommunity))
				bgpStatus, err := frr.GetBGPCommunityStatus(frrPod0, tsparams.NoAdvertiseCommunity,
					strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(Equal(1))

				By(fmt.Sprintf("Validate BGP Community %s exists on received route prefix",
					tsparams.CustomCommunity))
				bgpStatus, err = frr.GetBGPCommunityStatus(frrPod1, tsparams.CustomCommunity,
					strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Failed to collect bgp community status")
				Expect(len(bgpStatus.Routes)).To(Equal(1))
			})

		It("Update the node selector option with a label that does not exist",
			reportxml.ID("53989"), func() {

				By("Creating a BGPAdvertisement with the nodeSelector to bgppeer1")
				setupBgpAdvertisement(bgpAdvertisementName1, tsparams.NoAdvertiseCommunity, ipaddressPoolName1,
					100, []string{tsparams.BgpPeerName1}, worker0NodeLabel)

				By("Creating a BGPAdvertisement with the nodeSelector to bgppeer2")
				setupBgpAdvertisement(bgpAdvertisementName2, tsparams.CustomCommunity, ipaddressPoolName2,
					200, []string{tsparams.BgpPeerName2}, worker1NodeLabel)

				verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1, nodeAddrList, addressPool, addressPool2)

				By("Validate Local Preference from Frr node0")
				err = frr.ValidateLocalPref(frrPod0, 100, strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Fail to validate local preference")

				By("Validate Local Preference from Frr node1")
				err = frr.ValidateLocalPref(frrPod1, 200, strings.ToLower(netparam.IPV4Family))
				Expect(err).ToNot(HaveOccurred(), "Fail to validate local preference")

				By("Create a non existing label to use in test case.")
				nonExistingLabel := []metav1.LabelSelector{
					{MatchLabels: map[string]string{corev1.LabelHostname: "non-existing-label"}},
				}

				By("Update bgpadvertisement2 node selector option with a non existing node label.")
				updateBgpAdvertisementWithNodeSelector(bgpAdvertisementName2, nonExistingLabel)

				verifyBGPStatusAndRouteAfterLabelUpdate(frrPod0, frrPod1, removePrefixFromIPList(ipv4NodeAddrList),
					addressPool)
			})
	})
})

func createIPAddressPool(name string, ipPrefix []string) *metallb.IPAddressPoolBuilder {
	var ipPools []string

	Expect(len(ipPrefix)).To(BeNumerically(">", 0), "IPPrefix must contain at least one start-end pair")
	Expect(len(ipPrefix)%2).To(Equal(0), "IPPrefix length must be even (start,end pairs)")

	for i := 0; i < len(ipPrefix); i += 2 {
		ipPools = append(ipPools, fmt.Sprintf("%s-%s", ipPrefix[i], ipPrefix[i+1]))
	}

	ipAddressPool, err := metallb.NewIPAddressPoolBuilder(
		APIClient,
		name,
		NetConfig.MlbOperatorNamespace,
		ipPools).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool")

	return ipAddressPool
}

func setupTestCase(ipAddressPool1, ipAddressPool2 *metallb.IPAddressPoolBuilder,
	frrk8sPods []*pod.Builder) (*pod.Builder, *pod.Builder) {
	By("Creating two MetalLB service")

	setupMetalLbService(
		tsparams.MetallbServiceName,
		netparam.IPV4Family,
		tsparams.LabelValue1,
		ipAddressPool1,
		corev1.ServiceExternalTrafficPolicyTypeCluster)
	setupMetalLbService(
		tsparams.MetallbServiceName2,
		netparam.IPV4Family,
		tsparams.LabelValue1,
		ipAddressPool2,
		corev1.ServiceExternalTrafficPolicyTypeCluster)

	By("Creating nginx test pod on worker node 0")
	setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
		workerNodeList[0].Definition.Name,
		tsparams.LabelValue1)

	By("Creating nginx test pod on worker node 1")
	setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[1].Definition.Name,
		workerNodeList[1].Definition.Name,
		tsparams.LabelValue1)

	By("Creating External NAD for master FRR pods")
	createExternalNad(tsparams.ExternalMacVlanNADName)

	By("Creating static ip annotation for FRR master 0")

	staticIPAnnotation0 := pod.StaticIPAnnotation(
		tsparams.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0],
			netparam.IPSubnet24)})

	By("Creating static ip annotation for FRR master 1")

	staticIPAnnotation1 := pod.StaticIPAnnotation(
		tsparams.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[1],
			netparam.IPSubnet24)})

	By("Creating MetalLb configMap for FRR master pods")

	masterConfigMap := createConfigMap(tsparams.LocalBGPASN, ipv4NodeAddrList, false, false)

	By("Create FRR Pod on Master 0")

	frrPod0 := createFrrPod(
		masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation0, "frr-master0")

	By("Create FRR Pod on Master 1")

	frrPod1 := createFrrPod(
		masterNodeList[1].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation1, "frr-master1")

	By("Create two BGPPeers")
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
		false, 0, frrk8sPods)

	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName2, ipv4metalLbIPList[1], "", tsparams.LocalBGPASN,
		false, 0, frrk8sPods)

	return frrPod0, frrPod1
}

func verifyBGPConnectivityAndPrefixes(frrPod0, frrPod1 *pod.Builder, nodeAddrList, addressPool1,
	addressPool2 []string) {
	By("Checking that BGP session is established on Frr Master 0")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod0, removePrefixFromIPList(ipv4NodeAddrList))

	By("Checking that BGP session is established on Frr Master 1")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod1, removePrefixFromIPList(ipv4NodeAddrList))

	By("Validating BGP route prefix on Frr Master 0")
	validatePrefix(frrPod0, netparam.IPV4Family, netparam.IPSubnetInt32,
		removePrefixFromIPList([]string{nodeAddrList[0]}), addressPool1)

	By("Validating BGP route prefix on Frr Master 1")
	validatePrefix(frrPod1, netparam.IPV4Family, netparam.IPSubnetInt32,
		removePrefixFromIPList([]string{nodeAddrList[1]}), addressPool2)
}

func verifyBGPStatusAndRouteAfterLabelUpdate(frrPod0, frrPod1 *pod.Builder, nodeAddrList, addressPool []string) {
	By("Checking that BGP session is established on Frr Master 0")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod0, removePrefixFromIPList(ipv4NodeAddrList))

	By("Checking that BGP session is established on Frr Master 1")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod1, removePrefixFromIPList(ipv4NodeAddrList))

	By("Validating BGP route prefix on Frr Master 0")
	validatePrefix(frrPod0, netparam.IPV4Family, netparam.IPSubnetInt32,
		removePrefixFromIPList([]string{nodeAddrList[0]}), []string{addressPool[0]})

	By("Validating BGP route prefix on Frr Master 1")
	validatePrefix(frrPod1, netparam.IPV4Family, netparam.IPSubnetInt32,
		removePrefixFromIPList([]string{nodeAddrList[1]}), []string{addressPool[1]}, true)
}
