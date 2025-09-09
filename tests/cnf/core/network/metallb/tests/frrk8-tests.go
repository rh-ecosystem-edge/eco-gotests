package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("FRR", Ordered, Label(tsparams.LabelFRRTestCases), ContinueOnFailure, func() {
	var (
		externalAdvertisedIPv4Routes = []string{"192.168.100.0/24", "192.168.200.0/24"}
		externalAdvertisedIPv6Routes = []string{"2001:100::0/64", "2001:200::0/64"}
		hubIPv4ExternalAddresses     = []string{"172.16.0.10", "172.16.0.11"}
		frrExternalMasterIPAddress   = "172.16.0.1"
		frrNodeSecIntIPv4Addresses   = []string{"10.100.100.254", "10.100.100.253"}
		hubSecIntIPv4Addresses       = []string{"10.100.100.131", "10.100.100.132"}
		hubPodWorker0                = "hub-pod-worker-0"
		hubPodWorker1                = "hub-pod-worker-1"
		frrCongigAllowAll            = "frrconfig-allow-all"
	)

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}
	})

	Context("IBGP Single hop", func() {

		var (
			nodeAddrList       []string
			addressPool        []string
			frrk8sPods         []*pod.Builder
			frrConfigFiltered1 = "frrconfig-filtered-1"
			frrConfigFiltered2 = "frrconfig-filtered-2"
			err                error
		)

		BeforeAll(func() {
			By("Setting test iteration parameters")
			_, _, _, nodeAddrList, addressPool, _, err =
				metallbenv.DefineIterationParams(
					ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
			Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")
		})

		AfterEach(func() {
			By("Clean metallb operator and test namespaces")
			resetOperatorAndTestNS()
		})

		It("Verify the FRR node only receives routes that are configured in the allowed prefixes",
			reportxml.ID("74272"), func() {
				prefixToFilter := externalAdvertisedIPv4Routes[1]

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")
				frrk8sPods = verifyAndCreateFRRk8sPodList()

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a frrconfiguration with prefix filter")

				createFrrConfiguration("frrconfig-filtered", ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, []string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]},
					false, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate BGP received routes")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				By("Validate BGP route is filtered")
				verifyBlockedRoutes(frrk8sPods, prefixToFilter)
			})

		It("Verify that when the allow all mode is configured all routes are received on the FRR speaker",
			reportxml.ID("74273"), func() {

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")
				frrk8sPods = verifyAndCreateFRRk8sPodList()

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a frrconfiguration allow all")
				createFrrConfiguration(frrCongigAllowAll, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					nil, false, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate both BGP routes are received")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
			})

		It("Verify that a FRR speaker can be updated by merging two different FRRConfigurations",
			reportxml.ID("74274"), func() {

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")
				frrk8sPods = verifyAndCreateFRRk8sPodList()

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create first frrconfiguration that receives a single route")
				createFrrConfiguration(frrConfigFiltered1, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					[]string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]}, false, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate BGP received only the first route")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])

				By("Validate the second BGP route not configured in the frrconfiguration is not received")
				verifyBlockedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])

				By("Create second frrconfiguration that receives a single route")
				createFrrConfiguration(frrConfigFiltered2, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					[]string{externalAdvertisedIPv4Routes[1], externalAdvertisedIPv6Routes[1]}, false,
					false)

				By("Validate BGP received both the first and second routes")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
			})

		It("Verify that a FRR speaker rejects a contrasting FRRConfiguration merge",
			reportxml.ID("74275"), func() {

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sWebhookDeployment deployment is in Ready state.")
				verifyAndCreateFRRk8sPodList()

				By("Create first frrconfiguration that receive a single route")
				createFrrConfiguration(frrConfigFiltered1, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					[]string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]}, false,
					false)

				By("Create second frrconfiguration fails when using an incorrect AS configuration")
				createFrrConfiguration(frrConfigFiltered2, ipv4metalLbIPList[0], tsparams.RemoteBGPASN,
					[]string{externalAdvertisedIPv4Routes[1], externalAdvertisedIPv6Routes[1]}, false,
					true)
			})

		It("Verify that the BGP status is correctly updated in the FRRNodeState",
			reportxml.ID("74280"), func() {

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")
				verifyAndCreateFRRk8sPodList()

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BGPTestPeer, ipv4metalLbIPList[0], "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Create first frrconfiguration that receives a single route")
				createFrrConfiguration(frrConfigFiltered1, ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, []string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]},
					false, false)

				By("Verify node state updates on worker node 0")
				Eventually(func() string {
					// Get the routes
					frrNodeState, err := metallb.ListFrrNodeState(APIClient, client.ListOptions{
						FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": workerNodeList[0].Object.Name})})
					Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

					return frrNodeState[0].Object.Status.RunningConfig

					// Return the routes to be checked
				}, 60*time.Second, 5*time.Second).Should(SatisfyAll(
					ContainSubstring(fmt.Sprintf("permit %s", externalAdvertisedIPv4Routes[0])),
					Not(ContainSubstring(fmt.Sprintf("permit %s", externalAdvertisedIPv4Routes[1]))),
				), "Fail to find all expected received routes")

				By("Create second frrconfiguration that receives a single route")
				createFrrConfiguration(frrConfigFiltered2, ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, []string{externalAdvertisedIPv4Routes[1], externalAdvertisedIPv6Routes[1]},
					false, false)

				By("Verify node state updates on worker node 1")
				Eventually(func() string {
					// Get the routes
					frrNodeState, err := metallb.ListFrrNodeState(APIClient, client.ListOptions{
						FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": workerNodeList[1].Object.Name})})
					Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

					return frrNodeState[0].Object.Status.RunningConfig

					// Return the routes to be checked
				}, 60*time.Second, 5*time.Second).Should(SatisfyAll(
					ContainSubstring(fmt.Sprintf("permit %s", externalAdvertisedIPv4Routes[0])),
					ContainSubstring(fmt.Sprintf("permit %s", externalAdvertisedIPv4Routes[1])),
				), "Fail to find all expected received routes")
			})
	})

	Context("BGP Multihop", func() {

		var (
			frrk8sPods        []*pod.Builder
			masterClientPodIP string
			nodeAddrList      []string
			addressPool       []string
		)

		BeforeEach(func() {
			By("Creating a new instance of MetalLB Speakers on workers")
			err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

			By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
				"worker nodes.")
			frrk8sPods = verifyAndCreateFRRk8sPodList()

			By("Setting test iteration parameters")
			masterClientPodIP, _, _, nodeAddrList, addressPool, _, err =
				metallbenv.DefineIterationParams(
					ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
			Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")

			By("Creating an IPAddressPool and BGPAdvertisement")
			ipAddressPool := setupBgpAdvertisementAndIPAddressPool(
				tsparams.BGPAdvAndAddressPoolName, addressPool, netparam.IPSubnetInt32)

			By("Creating a MetalLB service")
			setupMetalLbService(
				tsparams.MetallbServiceName,
				netparam.IPV4Family,
				tsparams.LabelValue1,
				ipAddressPool,
				corev1.ServiceExternalTrafficPolicyTypeCluster)

			By("Creating nginx test pod on worker node")
			setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
				workerNodeList[0].Definition.Name,
				tsparams.LabelValue1)
		})

		AfterAll(func() {
			By("Clean metallb operator and test namespaces")
			resetOperatorAndTestNS()
		})

		AfterEach(func() {
			By("Removing static routes from the speakers")
			frrk8sPods = verifyAndCreateFRRk8sPodList()
			speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList,
				[]string{ipv4metalLbIPList[0], ipv4metalLbIPList[1], frrNodeSecIntIPv4Addresses[0], frrNodeSecIntIPv4Addresses[1]})
			Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

			for _, frrk8sPod := range frrk8sPods {
				out, err := netenv.SetStaticRoute(frrk8sPod, "del", frrExternalMasterIPAddress,
					frrconfig.ContainerName, speakerRoutesMap)
				Expect(err).ToNot(HaveOccurred(), out)
			}

			srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Fail to set vlanID")

			By("Removing secondary interface on worker node 0")
			secIntWorker0Policy := nmstate.NewPolicyBuilder(APIClient, "sec-int-worker0", NetConfig.WorkerLabelMap).
				WithAbsentInterface(fmt.Sprintf("%s.%d", srIovInterfacesUnderTest[0], vlanID))
			err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, secIntWorker0Policy)
			Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

			By("Removing secondary interface on worker node 1")
			secIntWorker1Policy := nmstate.NewPolicyBuilder(APIClient, "sec-int-worker1", NetConfig.WorkerLabelMap).
				WithAbsentInterface(fmt.Sprintf("%s.%d", srIovInterfacesUnderTest[0], vlanID))
			err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, secIntWorker1Policy)
			Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

			By("Collect list of nodeNetworkConfigPolicies and delete them.")
			By("Removing NMState policies")
			err = nmstate.CleanAllNMStatePolicies(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

			By("Reset metallb operator namespaces")
			resetOperatorAndTestNS()
		})

		It("Validate a FRR node receives and sends IPv4 and IPv6 routes from an IBGP multihop FRR instance",
			reportxml.ID("74278"), func() {

				By("Adding static routes to the speakers")
				speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList,
					ipv4metalLbIPList)
				Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

				for _, frrk8sPod := range frrk8sPods {
					out, err := netenv.SetStaticRoute(frrk8sPod, "add", masterClientPodIP,
						frrconfig.ContainerName, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

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
				hubConfigMap := createHubConfigMap("frr-hub-node-config")

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name,
					hubConfigMap.Definition.Name,
					netparam.IPForwardAndSleepCmd,
					hub0BRstaticIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name,
					hubConfigMap.Definition.Name,
					netparam.IPForwardAndSleepCmd,
					hub1BRstaticIPAnnotation)

				By("Creating configmap and MetalLb Master pod")
				frrPod := createMasterFrrPod(tsparams.LocalBGPASN, frrExternalMasterIPAddress, nodeAddrList,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, false)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, frrExternalMasterIPAddress, "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a frrconfiguration allow all for EBGP multihop")
				createFrrConfiguration(frrCongigAllowAll, frrExternalMasterIPAddress,
					tsparams.LocalBGPASN, nil,
					false, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate that both BGP routes are received")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
			})

		It("Validate a FRR node receives and sends IPv4 and IPv6 routes from an EBGP multihop FRR instance",
			reportxml.ID("47279"), func() {

				By("Adding static routes to the speakers")
				speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList,
					ipv4metalLbIPList)
				Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

				for _, frrk8sPod := range frrk8sPods {
					out, err := netenv.SetStaticRoute(frrk8sPod, "add", masterClientPodIP,
						frrconfig.ContainerName, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

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
				hubConfigMap := createHubConfigMap("frr-hub-node-config")

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name,
					hubConfigMap.Definition.Name,
					netparam.IPForwardAndSleepCmd,
					hub0BRstaticIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name,
					hubConfigMap.Definition.Name,
					netparam.IPForwardAndSleepCmd,
					hub1BRstaticIPAnnotation)

				By("Creating MetalLb Master pod configMap")
				frrPod := createMasterFrrPod(tsparams.RemoteBGPASN, frrExternalMasterIPAddress, nodeAddrList,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, true)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, frrExternalMasterIPAddress, "",
					tsparams.RemoteBGPASN, true, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a frrconfiguration allow all for EBGP multihop")
				createFrrConfiguration(frrCongigAllowAll, frrExternalMasterIPAddress, tsparams.RemoteBGPASN,
					nil, true, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate that both BGP routes are received")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
			})
		It("Verify Frrk8 iBGP multihop over a secondary interface",
			reportxml.ID("75248"), func() {

				By("Collecting interface and VLAN information to create the secondary interface")
				srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

				vlanID, err := NetConfig.GetVLAN()
				Expect(err).ToNot(HaveOccurred(), "Fail to set vlanID")

				By("create a secondary IP address on the worker node 0")
				createSecondaryInterfaceOnNode("sec-int-worker0", workerNodeList[0].Definition.Name,
					srIovInterfacesUnderTest[0], frrNodeSecIntIPv4Addresses[0], "2001:100::254", vlanID)

				By("create a secondary IP address on the worker node 1")
				createSecondaryInterfaceOnNode("sec-int-worker1", workerNodeList[1].Definition.Name,
					srIovInterfacesUnderTest[0], frrNodeSecIntIPv4Addresses[1], "2001:100::253", vlanID)

				By("Adding static routes to the speakers")
				speakerRoutesMap, err := netenv.BuildRoutesMapWithSpecificRoutes(frrk8sPods, workerNodeList,
					hubSecIntIPv4Addresses)
				Expect(err).ToNot(HaveOccurred(), "Failed to create route map with specific routes")

				for _, frrk8sPod := range frrk8sPods {
					Eventually(func() error {
						out, err := netenv.SetStaticRoute(frrk8sPod, "add", fmt.Sprintf("%s/32",
							frrExternalMasterIPAddress), frrconfig.ContainerName, speakerRoutesMap)
						if err != nil {
							return fmt.Errorf("error adding static route: %s", out)
						}

						return nil
					}, time.Minute, 5*time.Second).Should(Succeed(),
						"Failed to add static route for pod %s", frrk8sPod.Definition.Name)
				}

				interfaceNameWithVlan := fmt.Sprintf("%s.%d", srIovInterfacesUnderTest[0], vlanID)

				By("Creating External NAD for hub FRR pods secondary interface")
				createExternalNadWithMasterInterface(tsparams.HubMacVlanNADSecIntName, interfaceNameWithVlan)

				By("Creating External NAD for master FRR pod")
				err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

				By("Creating External NAD for hub FRR pods")
				err = define.CreateExternalNad(APIClient, tsparams.HubMacVlanNADName, tsparams.TestNamespaceName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

				By("Creating MetalLb Hub pod configMap")
				createHubConfigMapSecInt := createHubConfigMap("frr-hub-node-config")

				By("Creating static ip annotation for hub0")
				hub0BRStaticSecIntIPAnnotation := frrconfig.CreateStaticIPAnnotations(tsparams.HubMacVlanNADSecIntName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/%s", hubSecIntIPv4Addresses[0], netparam.IPSubnet24)},
					[]string{fmt.Sprintf("%s/%s", hubIPv4ExternalAddresses[0], netparam.IPSubnet24)})

				By("Creating static ip annotation for hub1")
				hub1SecIntIPAnnotation := frrconfig.CreateStaticIPAnnotations(tsparams.HubMacVlanNADSecIntName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/%s", hubSecIntIPv4Addresses[1], netparam.IPSubnet24)},
					[]string{fmt.Sprintf("%s/%s", hubIPv4ExternalAddresses[1], netparam.IPSubnet24)})

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name, createHubConfigMapSecInt.Definition.Name, netparam.IPForwardAndSleepCmd,
					hub0BRStaticSecIntIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name, createHubConfigMapSecInt.Definition.Name, netparam.IPForwardAndSleepCmd,
					hub1SecIntIPAnnotation)

				By("Creating MetalLb Master pod configMap")
				frrPod := createMasterFrrPod(tsparams.LocalBGPASN, frrExternalMasterIPAddress, frrNodeSecIntIPv4Addresses,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, false)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, frrExternalMasterIPAddress, "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, frrNodeSecIntIPv4Addresses)
				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, frrNodeSecIntIPv4Addresses,
					addressPool)

				By("Create a frrconfiguration allow all for IBGP multihop")
				createFrrConfiguration(frrCongigAllowAll, frrExternalMasterIPAddress,
					tsparams.LocalBGPASN, nil,
					false, false)

				By("Validate that both BGP routes are received")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
			})
	})

})

func deployTestPods(addressPool, hubIPAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string) *pod.Builder {
	By("Creating an IPAddressPool and BGPAdvertisement")

	ipAddressPool := setupBgpAdvertisementAndIPAddressPool(tsparams.BGPAdvAndAddressPoolName,
		addressPool, netparam.IPSubnetInt32)

	By("Creating a MetalLB service")
	setupMetalLbService(
		tsparams.MetallbServiceName,
		netparam.IPV4Family,
		tsparams.LabelValue1,
		ipAddressPool,
		corev1.ServiceExternalTrafficPolicyTypeCluster)

	By("Creating nginx test pod on worker node")
	setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
		workerNodeList[0].Definition.Name,
		tsparams.LabelValue1)

	By("Creating External NAD")

	err := define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

	By("Creating static ip annotation")

	staticIPAnnotation := pod.StaticIPAnnotation(
		frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], netparam.IPSubnet24)})

	By("Creating MetalLb configMap")

	masterConfigMap := createConfigMapWithStaticRoutes(tsparams.LocalBGPASN, ipv4NodeAddrList, hubIPAddresses,
		externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, false, false)

	By("Creating FRR Pod")

	frrPod := createFrrPod(
		masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation)

	return frrPod
}

func createFrrConfiguration(name, bgpPeerIP string, remoteAS uint32, filteredIP []string, ebgp, expectToFail bool) {
	frrConfig := metallb.NewFrrConfigurationBuilder(APIClient, name,
		NetConfig.Frrk8sNamespace).
		WithBGPRouter(tsparams.LocalBGPASN).
		WithBGPNeighbor(bgpPeerIP, remoteAS, 0)

	// Check if there are filtered IPs and set the appropriate mode
	if len(filteredIP) > 0 {
		frrConfig.WithToReceiveModeFiltered(filteredIP, 0, 0)
	} else {
		frrConfig.WithToReceiveModeAll(0, 0)
	}

	// If eBGP is enabled, configure MultiHop
	if ebgp {
		frrConfig.WithEBGPMultiHop(0, 0)
	}

	// Set Password and Port
	frrConfig.
		WithBGPPassword("bgp-test", 0, 0).
		WithPort(179, 0, 0)

	if expectToFail {
		_, err := frrConfig.Create()

		Expect(err).To(HaveOccurred(), "Failed expected to not create a FRR configuration for %s", name)
	} else {
		_, err := frrConfig.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create FRR configuration for %s", name)
	}
}

func createMasterFrrPod(localAS int, frrExternalMasterIPAddress string, ipv4NodeAddrList,
	hubIPAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string, ebgpMultiHop bool) *pod.Builder {
	masterConfigMap := createConfigMapWithStaticRoutes(localAS, ipv4NodeAddrList, hubIPAddresses,
		externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, ebgpMultiHop, false)

	By("Creating static ip annotation for master FRR pod")

	masterStaticIPAnnotation := pod.StaticIPAnnotation(
		tsparams.HubMacVlanNADName, []string{fmt.Sprintf("%s/%s", frrExternalMasterIPAddress, netparam.IPSubnet24)})

	By("Creating FRR Master Pod")

	frrPod := createFrrPod(
		masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, masterStaticIPAnnotation)

	return frrPod
}

func createConfigMapWithStaticRoutes(
	bgpAsn int, nodeAddrList, hubIPAddresses, externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes []string,
	enableMultiHop, enableBFD bool) *configmap.Builder {
	frrBFDConfig := frr.DefineBGPConfigWithStaticRouteAndNetwork(
		bgpAsn, tsparams.LocalBGPASN, hubIPAddresses, externalAdvertisedIPv4Routes,
		externalAdvertisedIPv6Routes, netcmd.RemovePrefixFromIPList(nodeAddrList), enableMultiHop, enableBFD)
	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrBFDConfig, "")
	masterConfigMap, err := configmap.NewBuilder(APIClient, "frr-master-node-config", tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")

	return masterConfigMap
}

func verifyExternalAdvertisedRoutes(frrPod *pod.Builder, ipv4NodeAddrList, externalExpectedRoutes []string) {
	// Get advertised routes from FRR pod, now returned as a map of node IPs to their advertised routes
	advertisedRoutesMap, err := frr.GetBGPAdvertisedRoutes(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))
	Expect(err).ToNot(HaveOccurred(), "Failed to find advertised routes")

	// Iterate through each node in the advertised routes map
	for nodeIP, actualRoutes := range advertisedRoutesMap {
		// Split the string of advertised routes into a slice of routes
		routesSlice := strings.Split(strings.TrimSpace(actualRoutes), "\n")

		// Check that the actual routes for each node contain all the expected routes
		for _, expectedRoute := range externalExpectedRoutes {
			matched, err := ContainElement(expectedRoute).Match(routesSlice)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to match route %s for node %s", expectedRoute, nodeIP))

			Expect(matched).To(BeTrue(), fmt.Sprintf("Expected route %s not found for node %s", expectedRoute, nodeIP))
		}
	}
}

func createSecondaryInterfaceOnNode(policyName, nodeName, interfaceName, ipv4Address, ipv6Address string,
	vlanID uint16) {
	secondaryInterface := nmstate.NewPolicyBuilder(APIClient, policyName, map[string]string{
		corev1.LabelHostname: nodeName,
	})

	secondaryInterface.WithVlanInterfaceIP(interfaceName, ipv4Address, ipv6Address, vlanID)

	_, err := secondaryInterface.Create()
	Expect(err).ToNot(HaveOccurred(),
		"fail to create secondary interface: %s.+%d", interfaceName, vlanID)
}

func verifyReceivedRoutes(frrk8sPods []*pod.Builder, allowedPrefixes string) {
	By("Validate BGP received routes")

	Eventually(func() string {
		// Get the routes
		routes, err := frr.VerifyBGPReceivedRoutesOnFrrNodes(frrk8sPods)
		Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

		return routes
	}, 60*time.Second, 5*time.Second).Should(ContainSubstring(allowedPrefixes),
		"Failed to find all expected received route")
}

func verifyBlockedRoutes(frrk8sPods []*pod.Builder, blockedPrefixes string) {
	By("Validate BGP blocked routes")

	Eventually(func() string {
		// Get the routes
		routes, err := frr.VerifyBGPReceivedRoutesOnFrrNodes(frrk8sPods)
		Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

		return routes
	}, 60*time.Second, 5*time.Second).Should(Not(ContainSubstring(blockedPrefixes)),
		"Failed the blocked route was  received.")
}
