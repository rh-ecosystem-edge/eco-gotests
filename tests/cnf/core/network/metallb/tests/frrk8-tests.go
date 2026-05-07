package tests

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypes"
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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gomegatypes "github.com/onsi/gomega/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ovn"
	ovnv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/routeadvertisement/v1"
	ovntypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/types"
)

var _ = Describe("FRR", Ordered, Label(tsparams.LabelFRRTestCases), ContinueOnFailure, func() {
	var (
		externalAdvertisedIPv4Routes = []string{"192.168.100.0/24", "192.168.200.0/24"}
		externalAdvertisedIPv6Routes = []string{"2001:100::0/64", "2001:200::0/64"}
		hubIPv4ExternalAddresses     = []string{"172.16.0.10", "172.16.0.11"}
		frrExternalMasterIPAddress   = "172.16.0.1"
		frrNodeSecIntIPv4Addresses   = []string{"10.100.100.254", "10.100.100.253"}
		frrNodeSecIntIPv6Addresses   = []string{"2001:100::254", "2001:100::253"}
		hubSecIntIPv4Addresses       = []string{"10.100.100.131", "10.100.100.132"}
		hubPodWorker0                = "hub-pod-worker-0"
		hubPodWorker1                = "hub-pod-worker-1"
		frrCongigAllowAll            = "frrconfig-allow-all"
	)

	BeforeAll(func() {
		By("Checking if cluster is SNO")

		if IsSNO {
			Skip("Skipping test on SNO (Single Node OpenShift) cluster - requires 2+ workers")
		}

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
				validateBGPSessionState("Established", "N/A", ipv4metalLbIPList[0], workerNodeList)

				By("Validating the service BGP status")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

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
				validateBGPSessionState("Established", "N/A", ipv4metalLbIPList[0], workerNodeList)

				By("Validating the service BGP status")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a frrconfiguration allow all")
				createFrrConfiguration(frrCongigAllowAll, ipv4metalLbIPList[0], tsparams.LocalBGPASN, nil, false, false)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate both external BGP routes are received by FRR-K8s speakers")
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
				validateBGPSessionState("Established", "N/A", ipv4metalLbIPList[0], workerNodeList)

				By("Validating the service BGP status")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

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

				By("Verifying that the frrk8sStatuscleanerDeployment deployment is in Ready state.")
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
			By("Cleaning up any existing NMState policies from previous test runs")

			err := nmstate.CleanAllNMStatePolicies(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean up NMState policies")

			By("Creating a new instance of MetalLB Speakers on workers")

			err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
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
			validateAddressPool(tsparams.BGPAdvAndAddressPoolName, mlbtypes.IPAddressPoolStatus{
				AvailableIPv4: 240,
				AvailableIPv6: 0,
				AssignedIPv4:  0,
				AssignedIPv6:  0,
			})

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
			validateAddressPool(tsparams.BGPAdvAndAddressPoolName, mlbtypes.IPAddressPoolStatus{
				AvailableIPv4: 239,
				AvailableIPv6: 0,
				AssignedIPv4:  1,
				AssignedIPv6:  0,
			})
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

			By("Removing secondary IP addresses from worker nodes")
			removeSecondaryIPsFromWorkerNodes(workerNodeList, frrNodeSecIntIPv4Addresses, frrNodeSecIntIPv6Addresses)

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
				validateBGPSessionState("Established", "N/A", frrExternalMasterIPAddress, workerNodeList)

				By("Validating the service BGP statuses")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

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

				By("Validating the service BGP statuses")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))
				validateBGPSessionState("Established", "N/A", frrExternalMasterIPAddress, workerNodeList)

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
				By("Collecting SR-IOV interface for secondary network")

				srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

				By("Assign secondary IP address to base interface on worker node 0")
				addSecondaryIPToInterface("sec-int-worker0", workerNodeList[0].Definition.Name,
					srIovInterfacesUnderTest[0], frrNodeSecIntIPv4Addresses[0], frrNodeSecIntIPv6Addresses[0])

				By("Assign secondary IP address to base interface on worker node 1")
				addSecondaryIPToInterface("sec-int-worker1", workerNodeList[1].Definition.Name,
					srIovInterfacesUnderTest[0], frrNodeSecIntIPv4Addresses[1], frrNodeSecIntIPv6Addresses[1])

				secondaryInterfaceName := srIovInterfacesUnderTest[0]

				By("Verify secondary interfaces are UP with IP addresses on worker nodes")

				for _, workerNode := range workerNodeList {
					checkInterfaceExistsOnNode(workerNode.Definition.Name, secondaryInterfaceName)
				}

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

				By("Creating External NAD for hub FRR pods secondary interface")
				createExternalNadWithMasterInterface(tsparams.HubMacVlanNADSecIntName, secondaryInterfaceName)

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

				By("Validating the service BGP statuses")
				validateServiceBGPStatus(
					workerNodeList, tsparams.MetallbServiceName, tsparams.TestNamespaceName, []string{tsparams.BgpPeerName1})

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, frrNodeSecIntIPv4Addresses)
				validateBGPSessionState("Established", "N/A", frrExternalMasterIPAddress, workerNodeList)

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

	Context("OVN-K RouteAdvertisements", func() {
		var (
			nodeAddrList []string
			addressPool  []string
			frrk8sPods   []*pod.Builder
			err          error
		)

		BeforeAll(func() {
			By("Enabling OVN-K RouteAdvertisements on the cluster")
			enableOVNKRouteAdvertisements()

			By("Setting test iteration parameters")

			_, _, _, nodeAddrList, addressPool, _, err =
				metallbenv.DefineIterationParams(
					ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
			Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")
		})

		AfterAll(func() {
			By("Disabling OVN-K RouteAdvertisements on the cluster")
			disableOVNKRouteAdvertisements()
		})

		AfterEach(func() {
			By("Clean metallb operator and test namespaces")
			resetOperatorAndTestNS()

			By("Cleaning up RouteAdvertisement resources")
			cleanupRouteAdvertisements()
		})

		It("Verify OVN-K RouteAdvertisement advertises pod network routes to external FRR",
			reportxml.ID("86912"), func() {
				By("Creating a new instance of MetalLB Speakers on workers")

				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
					"worker nodes.")

				frrk8sPods = verifyAndCreateFRRk8sPodList()

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers with multipath disabled for RouteAdvertisement compatibility")
				createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
					tsparams.LocalBGPASN, false, 0, frrk8sPods, true)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32,
					removePrefixFromIPList(nodeAddrList), addressPool)

				By("Create a RouteAdvertisement-compatible FRR configuration")
				createOVNKCompatibleFRRConfiguration(frrCongigAllowAll, ipv4metalLbIPList[0], tsparams.LocalBGPASN)

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Creating OVN-K RouteAdvertisement for pod network")
				createOVNKRouteAdvertisementAndWaitUntilReady("test-routeadvertisement")

				By("Validate both external BGP routes are received by FRR-K8s speakers")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[0])
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])

				By("Verify external FRR pod receives OVN-K pod network routes via RouteAdvertisement")
				verifyOVNKPodNetworkRoutesReceived(frrPod)
			})
	})
})

func deployTestPods(addressPool, hubIPAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string) *pod.Builder {
	By("Creating an IPAddressPool and BGPAdvertisement")

	ipAddressPool := setupBgpAdvertisementAndIPAddressPool(tsparams.BGPAdvAndAddressPoolName,
		addressPool, netparam.IPSubnetInt32)
	validateAddressPool(tsparams.BGPAdvAndAddressPoolName, mlbtypes.IPAddressPoolStatus{
		AvailableIPv4: 240,
		AvailableIPv6: 0,
		AssignedIPv4:  0,
		AssignedIPv6:  0,
	})

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
	validateAddressPool(tsparams.BGPAdvAndAddressPoolName, mlbtypes.IPAddressPoolStatus{
		AvailableIPv4: 239,
		AvailableIPv6: 0,
		AssignedIPv4:  1,
		AssignedIPv6:  0,
	})

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

func addSecondaryIPToInterface(policyName, nodeName, interfaceName, ipv4Address, ipv6Address string) {
	secondaryInterface := nmstate.NewPolicyBuilder(APIClient, policyName, map[string]string{
		corev1.LabelHostname: nodeName,
	}).WithEthernetInterface(interfaceName, ipv4Address, ipv6Address)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, secondaryInterface)
	Expect(err).ToNot(HaveOccurred(),
		"fail to create NMState policy for interface: %s", interfaceName)
}

func removeSecondaryIPsFromWorkerNodes(workerNodeList []*nodes.Builder, ipv4Addresses, ipv6Addresses []string) {
	srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
	Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

	for idx, workerNode := range workerNodeList {
		if idx >= len(ipv4Addresses) {
			break
		}

		ipv4Addr := ipv4Addresses[idx]
		ipv6Addr := ipv6Addresses[idx]

		if len(srIovInterfacesUnderTest) > 0 {
			interfaceName := srIovInterfacesUnderTest[0]

			ipv4Cmd := fmt.Sprintf("ip addr del %s/24 dev %s 2>/dev/null || true", ipv4Addr, interfaceName)
			_, _ = netcmd.RunCommandOnHostNetworkPod(workerNode.Definition.Name,
				NetConfig.MlbOperatorNamespace, ipv4Cmd)

			ipv6Cmd := fmt.Sprintf("ip addr del %s/64 dev %s 2>/dev/null || true", ipv6Addr, interfaceName)
			_, _ = netcmd.RunCommandOnHostNetworkPod(workerNode.Definition.Name,
				NetConfig.MlbOperatorNamespace, ipv6Cmd)
		}
	}
}

func checkInterfaceExistsOnNode(nodeName, interfaceName string) {
	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull NodeNetworkState for node %s", nodeName)

	netInterface, err := nodeNetworkState.GetInterfaceType(interfaceName, "ethernet")
	Expect(err).ToNot(HaveOccurred(), "Interface %s not found on node %s", interfaceName, nodeName)

	Expect(netInterface.State).To(Equal("up"),
		"Interface %s is not UP on node %s (current state: %s)", interfaceName, nodeName, netInterface.State)

	hasIPv4 := netInterface.Ipv4.Enabled && len(netInterface.Ipv4.Address) > 0
	hasIPv6 := netInterface.Ipv6.Enabled && len(netInterface.Ipv6.Address) > 0

	Expect(hasIPv4 || hasIPv6).To(BeTrue(),
		"Interface %s does not have an IP address assigned on node %s", interfaceName, nodeName)
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

func createOVNKRouteAdvertisementAndWaitUntilReady(name string) {
	By(fmt.Sprintf("Creating OVN RouteAdvertisement '%s' for PodNetwork", name))

	// Define advertisement types
	advertisements := []ovnv1.AdvertisementType{ovnv1.PodNetwork}

	// Create selectors to match YAML spec
	nodeSelector := metav1.LabelSelector{}
	frrConfigurationSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"routeadvertisement.k8s.ovn.org": "enabled",
		},
	}
	networkSelectors := ovntypes.NetworkSelectors{
		{NetworkSelectionType: ovntypes.DefaultNetwork},
	}

	routeAdv, err := ovn.NewRouteAdvertisementBuilder(
		APIClient,
		name).
		WithAdvertisements(advertisements).
		WithNodeSelector(nodeSelector).
		WithFRRConfigurationSelector(frrConfigurationSelector).
		WithNetworkSelectors(networkSelectors).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create RouteAdvertisement")

	By("Waiting for RouteAdvertisement to be accepted")

	Eventually(func() string {
		refreshed, err := routeAdv.Get()
		if err != nil {
			return ""
		}

		return refreshed.Status.Status
	}, time.Minute, 5*time.Second).Should(Equal("Accepted"),
		"RouteAdvertisement should reach Accepted status")
}

// createOVNKCompatibleFRRConfiguration creates an FRR config with multipath disabled for RouteAdvertisement.
func createOVNKCompatibleFRRConfiguration(name, bgpPeerIP string, remoteAS uint32) {
	By("Creating RouteAdvertisement-compatible FRR Configuration")

	frrConfig := metallb.NewFrrConfigurationBuilder(APIClient, name,
		NetConfig.Frrk8sNamespace).
		WithBGPRouter(tsparams.LocalBGPASN).
		WithBGPNeighbor(bgpPeerIP, remoteAS, 0).
		WithToReceiveModeAll(0, 0).
		WithBGPPassword("bgp-test", 0, 0).
		WithPort(179, 0, 0).
		WithBGPNeighborDisableMP(true, 0, 0)

	// Add label for RouteAdvertisement selector matching
	frrConfig.Definition.Labels = map[string]string{
		"routeadvertisement.k8s.ovn.org": "enabled",
	}

	_, err := frrConfig.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create RouteAdvertisement-compatible FRR configuration")
}

// getClusterPodNetworkCIDR retrieves the IPv4 cluster pod network CIDR from the network operator configuration.
func getClusterPodNetworkCIDR() (string, uint32) {
	By("Getting cluster network configuration from network operator")

	clusterNetwork, err := cluster.GetOCPNetworkOperatorConfig(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to get network operator config")

	networkObj, err := clusterNetwork.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to get network object")
	Expect(networkObj.Spec.ClusterNetwork).ToNot(BeEmpty(), "No cluster networks defined in network operator")

	var (
		clusterCIDR string
		hostPrefix  uint32
	)

	for _, clusterNet := range networkObj.Spec.ClusterNetwork {
		// Check if this is an IPv4 CIDR (doesn't contain ":")
		if !strings.Contains(clusterNet.CIDR, ":") {
			clusterCIDR = clusterNet.CIDR
			hostPrefix = clusterNet.HostPrefix

			break
		}
	}

	Expect(clusterCIDR).ToNot(BeEmpty(), "No IPv4 cluster network found in network operator config")

	klog.V(90).Infof("Cluster pod network CIDR: %s with host prefix: /%d", clusterCIDR, hostPrefix)

	return clusterCIDR, hostPrefix
}

// verifyOVNKPodNetworkRoutesReceived verifies that the external FRR pod receives
// pod network routes advertised by OVN-K RouteAdvertisement within the cluster network range.
func verifyOVNKPodNetworkRoutesReceived(frrPod *pod.Builder) {
	By("Verifying external FRR pod receives OVN-K pod network routes from RouteAdvertisement")

	clusterCIDR, hostPrefix := getClusterPodNetworkCIDR()

	By(fmt.Sprintf("Expecting pod network routes within %s (with /%d subnets per node)", clusterCIDR, hostPrefix))

	// Parse the cluster CIDR to extract the base network for matching
	// Example: "10.128.0.0/14" -> we'll look for routes starting with "10.128.", "10.129.", "10.130.", "10.131."
	cidrParts := strings.Split(clusterCIDR, "/")
	Expect(cidrParts).To(HaveLen(2), "invalid CIDR format: %s", clusterCIDR)

	baseIP := cidrParts[0]

	ipParts := strings.Split(baseIP, ".")
	Expect(ipParts).To(HaveLen(4), "invalid IP format: %s", baseIP)

	// Calculate the number of /16 ranges based on the CIDR prefix length
	// For a /14 network like 10.128.0.0/14, valid ranges are 10.128-131.x.x (4 ranges)
	// For a /16 network like 10.128.0.0/16, valid range is just 10.128.x.x (1 range)
	cidrMaskBits, err := strconv.Atoi(cidrParts[1])
	Expect(err).ToNot(HaveOccurred(), "Failed to parse CIDR mask bits")

	numRanges := 1 << (16 - cidrMaskBits) // 2^(16 - maskBits) gives number of /16 ranges
	if numRanges > 4 {
		numRanges = 4 // Cap at 4 for practical checking
	}

	if numRanges < 1 {
		numRanges = 1
	}

	secondOctet, err := strconv.Atoi(ipParts[1])
	Expect(err).ToNot(HaveOccurred(), "Failed to parse second octet")

	// Build matchers dynamically based on calculated range
	matchers := make([]gomegatypes.GomegaMatcher, numRanges)
	for i := range numRanges {
		matchers[i] = ContainSubstring(fmt.Sprintf("%s.%d.", ipParts[0], secondOctet+i))
	}

	Eventually(func() string {
		output, err := frrPod.ExecCommand([]string{"vtysh", "-c", "show ip bgp"})
		if err != nil {
			By(fmt.Sprintf("Failed to get BGP routes from external FRR pod: %v", err))

			return ""
		}

		routes := output.String()

		return routes
	}, 3*time.Minute, 15*time.Second).Should(SatisfyAny(matchers...),
		fmt.Sprintf("External FRR pod should receive pod network routes (/%d subnets) "+
			"from cluster network %s via OVN-K RouteAdvertisement", hostPrefix, clusterCIDR))
}

// enableOVNKRouteAdvertisements enables RouteAdvertisements on the OVN-K network configuration.
func enableOVNKRouteAdvertisements() {
	By("Enabling RouteAdvertisements on network operator")

	networkOperator, err := network.PullOperator(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

	_, err = networkOperator.SetRouteAdvertisements(operatorv1.RouteAdvertisementsEnabled, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to enable RouteAdvertisements on network operator")
}

// disableOVNKRouteAdvertisements disables RouteAdvertisements on the OVN-K network configuration.
func disableOVNKRouteAdvertisements() {
	By("Disabling RouteAdvertisements on network operator")

	networkOperator, err := network.PullOperator(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

	_, err = networkOperator.SetRouteAdvertisements(operatorv1.RouteAdvertisementsDisabled, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to disable RouteAdvertisements on network operator")
}

// cleanupRouteAdvertisements removes test-created RouteAdvertisement resources from the cluster.
func cleanupRouteAdvertisements() {
	By("Cleaning up RouteAdvertisement resources")

	listRA, err := ovn.ListRouteAdvertisements(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to list RouteAdvertisements")

	for _, routeAdv := range listRA {
		// Only delete test-created RouteAdvertisements (prefix "test-")
		if !strings.HasPrefix(routeAdv.Definition.Name, "test-") {
			continue
		}

		err := routeAdv.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to delete RouteAdvertisement %s", routeAdv.Definition.Name))
	}
}
