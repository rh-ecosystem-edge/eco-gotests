package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		frrK8WebHookServer           = "frr-k8s-webhook-server"
		frrCongigAllowAll            = "frrconfig-allow-all"
		frrNodeLabel                 = "app=frr-k8s"
		err                          error
	)

	BeforeAll(func() {

		By("Getting MetalLb load balancer ip addresses")
		ipv4metalLbIPList, ipv6metalLbIPList, err = metallbenv.GetMetalLbIPByIPStack()
		Expect(err).ToNot(HaveOccurred(), tsparams.MlbAddressListError)

		By("List CNF worker nodes in cluster")
		cnfWorkerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

		By("Selecting worker node for BGP tests")
		workerLabelMap, workerNodeList = setWorkerNodeListAndLabelForBfdTests(cnfWorkerNodeList, metalLbTestsLabel)
		ipv4NodeAddrList, err = nodes.ListExternalIPv4Networks(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(workerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to collect external nodes ip addresses")

		err = metallbenv.IsEnvVarMetalLbIPinNodeExtNetRange(ipv4NodeAddrList, ipv4metalLbIPList, nil)
		Expect(err).ToNot(HaveOccurred(), "Failed to validate metalLb exported ip address")

		By("Listing master nodes")
		masterNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to list master nodes")
		Expect(len(masterNodeList)).To(BeNumerically(">", 0),
			"Failed to detect master nodes")
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

		It("Verify that prefixes configured with alwaysBlock are not received by the FRR speakers",
			reportxml.ID("74270"), func() {
				prefixToBlock := externalAdvertisedIPv4Routes[0]

				By("Creating a new instance of MetalLB Speakers on workers blocking specific incoming prefixes")
				createNewMetalLbDaemonSetAndWaitUntilItsRunningWithAlwaysBlock(tsparams.DefaultTimeout,
					workerLabelMap, []string{prefixToBlock})

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

				By("Creating external FRR pod on master node")
				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList), addressPool, 32)

				By("Create a frrconfiguration allow all")
				createFrrConfiguration(frrCongigAllowAll, ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, nil, false, false)

				frrk8sPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
					LabelSelector: frrNodeLabel,
				})
				Expect(err).ToNot(HaveOccurred(), "Fail to find Frrk8 pod list")

				By("Verify that the node FRR pods advertises two routes")
				verifyExternalAdvertisedRoutes(frrPod, ipv4NodeAddrList, externalAdvertisedIPv4Routes)

				By("Validate that only the allowed route was received")
				verifyReceivedRoutes(frrk8sPods, externalAdvertisedIPv4Routes[1])
				By("Validate that only the route not allowed was blocked")
				verifyBlockedRoutes(frrk8sPods, prefixToBlock)
			})

		It("Verify the FRR node only receives routes that are configured in the allowed prefixes",
			reportxml.ID("74272"), func() {
				prefixToFilter := externalAdvertisedIPv4Routes[1]

				By("Creating a new instance of MetalLB Speakers on workers")
				err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList), addressPool, 32)

				By("Create a frrconfiguration with prefix filter")

				createFrrConfiguration("frrconfig-filtered", ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, []string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]},
					false, false)

				frrk8sPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
					LabelSelector: frrNodeLabel,
				})
				Expect(err).ToNot(HaveOccurred(), "Fail to find Frrk8 pod list")

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

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList), addressPool, 32)

				By("Create a frrconfiguration allow all")
				createFrrConfiguration(frrCongigAllowAll, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					nil, false, false)

				frrk8sPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
					LabelSelector: frrNodeLabel,
				})
				Expect(err).ToNot(HaveOccurred(), "Fail to find Frrk8 pod list")

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

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

				frrPod := deployTestPods(addressPool, hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList), addressPool, 32)

				By("Create first frrconfiguration that receieves a single route")
				createFrrConfiguration(frrConfigFiltered1, ipv4metalLbIPList[0], tsparams.LocalBGPASN,
					[]string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]}, false, false)

				frrk8sPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
					LabelSelector: frrNodeLabel,
				})
				Expect(err).ToNot(HaveOccurred(), "Fail to find Frrk8 pod list")

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

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

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

				By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
				frrk8sWebhookDeployment, err := deployment.Pull(
					APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
				Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
					"frr-k8s-webhook-server deployment is not ready")

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(ipv4metalLbIPList[0], "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Create first frrconfiguration that receives a single route")
				createFrrConfiguration(frrConfigFiltered1, ipv4metalLbIPList[0],
					tsparams.LocalBGPASN, []string{externalAdvertisedIPv4Routes[0], externalAdvertisedIPv6Routes[0]},
					false, false)

				By("Verify node state updates on worker node 0")
				Eventually(func() string {
					frrNodeState, err := metallb.ListFrrNodeState(APIClient, client.ListOptions{
						FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": workerNodeList[0].Object.Name})})
					Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

					return frrNodeState[0].Object.Status.RunningConfig

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
					frrNodeState, err := metallb.ListFrrNodeState(APIClient, client.ListOptions{
						FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": workerNodeList[1].Object.Name})})
					Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP routes")

					return frrNodeState[0].Object.Status.RunningConfig
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

			By("Waiting until the new frr-k8s-webhook-server deployment is in Ready state.")
			frrk8sWebhookDeployment, err := deployment.Pull(
				APIClient, frrK8WebHookServer, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Fail to pull frr-k8s-webhook-server")
			Expect(frrk8sWebhookDeployment.IsReady(30*time.Second)).To(BeTrue(),
				"frr-k8s-webhook-server deployment is not ready")

			By("Collecting information before test")
			frrk8sPods, err = pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
				LabelSelector: frrNodeLabel,
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list speaker pods")
			By("Setting test iteration parameters")
			masterClientPodIP, _, _, nodeAddrList, addressPool, _, err =
				metallbenv.DefineIterationParams(
					ipv4metalLbIPList, ipv6metalLbIPList, ipv4NodeAddrList, ipv6NodeAddrList, netparam.IPV4Family)
			Expect(err).ToNot(HaveOccurred(), "Fail to set iteration parameters")

			By("Creating an IPAddressPool and BGPAdvertisement")
			ipAddressPool := setupBgpAdvertisement(addressPool, int32(32))

			By("Creating a MetalLB service")
			setupMetalLbService(netparam.IPV4Family, ipAddressPool, "Cluster")

			By("Creating nginx test pod on worker node")
			setupNGNXPod(workerNodeList[0].Definition.Name)
		})

		AfterAll(func() {
			By("Clean metallb operator and test namespaces")
			resetOperatorAndTestNS()
		})

		AfterEach(func() {
			By("Removing static routes from the speakers")
			frrk8sPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace, metav1.ListOptions{
				LabelSelector: tsparams.FRRK8sDefaultLabel,
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list pods")

			speakerRoutesMap := buildRoutesMapWithSpecificRoutes(frrk8sPods, []string{ipv4metalLbIPList[0],
				ipv4metalLbIPList[1], frrNodeSecIntIPv4Addresses[0], frrNodeSecIntIPv4Addresses[1]})

			for _, frrk8sPod := range frrk8sPods {
				out, err := frr.SetStaticRoute(frrk8sPod, "del", frrExternalMasterIPAddress, speakerRoutesMap)
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
				speakerRoutesMap := buildRoutesMapWithSpecificRoutes(frrk8sPods, ipv4metalLbIPList)

				for _, frrk8sPod := range frrk8sPods {
					out, err := frr.SetStaticRoute(frrk8sPod, "add", masterClientPodIP, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

				By("Creating External NAD for master FRR pod")
				createExternalNad(tsparams.ExternalMacVlanNADName)

				By("Creating External NAD for hub FRR pods")
				createExternalNad(tsparams.HubMacVlanNADName)

				By("Creating static ip annotation for hub0")
				hub0BRstaticIPAnnotation := createStaticIPAnnotations(tsparams.ExternalMacVlanNADName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", ipv4metalLbIPList[0])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[0])})

				By("Creating static ip annotation for hub1")
				hub1BRstaticIPAnnotation := createStaticIPAnnotations(tsparams.ExternalMacVlanNADName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", ipv4metalLbIPList[1])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[1])})

				By("Creating MetalLb Hub pod configMap")
				hubConfigMap := createHubConfigMap("frr-hub-node-config")

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name, hubConfigMap.Definition.Name, []string{}, hub0BRstaticIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name, hubConfigMap.Definition.Name, []string{}, hub1BRstaticIPAnnotation)

				By("Creating configmap and MetalLb Master pod")
				frrPod := createMasterFrrPod(tsparams.LocalBGPASN, frrExternalMasterIPAddress, nodeAddrList,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, false)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(frrExternalMasterIPAddress, "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList),
					addressPool, 32)

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
				speakerRoutesMap := buildRoutesMapWithSpecificRoutes(frrk8sPods, ipv4metalLbIPList)

				for _, frrk8sPod := range frrk8sPods {
					out, err := frr.SetStaticRoute(frrk8sPod, "add", masterClientPodIP, speakerRoutesMap)
					Expect(err).ToNot(HaveOccurred(), out)
				}

				By("Creating External NAD for master FRR pod")
				createExternalNad(tsparams.ExternalMacVlanNADName)

				By("Creating External NAD for hub FRR pods")
				createExternalNad(tsparams.HubMacVlanNADName)

				By("Creating static ip annotation for hub0")
				hub0BRstaticIPAnnotation := createStaticIPAnnotations(tsparams.ExternalMacVlanNADName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", ipv4metalLbIPList[0])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[0])})

				By("Creating static ip annotation for hub1")
				hub1BRstaticIPAnnotation := createStaticIPAnnotations(tsparams.ExternalMacVlanNADName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", ipv4metalLbIPList[1])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[1])})

				By("Creating MetalLb Hub pod configMap")
				hubConfigMap := createHubConfigMap("frr-hub-node-config")

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name, hubConfigMap.Definition.Name, []string{}, hub0BRstaticIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name, hubConfigMap.Definition.Name, []string{}, hub1BRstaticIPAnnotation)

				By("Creating MetalLb Master pod configMap")
				frrPod := createMasterFrrPod(tsparams.RemoteBGPASN, frrExternalMasterIPAddress, nodeAddrList,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, true)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(frrExternalMasterIPAddress, "", tsparams.RemoteBGPASN,
					true, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, removePrefixFromIPList(ipv4NodeAddrList))

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, removePrefixFromIPList(nodeAddrList),
					addressPool, 32)

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
				speakerRoutesMap := buildRoutesMapWithSpecificRoutes(frrk8sPods, hubSecIntIPv4Addresses)

				for _, frrk8sPod := range frrk8sPods {
					// Wait until the interface is created before adding the static route
					Eventually(func() error {
						// Here you can add logic to check if the interface exists
						out, err := frr.SetStaticRoute(frrk8sPod, "add", fmt.Sprintf("%s/32",
							frrExternalMasterIPAddress), speakerRoutesMap)
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
				createExternalNad(tsparams.ExternalMacVlanNADName)

				By("Creating External NAD for hub FRR pods")
				createExternalNad(tsparams.HubMacVlanNADName)

				By("Creating MetalLb Hub pod configMap")
				createHubConfigMapSecInt := createHubConfigMap("frr-hub-node-config")

				By("Creating static ip annotation for hub0")
				hub0BRStaticSecIntIPAnnotation := createStaticIPAnnotations(tsparams.HubMacVlanNADSecIntName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", hubSecIntIPv4Addresses[0])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[0])})

				By("Creating static ip annotation for hub1")
				hub1SecIntIPAnnotation := createStaticIPAnnotations(tsparams.HubMacVlanNADSecIntName,
					tsparams.HubMacVlanNADName,
					[]string{fmt.Sprintf("%s/24", hubSecIntIPv4Addresses[1])},
					[]string{fmt.Sprintf("%s/24", hubIPv4ExternalAddresses[1])})

				By("Creating FRR Hub pod on worker node 0")
				_ = createFrrHubPod(hubPodWorker0,
					workerNodeList[0].Object.Name, createHubConfigMapSecInt.Definition.Name, []string{},
					hub0BRStaticSecIntIPAnnotation)

				By("Creating FRR Hub pod on worker node 1")
				_ = createFrrHubPod(hubPodWorker1,
					workerNodeList[1].Object.Name, createHubConfigMapSecInt.Definition.Name, []string{},
					hub1SecIntIPAnnotation)

				By("Creating MetalLb Master pod configMap")
				frrPod := createMasterFrrPod(tsparams.LocalBGPASN, frrExternalMasterIPAddress, frrNodeSecIntIPv4Addresses,
					hubIPv4ExternalAddresses, externalAdvertisedIPv4Routes,
					externalAdvertisedIPv6Routes, false)

				By("Creating BGP Peers")
				createBGPPeerAndVerifyIfItsReady(frrExternalMasterIPAddress, "", tsparams.LocalBGPASN,
					false, 0, frrk8sPods)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, frrNodeSecIntIPv4Addresses)
				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, frrNodeSecIntIPv4Addresses, addressPool, 32)

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

func createNewMetalLbDaemonSetAndWaitUntilItsRunningWithAlwaysBlock(timeout time.Duration,
	nodeLabel map[string]string, prefixes []string) {
	By("Verifying if metalLb daemonset is running")

	metalLbIo, err := metallb.Pull(APIClient, tsparams.MetalLbIo, NetConfig.MlbOperatorNamespace)

	if err == nil {
		By("MetalLb daemonset is running. Removing daemonset.")

		_, err = metalLbIo.Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete MetalLb daemonset")
	}

	By("Create new metalLb speaker's daemonSet.")

	metalLbIo = metallb.NewBuilder(APIClient, tsparams.MetalLbIo, NetConfig.MlbOperatorNamespace, nodeLabel)
	metalLbIo.WithFRRConfigAlwaysBlock(prefixes)

	_, err = metalLbIo.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create new MetalLb daemonset")

	var metalLbDs *daemonset.Builder

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			metalLbDs, err = daemonset.Pull(APIClient, tsparams.MetalLbDsName, NetConfig.MlbOperatorNamespace)
			if err != nil {
				By(fmt.Sprintf("Error pulling speakers in %s namespace %s, retry",
					tsparams.MetalLbDsName, NetConfig.MlbOperatorNamespace))

				return false, nil
			}

			metalLbDs, err = daemonset.Pull(APIClient, tsparams.FrrDsName, NetConfig.MlbOperatorNamespace)
			if err != nil {
				By(fmt.Sprintf("Error pulling frrk8s in %s namespace %s, retry",
					tsparams.FRRK8sDefaultLabel, NetConfig.MlbOperatorNamespace))

				return false, nil
			}

			return true, nil
		})
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for MetalLb daemonset readiness")

	By("Waiting until the new metalLb daemonset is in Ready state.")
	Expect(metalLbDs.IsReady(timeout)).To(BeTrue(), "MetalLb daemonset is not ready")
}

func deployTestPods(addressPool, hubIPAddresses, externalAdvertisedIPv4Routes,
	externalAdvertisedIPv6Routes []string) *pod.Builder {
	By("Creating an IPAddressPool and BGPAdvertisement")

	ipAddressPool := setupBgpAdvertisement(addressPool, int32(32))

	By("Creating a MetalLB service")
	setupMetalLbService(netparam.IPV4Family, ipAddressPool, "Cluster")

	By("Creating nginx test pod on worker node")
	setupNGNXPod(workerNodeList[0].Definition.Name)

	By("Creating External NAD")
	createExternalNad(tsparams.ExternalMacVlanNADName)

	By("Creating static ip annotation")

	staticIPAnnotation := pod.StaticIPAnnotation(
		externalNad.Definition.Name, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], "24")})

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
		NetConfig.MlbOperatorNamespace).
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
		tsparams.HubMacVlanNADName, []string{fmt.Sprintf("%s/24", frrExternalMasterIPAddress)})

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
		externalAdvertisedIPv6Routes, removePrefixFromIPList(nodeAddrList), enableMultiHop, enableBFD)
	configMapData := frr.DefineBaseConfig(tsparams.DaemonsFile, frrBFDConfig, "")
	masterConfigMap, err := configmap.NewBuilder(APIClient, "frr-master-node-config", tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")

	return masterConfigMap
}

func verifyExternalAdvertisedRoutes(frrPod *pod.Builder, ipv4NodeAddrList, externalExpectedRoutes []string) {
	// Get advertised routes from FRR pod, now returned as a map of node IPs to their advertised routes
	advertisedRoutesMap, err := frr.GetBGPAdvertisedRoutes(frrPod, removePrefixFromIPList(ipv4NodeAddrList))
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

func buildRoutesMapWithSpecificRoutes(podList []*pod.Builder, nextHopList []string) map[string]string {
	Expect(len(podList)).ToNot(BeZero(), "Pod list is empty")
	Expect(len(nextHopList)).ToNot(BeZero(), "Nexthop IP addresses list is empty")
	Expect(len(nextHopList)).To(BeNumerically(">=", len(podList)),
		fmt.Sprintf("Number of speaker IP addresses[%d] is less than the number of pods[%d]",
			len(nextHopList), len(podList)))

	routesMap := make(map[string]string)

	for _, frrPod := range podList {
		if frrPod.Definition.Spec.NodeName == workerNodeList[0].Definition.Name {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[1]
		} else {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[0]
		}
	}

	return routesMap
}

func addSecondaryIPToInterface(policyName, nodeName, interfaceName, ipv4Address, ipv6Address string) {
	By(fmt.Sprintf("Adding secondary IPs using policy %s on node %s", policyName, nodeName))

	ipv4Cmd := fmt.Sprintf("ip addr add %s/24 dev %s 2>/dev/null || true", ipv4Address, interfaceName)
	_, err := cmd.RunCommandOnHostNetworkPod(nodeName, NetConfig.MlbOperatorNamespace, ipv4Cmd)
	Expect(err).ToNot(HaveOccurred(), "Failed to add IPv4 address on interface %s", interfaceName)

	ipv6Cmd := fmt.Sprintf("ip addr add %s/64 dev %s 2>/dev/null || true", ipv6Address, interfaceName)
	_, err = cmd.RunCommandOnHostNetworkPod(nodeName, NetConfig.MlbOperatorNamespace, ipv6Cmd)
	Expect(err).ToNot(HaveOccurred(), "Failed to add IPv6 address on interface %s", interfaceName)
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
			_, _ = cmd.RunCommandOnHostNetworkPod(workerNode.Definition.Name,
				NetConfig.MlbOperatorNamespace, ipv4Cmd)

			ipv6Cmd := fmt.Sprintf("ip addr del %s/64 dev %s 2>/dev/null || true", ipv6Addr, interfaceName)
			_, _ = cmd.RunCommandOnHostNetworkPod(workerNode.Definition.Name,
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

func createStaticIPAnnotations(internalNADName, externalNADName string, internalIPAddresses,
	externalIPAddresses []string) []*types.NetworkSelectionElement {
	ipAnnotation := pod.StaticIPAnnotation(internalNADName, internalIPAddresses)
	ipAnnotation = append(ipAnnotation,
		pod.StaticIPAnnotation(externalNADName, externalIPAddresses)...)

	return ipAnnotation
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
