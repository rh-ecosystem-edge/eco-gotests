package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// Server ready timeout.
	serverReadyTimeout = 30 * time.Second
)

var _ = Describe("SR-IOV IPv4", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var (
		workerNodeList               []*nodes.Builder
		sriovInterfaces              []string
		sriovInterfacePF1            string
		sriovInterfacePF2            string
		clientMTU500, serverMTU500   *pod.Builder
		clientMTU9000, serverMTU9000 *pod.Builder
		err                          error
	)

	BeforeAll(func() {
		By("Listing worker nodes")
		workerNodeList, err = nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
		Expect(len(workerNodeList)).To(BeNumerically(">=", 2),
			"Cluster needs at least 2 worker nodes for SR-IOV tests")

		By("Getting SR-IOV interfaces for testing")
		sriovInterfaces, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to get SR-IOV interfaces")
		Expect(len(sriovInterfaces)).To(BeNumerically(">=", 2),
			"Need at least 2 SR-IOV interfaces for full test coverage")
		sriovInterfacePF1 = sriovInterfaces[0]
		sriovInterfacePF2 = sriovInterfaces[1]

		By("Validating SR-IOV interfaces exist on nodes")
		err = sriovenv.ValidateSriovInterfaces(workerNodeList, 2)
		Expect(err).ToNot(HaveOccurred(), "SR-IOV interfaces validation failed")

		By("Enabling SCTP kernel module on worker nodes")
		err = sriovenv.EnableSCTPOnWorkers(workerNodeList)
		Expect(err).ToNot(HaveOccurred(), "Failed to enable SCTP on workers")

		By("Creating all SR-IOV policies upfront (one-time node reboot)")
		createAllSriovPolicies(sriovInterfacePF1, sriovInterfacePF2)

		By("Waiting for SR-IOV policies to be applied on PF1")
		err = sriovenv.WaitUntilVfsCreated(
			workerNodeList, sriovInterfacePF1, tsparams.TotalVFs, 10*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed waiting for VFs on PF1")

		By("Waiting for SR-IOV policies to be applied on PF2")
		err = sriovenv.WaitUntilVfsCreated(
			workerNodeList, sriovInterfacePF2, tsparams.TotalVFs, 5*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed waiting for VFs on PF2")
	})

	AfterAll(func() {
		By("Cleaning up all SR-IOV policies")
		err = sriovenv.CleanupAllSriovResources(NetConfig.CnfMcpLabel, tsparams.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to cleanup SR-IOV resources")
	})

	// Context for Same Node, Same PF connectivity tests.
	Context("Same Node Same PF", Label("samenode-samepf"), func() {
		var testNode string

		BeforeAll(func() {
			testNode = workerNodeList[0].Definition.Name

			By("Creating SR-IOV Networks for Same Node Same PF")
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkSamePFMTU500, tsparams.SriovResourcePF1MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 500")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkSamePFMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 9000")

			By("Creating client and server pods for MTU 500")
			clientMTU500, err = sriovenv.CreateTestClientPod(
				tsparams.ClientPodMTU500, testNode, tsparams.SriovNetworkSamePFMTU500,
				[]string{tsparams.ClientIPv4IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 500")

			serverMTU500, err = sriovenv.CreateTestServerPod(
				tsparams.ServerPodMTU500, testNode, tsparams.SriovNetworkSamePFMTU500,
				[]string{tsparams.ServerIPv4IPAddress}, tsparams.ServerIPv4Bare,
				tsparams.ServerMacAddress, tsparams.MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 500")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, err = sriovenv.CreateTestClientPod(
				tsparams.ClientPodMTU9000, testNode, tsparams.SriovNetworkSamePFMTU9000,
				[]string{tsparams.ClientIPv4IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			serverMTU9000, err = sriovenv.CreateTestServerPod(
				tsparams.ServerPodMTU9000, testNode, tsparams.SriovNetworkSamePFMTU9000,
				[]string{tsparams.ServerIPv4IPAddress2}, tsparams.ServerIPv4Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready and listening")
			err = sriovenv.WaitForServerReady(serverMTU500, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 500 not ready")

			err = sriovenv.WaitForServerReady(serverMTU9000, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(clientMTU500, serverMTU500, clientMTU9000, serverMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Same Node Same PF")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkSamePFMTU500, tsparams.SriovNetworkSamePFMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity with Static IPAM and Static MAC",
			reportxml.ID("31801"), Label("ipv4", "static-ipam", "static-mac"), func() {
				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4IPAddress, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4IPAddress2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})
	})

	// Context for Same Node, Different PF connectivity tests.
	Context("Same Node Different PF", Label("samenode-diffpf"), func() {
		var testNode string

		BeforeAll(func() {
			testNode = workerNodeList[0].Definition.Name

			By("Creating SR-IOV Networks for Same Node Different PF")
			// Client networks use PF1 resources.
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFClientMTU500, tsparams.SriovResourcePF1MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client network for MTU 500")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFClientMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client network for MTU 9000")

			// Server networks use PF2 resources.
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFServerMTU500, tsparams.SriovResourcePF2MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server network for MTU 500")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFServerMTU9000, tsparams.SriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server network for MTU 9000")

			By("Creating client and server pods for MTU 500")
			clientMTU500, err = sriovenv.CreateTestClientPod(
				"client-diffpf-mtu500", testNode, tsparams.SriovNetworkDiffPFClientMTU500,
				[]string{tsparams.ClientIPv4IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 500")

			serverMTU500, err = sriovenv.CreateTestServerPod(
				"server-diffpf-mtu500", testNode, tsparams.SriovNetworkDiffPFServerMTU500,
				[]string{tsparams.ServerIPv4IPAddress}, tsparams.ServerIPv4Bare,
				tsparams.ServerMacAddress, tsparams.MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 500")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, err = sriovenv.CreateTestClientPod(
				"client-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFClientMTU9000,
				[]string{tsparams.ClientIPv4IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			serverMTU9000, err = sriovenv.CreateTestServerPod(
				"server-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFServerMTU9000,
				[]string{tsparams.ServerIPv4IPAddress2}, tsparams.ServerIPv4Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready")
			err = sriovenv.WaitForServerReady(serverMTU500, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 500 not ready")

			err = sriovenv.WaitForServerReady(serverMTU9000, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(clientMTU500, serverMTU500, clientMTU9000, serverMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Same Node Different PF")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkDiffPFClientMTU500, tsparams.SriovNetworkDiffPFServerMTU500,
				tsparams.SriovNetworkDiffPFClientMTU9000, tsparams.SriovNetworkDiffPFServerMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity between different PFs on same node",
			reportxml.ID("31802"), Label("ipv4", "static-ipam", "static-mac", "diff-pf"), func() {
				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4IPAddress, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4IPAddress2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})
	})

	// Context for Different Node connectivity tests.
	Context("Different Node", Label("diffnode"), func() {
		var (
			clientNode string
			serverNode string
		)

		BeforeAll(func() {
			clientNode = workerNodeList[0].Definition.Name
			serverNode = workerNodeList[1].Definition.Name

			By(fmt.Sprintf("Using client on node %s and server on node %s", clientNode, serverNode))

			By("Creating SR-IOV Networks for Different Node")
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffNodeMTU500, tsparams.SriovResourcePF1MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 500")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffNodeMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 9000")

			By("Creating client and server pods for MTU 500")
			clientMTU500, err = sriovenv.CreateTestClientPod(
				"client-diffnode-mtu500", clientNode, tsparams.SriovNetworkDiffNodeMTU500,
				[]string{tsparams.ClientIPv4IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 500")

			serverMTU500, err = sriovenv.CreateTestServerPod(
				"server-diffnode-mtu500", serverNode, tsparams.SriovNetworkDiffNodeMTU500,
				[]string{tsparams.ServerIPv4IPAddress}, tsparams.ServerIPv4Bare,
				tsparams.ServerMacAddress, tsparams.MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 500")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, err = sriovenv.CreateTestClientPod(
				"client-diffnode-mtu9000", clientNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ClientIPv4IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			serverMTU9000, err = sriovenv.CreateTestServerPod(
				"server-diffnode-mtu9000", serverNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ServerIPv4IPAddress2}, tsparams.ServerIPv4Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready")
			err = sriovenv.WaitForServerReady(serverMTU500, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 500 not ready")

			err = sriovenv.WaitForServerReady(serverMTU9000, serverReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(clientMTU500, serverMTU500, clientMTU9000, serverMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Different Node")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkDiffNodeMTU500, tsparams.SriovNetworkDiffNodeMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity between different nodes",
			reportxml.ID("31803"), Label("ipv4", "static-ipam", "static-mac", "diff-node"), func() {
				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4IPAddress, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4IPAddress2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})
	})
})

// createAllSriovPolicies creates all SR-IOV policies for both PFs upfront.
// This causes a single node reboot instead of multiple reboots per context.
func createAllSriovPolicies(pf1, pf2 string) {
	By("Creating SR-IOV policy for PF1 MTU 500 (VFs 0-2)")

	err := sriovenv.CreateSriovPolicyWithMTU(
		"policy-pf1-mtu500", tsparams.SriovResourcePF1MTU500, pf1,
		tsparams.MTU500, tsparams.TotalVFs, tsparams.VFStartMTU500, tsparams.VFEndMTU500)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF1 MTU 500")

	By("Creating SR-IOV policy for PF1 MTU 9000 (VFs 3-5)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-pf1-mtu9000", tsparams.SriovResourcePF1MTU9000, pf1,
		tsparams.MTU9000, tsparams.TotalVFs, tsparams.VFStartMTU9000, tsparams.VFEndMTU9000)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF1 MTU 9000")

	By("Creating SR-IOV policy for PF2 MTU 500 (VFs 0-2)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-pf2-mtu500", tsparams.SriovResourcePF2MTU500, pf2,
		tsparams.MTU500, tsparams.TotalVFs, tsparams.VFStartMTU500, tsparams.VFEndMTU500)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF2 MTU 500")

	By("Creating SR-IOV policy for PF2 MTU 9000 (VFs 3-5)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-pf2-mtu9000", tsparams.SriovResourcePF2MTU9000, pf2,
		tsparams.MTU9000, tsparams.TotalVFs, tsparams.VFStartMTU9000, tsparams.VFEndMTU9000)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF2 MTU 9000")
}
