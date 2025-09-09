package tests

import (
	"fmt"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FRR", Ordered, Label(tsparams.LabelBGPTestCases), ContinueOnFailure, func() {
	var frrk8sPods []*pod.Builder

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()
	})

	BeforeEach(func() {
		By("Creating a new instance of MetalLB Speakers on workers")
		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

		By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
			"worker nodes.")
		frrk8sPods = verifyAndCreateFRRk8sPodList()
	})

	AfterEach(func() {
		By("Cleaning MetalLb operator namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetBGPAdvertisementGVR(),
			metallb.GetBGPPeerGVR(),
			metallb.GetBFDProfileGVR(),
			metallb.GetL2AdvertisementGVR(),
			metallb.GetIPAddressPoolGVR(),
			metallb.GetMetalLbIoGVR())
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

	AfterAll(func() {
		By("Removing test label from worker nodes")
		if len(cnfWorkerNodeList) > 2 {
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}
	})

	It("Verify configuration of a FRR node router peer with the connectTime less than the default of 120 seconds",
		reportxml.ID("74414"), func() {

			By("Creating BGP Peers with 10 second retry connect timer")
			createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
				tsparams.LocalBGPASN, false, 10, frrk8sPods)

			By("Validate BGP Peers with 10 second retry connect timer")
			Eventually(func() int {
				// Get the connect time configuration
				connectTimeValue, err := frr.FetchBGPConnectTimeValue(frrk8sPods, ipv4metalLbIPList[0])
				Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP connect time")

				// Return the integer value of ConnectRetryTimer for assertion
				return connectTimeValue
			}, 60*time.Second, 5*time.Second).Should(Equal(10),
				"Failed to fetch BGP connect time")
		})

	It("Verify the retry timers reconnects to a neighbor with a timer connect less then 10s after a BGP tcp reset",
		reportxml.ID("74416"), func() {

			By("Create an external FRR Pod")
			frrPod := createAndDeployFRRPod()

			By("Creating BGP Peers with 10 second retry connect timer")
			createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
				tsparams.LocalBGPASN, false, 10, frrk8sPods)

			By("Validate BGP Peers with 10 second retry connect timer")
			Eventually(func() int {
				// Get the connect time configuration
				connectTimeValue, err := frr.FetchBGPConnectTimeValue(frrk8sPods, ipv4metalLbIPList[0])
				Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP connect time")

				// Return the integer value of ConnectRetryTimer for assertion
				return connectTimeValue
			}, 60*time.Second, 5*time.Second).Should(Equal(10),
				"Failed to fetch BGP connect time")

			By("Reset the BGP session ")
			err := frr.ResetBGPConnection(frrPod)
			Expect(err).ToNot(HaveOccurred(), "Failed to reset BGP connection")

			By("Verify that BGP session is re-established and up in less then 10 seconds")
			verifyMaxReConnectTime(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList), time.Second*10)
		})

	It("Update the timer to less then the default on an existing BGP connection",
		reportxml.ID("74417"), func() {

			By("Creating BGP Peers")
			createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
				tsparams.LocalBGPASN, false, 0, frrk8sPods)

			By("Validate BGP Peers with the default retry connect timer")
			Eventually(func() int {
				// Get the connect time configuration
				connectTimeValue, err := frr.FetchBGPConnectTimeValue(frrk8sPods, ipv4metalLbIPList[0])
				Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP connect time")

				// Return the integer value of ConnectRetryTimer for assertion
				return connectTimeValue
			}, 60*time.Second, 5*time.Second).Should(Equal(120),
				"Failed to fetch BGP connect time")

			By("Update the BGP Peers connect timer to 10 seconds")
			bgpPeer, err := metallb.PullBGPPeer(APIClient, tsparams.BgpPeerName1, NetConfig.MlbOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to find bgp peer")

			_, err = bgpPeer.WithConnectTime(metav1.Duration{Duration: 10 * time.Second}).Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to update the bgp peer with a 10s connect timer")

			By("Validate BGP Peers with the default retry connect timer")
			Eventually(func() int {
				// Get the connect time configuration
				connectTimeValue, err := frr.FetchBGPConnectTimeValue(frrk8sPods, ipv4metalLbIPList[0])
				Expect(err).ToNot(HaveOccurred(), "Failed to verify BGP connect time")

				// Return the integer value of ConnectRetryTimer for assertion
				return connectTimeValue
			}, 60*time.Second, 5*time.Second).Should(Equal(10),
				"Failed to fetch BGP connect time")
		})
})

func createAndDeployFRRPod() *pod.Builder {
	By("Creating External NAD")

	err := define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

	By("Creating static ip annotation")

	staticIPAnnotation := pod.StaticIPAnnotation(
		frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0],
			netparam.IPSubnet24)})

	By("Creating MetalLb configMap")

	masterConfigMap := createConfigMap(tsparams.LocalBGPASN, ipv4NodeAddrList, false, false)

	By("Creating FRR Pod")

	frrPod := createFrrPod(
		masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation)

	return frrPod
}

func verifyMaxReConnectTime(frrPod *pod.Builder, peerAddrList []string, maxConnectTime time.Duration) {
	for _, peerAddress := range netcmd.RemovePrefixFromIPList(peerAddrList) {
		Eventually(frr.BGPNeighborshipHasState,
			maxConnectTime, time.Second).
			WithArguments(frrPod, peerAddress, "Established").Should(
			BeTrue(), "Failed to receive BGP status UP")
	}
}
