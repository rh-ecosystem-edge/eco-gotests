package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypesv1beta2"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
)

var _ = Describe("BGP remote-dynamicAS", Ordered, Label(tsparams.LabelDynamicRemoteASTestCases),
	ContinueOnFailure, func() {
		var (
			frrExternalMasterIPAddress   = "172.16.0.1"
			hubIPv4ExternalAddresses     = []string{"172.16.0.10", "172.16.0.11"}
			externalAdvertisedIPv4Routes = []string{"192.168.100.0/24", "192.168.200.0/24"}
			externalAdvertisedIPv6Routes = []string{"2001:100::0/64", "2001:200::0/64"}
		)

		BeforeAll(func() {
			validateEnvVarAndGetNodeList()
		})

		AfterAll(func() {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		})

		Context("single hop", func() {
			AfterEach(func() {
				By("Clean metallb operator and test namespaces")
				resetOperatorAndTestNS()
			})

			It("Verify the establishment of an eBGP adjacency using neighbor peer remote-as external",
				reportxml.ID("76821"), func() {
					By("Setup test cases with Frr Node AS 64500 and external Frr AS 64501")
					frrk8sPods, frrPod := setupBGPRemoteASTestCase(hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
						externalAdvertisedIPv6Routes, tsparams.BgpPeerDynamicASeBGP, tsparams.RemoteBGPASN)

					By("Checking that BGP session is established and up")
					verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

					By("Validating external FRR AS number received on the FRR nodes")
					Eventually(func() error {
						return frr.ValidateBGPRemoteAS(frrk8sPods, ipv4metalLbIPList[0], tsparams.RemoteBGPASN)
					}, 60*time.Second, 5*time.Second).Should(Succeed(),
						fmt.Sprintf("The remoteASN does not match the expected AS: %d", tsparams.RemoteBGPASN))
				})

			It("Verify the establishment of an iBGP adjacency using neighbor peer remote-as internal",
				reportxml.ID("76822"), func() {
					By("Setup test cases with Frr Node AS 64500 and external Frr AS 64500")
					frrk8sPods, frrPod := setupBGPRemoteASTestCase(hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
						externalAdvertisedIPv6Routes, tsparams.BgpPeerDynamicASiBGP, tsparams.LocalBGPASN)

					By("Checking that BGP session is established and up")
					verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

					By("Validating external FRR AS number received on the FRR nodes")
					Eventually(func() error {
						return frr.ValidateBGPRemoteAS(frrk8sPods, ipv4metalLbIPList[0], tsparams.LocalBGPASN)
					}, 60*time.Second, 5*time.Second).Should(Succeed(),
						fmt.Sprintf("The remoteASN does not match the expected AS: %d", tsparams.LocalBGPASN))
				})

			It("Verify the failure to establish a iBGP adjacency with a misconfigured external FRR pod",
				reportxml.ID("76825"), func() {
					By("Setup test cases with Frr Node AS 64500 and misconfigured iBGP external Frr AS 64501")
					frrk8sPods, frrPod := setupBGPRemoteASTestCase(hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
						externalAdvertisedIPv6Routes, tsparams.BgpPeerDynamicASiBGP, tsparams.RemoteBGPASN)

					By("Checking that BGP session is down")
					verifyMetalLbBGPSessionsAreDownOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

					By("Validating external FRR AS number received is incorrect and marked as 0 on the FRR nodes")
					Eventually(func() error {
						return frr.ValidateBGPRemoteAS(frrk8sPods, ipv4metalLbIPList[0], 0)
					}, 60*time.Second, 5*time.Second).Should(Succeed(),
						fmt.Sprintf("The remoteASN does not match the expected AS: %d", 0))
				})
		})

		Context("multi hop", func() {
			var (
				frrNodeSecIntIPv4Addresses = []string{"10.100.100.254", "10.100.100.253"}
				hubPodWorkerNames          = []string{"hub-pod-worker-0", "hub-pod-worker-1"}
			)

			AfterEach(func() {
				By("Removing static routes from the speakers")
				frrk8sPods := verifyAndCreateFRRk8sPodList()

				speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList,
					[]string{ipv4metalLbIPList[0], ipv4metalLbIPList[1], frrNodeSecIntIPv4Addresses[0],
						frrNodeSecIntIPv4Addresses[1]})
				Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

				for _, frrk8sPod := range frrk8sPods {
					out, err := netenv.SetStaticRoute(frrk8sPod, "del", frrExternalMasterIPAddress,
						frrconfig.ContainerName, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

				By("Clean metallb operator and test namespaces")
				resetOperatorAndTestNS()
			})

			It("Verify the establishment of a multi-hop iBGP adjacency using neighbor peer remote-as external",
				reportxml.ID("76823"), func() {
					frrPod, frrk8sPods := setupBGPRemoteASMultiHopTest(ipv4metalLbIPList, hubIPv4ExternalAddresses,
						externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, hubPodWorkerNames,
						frrExternalMasterIPAddress, tsparams.LocalBGPASN, false)

					By("Creating a BGP Peer with dynamicASN")
					createBGPPeerWithDynamicASN(frrExternalMasterIPAddress, tsparams.BgpPeerDynamicASiBGP, false)

					By("Checking that BGP session is established and up")
					verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

					By("Validating external FRR AS number received on the FRR nodes")
					Eventually(func() error {
						return frr.ValidateBGPRemoteAS(frrk8sPods, frrExternalMasterIPAddress, tsparams.LocalBGPASN)
					}, 60*time.Second, 5*time.Second).Should(Succeed(),
						fmt.Sprintf("The remoteASN does not match the expected AS: %d", tsparams.LocalBGPASN))
				})

			It("Verify the establishment of a multi-hop eBGP adjacency using neighbor peer remote-as external",
				reportxml.ID("76824"), func() {
					frrPod, frrk8sPods := setupBGPRemoteASMultiHopTest(ipv4metalLbIPList, hubIPv4ExternalAddresses,
						externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, hubPodWorkerNames,
						frrExternalMasterIPAddress, tsparams.RemoteBGPASN, true)

					By("Creating a BGP Peer with dynamicASN")
					createBGPPeerWithDynamicASN(frrExternalMasterIPAddress, tsparams.BgpPeerDynamicASeBGP, true)

					By("Checking that BGP session is established and up")
					verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

					By("Validating external FRR AS number received on the FRR nodes")
					Eventually(func() error {
						return frr.ValidateBGPRemoteAS(frrk8sPods, frrExternalMasterIPAddress, tsparams.RemoteBGPASN)
					}, 60*time.Second, 5*time.Second).Should(Succeed(),
						fmt.Sprintf("The remoteASN does not match the expected AS: %d", tsparams.RemoteBGPASN))
				})
		})
	})

func createBGPPeerWithDynamicASN(peerIP, dynamicASN string, eBgpMultiHop bool) {
	By("Creating BGP Peer")

	bgpPeer := metallb.NewBPGPeerBuilder(APIClient, "testpeer", NetConfig.MlbOperatorNamespace,
		peerIP, tsparams.LocalBGPASN, 0).WithDynamicASN(mlbtypesv1beta2.DynamicASNMode(dynamicASN)).
		WithPassword(tsparams.BGPPassword).WithEBGPMultiHop(eBgpMultiHop)

	_, err := bgpPeer.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BGP peer")
}

func deployFrrExternalPod(hubIPAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string, localAS int) *pod.Builder {
	By("Creating External NAD")

	err := define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

	By("Creating static ip annotation")

	staticIPAnnotation := pod.StaticIPAnnotation(
		frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], "24")})

	By("Creating MetalLb configMap")

	masterConfigMap := createConfigMapWithStaticRoutes(localAS, ipv4NodeAddrList, hubIPAddresses,
		externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, false, false)

	By("Creating FRR Pod")

	frrPod := createFrrPod(
		masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation)

	return frrPod
}

func setupBGPRemoteASTestCase(hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string, dynamicAS string, externalFrrAS int) ([]*pod.Builder, *pod.Builder) {
	By("Creating a new instance of MetalLB Speakers on workers")

	err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

	By("Collect connection information for the Frr Node pods")

	frrk8sPods := verifyAndCreateFRRk8sPodList()

	By("Collect connection information for the Frr external pod")

	frrPod := deployFrrExternalPod(hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
		externalAdvertisedIPv6Routes, externalFrrAS)

	By("Creating a BGP Peer with dynamicASN")
	createBGPPeerWithDynamicASN(ipv4metalLbIPList[0], dynamicAS, false)

	return frrk8sPods, frrPod
}

func setupBGPRemoteASMultiHopTest(ipv4metalLbIPList, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes, hubPodWorkerName []string, frrExternalMasterIPAddress string, asNumber int,
	eBGP bool) (*pod.Builder, []*pod.Builder) {
	By("Creating a new instance of MetalLB Speakers on workers")

	err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

	By("Collecting information before test")

	frrk8sPods := verifyAndCreateFRRk8sPodList()

	By("Setting test parameters")

	masterClientPodIP, _, _, nodeAddrList, _, _, err :=
		metallbenv.DefineIterationParams(
			ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
	Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")

	By("Creating External NAD for master FRR pod")

	err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

	By("Creating External NAD for hub FRR pods")

	err = define.CreateExternalNad(APIClient, tsparams.HubMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

	By("Creating static ip annotation for hub0")

	hub0BRstaticIPAnnotation := frrconfig.CreateStaticIPAnnotations(frrconfig.ExternalMacVlanNADName,
		tsparams.HubMacVlanNADName,
		[]string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], netparam.IPSubnet24)},
		[]string{fmt.Sprintf("%s/%s", hubIPv4ExternalAddresses[0], netparam.IPSubnet24)})

	By("Creating static ip annotation for hub1")

	hub1BRstaticIPAnnotation := frrconfig.CreateStaticIPAnnotations(frrconfig.ExternalMacVlanNADName,
		tsparams.HubMacVlanNADName,
		[]string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[1], netparam.IPSubnet24)},
		[]string{fmt.Sprintf("%s/%s", hubIPv4ExternalAddresses[1], netparam.IPSubnet24)})

	By("Creating MetalLb Hub pod configMap")

	hubConfigMap := createHubConfigMap("hub-node-config")

	By("Creating FRR Hub pod on worker node 0")

	_ = createFrrHubPod(hubPodWorkerName[0],
		workerNodeList[0].Object.Name, hubConfigMap.Definition.Name, netparam.IPForwardAndSleepCmd, hub0BRstaticIPAnnotation)

	By("Creating FRR Hub pod on worker node 1")

	_ = createFrrHubPod(hubPodWorkerName[1],
		workerNodeList[1].Object.Name, hubConfigMap.Definition.Name, netparam.IPForwardAndSleepCmd, hub1BRstaticIPAnnotation)

	By("Creating configmap and MetalLb Master pod")

	frrPod := createMasterFrrPod(asNumber, frrExternalMasterIPAddress, nodeAddrList, hubIPv4ExternalAddresses,
		externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, eBGP)

	By("Adding static routes to the speakers")

	speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList, ipv4metalLbIPList)
	Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

	for _, frrk8sPod := range frrk8sPods {
		out, err := netenv.SetStaticRoute(frrk8sPod, "add", masterClientPodIP, frrconfig.ContainerName,
			speakerRoutesMap)
		Expect(err).ToNot(HaveOccurred(), out)
	}

	return frrPod, frrk8sPods
}
