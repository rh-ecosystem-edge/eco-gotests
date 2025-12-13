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
	// Server ready timeout for IPv6.
	ipv6ServerReadyTimeout = 60 * time.Second
)

var _ = Describe("SR-IOV IPv6", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var (
		workerNodeList                       []*nodes.Builder
		sriovInterfaces                      []string
		sriovInterfacePF1                    string
		sriovInterfacePF2                    string
		ipv6ClientMTU1280, ipv6ServerMTU1280 *pod.Builder
		ipv6ClientMTU9000, ipv6ServerMTU9000 *pod.Builder
		err                                  error
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
		createAllIPv6SriovPolicies(sriovInterfacePF1, sriovInterfacePF2)

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
				tsparams.SriovNetworkSamePFMTU1280, tsparams.SriovResourcePF1MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 1280")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkSamePFMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 9000")

			By("Creating client and server pods for MTU 1280 (IPv6-only)")
			ipv6ClientMTU1280, err = sriovenv.CreateTestClientPod(
				tsparams.ClientPodMTU1280, testNode, tsparams.SriovNetworkSamePFMTU1280,
				[]string{tsparams.ClientIPv6IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 1280")

			ipv6ServerMTU1280, err = sriovenv.CreateTestServerPod(
				tsparams.ServerPodMTU1280, testNode, tsparams.SriovNetworkSamePFMTU1280,
				[]string{tsparams.ServerIPv6IPAddress}, tsparams.ServerIPv6Bare,
				tsparams.ServerMacAddress, tsparams.MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 1280")

			By("Creating client and server pods for MTU 9000 (IPv6-only)")
			ipv6ClientMTU9000, err = sriovenv.CreateTestClientPod(
				tsparams.ClientPodMTU9000, testNode, tsparams.SriovNetworkSamePFMTU9000,
				[]string{tsparams.ClientIPv6IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			ipv6ServerMTU9000, err = sriovenv.CreateTestServerPod(
				tsparams.ServerPodMTU9000, testNode, tsparams.SriovNetworkSamePFMTU9000,
				[]string{tsparams.ServerIPv6IPAddress2}, tsparams.ServerIPv6Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready and listening")
			err = sriovenv.WaitForServerReady(ipv6ServerMTU1280, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 1280 not ready")

			err = sriovenv.WaitForServerReady(ipv6ServerMTU9000, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(
				ipv6ClientMTU1280, ipv6ServerMTU1280, ipv6ClientMTU9000, ipv6ServerMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Same Node Same PF")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkSamePFMTU1280, tsparams.SriovNetworkSamePFMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		FIt("Verify SR-IOV IPv6 connectivity with Static IPAM and Static MAC",
			reportxml.ID("31804"), Label("ipv6", "static-ipam", "static-mac"), func() {
				By("Running traffic tests with MTU 1280")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU1280, tsparams.ServerIPv6IPAddress, tsparams.MTU1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 1280")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU9000, tsparams.ServerIPv6IPAddress2, tsparams.MTU9000)
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
				tsparams.SriovNetworkDiffPFClientMTU1280, tsparams.SriovResourcePF1MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client network for MTU 1280")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFClientMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client network for MTU 9000")

			// Server networks use PF2 resources.
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFServerMTU1280, tsparams.SriovResourcePF2MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server network for MTU 1280")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffPFServerMTU9000, tsparams.SriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server network for MTU 9000")

			By("Creating client and server pods for MTU 1280 (IPv6-only)")
			ipv6ClientMTU1280, err = sriovenv.CreateTestClientPod(
				"client-ipv6-diffpf-mtu1280", testNode, tsparams.SriovNetworkDiffPFClientMTU1280,
				[]string{tsparams.ClientIPv6IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 1280")

			ipv6ServerMTU1280, err = sriovenv.CreateTestServerPod(
				"server-ipv6-diffpf-mtu1280", testNode, tsparams.SriovNetworkDiffPFServerMTU1280,
				[]string{tsparams.ServerIPv6IPAddress}, tsparams.ServerIPv6Bare,
				tsparams.ServerMacAddress, tsparams.MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 1280")

			By("Creating client and server pods for MTU 9000 (IPv6-only)")
			ipv6ClientMTU9000, err = sriovenv.CreateTestClientPod(
				"client-ipv6-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFClientMTU9000,
				[]string{tsparams.ClientIPv6IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			ipv6ServerMTU9000, err = sriovenv.CreateTestServerPod(
				"server-ipv6-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFServerMTU9000,
				[]string{tsparams.ServerIPv6IPAddress2}, tsparams.ServerIPv6Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready")
			err = sriovenv.WaitForServerReady(ipv6ServerMTU1280, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 1280 not ready")

			err = sriovenv.WaitForServerReady(ipv6ServerMTU9000, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(
				ipv6ClientMTU1280, ipv6ServerMTU1280, ipv6ClientMTU9000, ipv6ServerMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Same Node Different PF")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkDiffPFClientMTU1280, tsparams.SriovNetworkDiffPFServerMTU1280,
				tsparams.SriovNetworkDiffPFClientMTU9000, tsparams.SriovNetworkDiffPFServerMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv6 connectivity between different PFs on same node",
			reportxml.ID("31805"), Label("ipv6", "static-ipam", "static-mac", "diff-pf"), func() {
				By("Running traffic tests with MTU 1280")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU1280, tsparams.ServerIPv6IPAddress, tsparams.MTU1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 1280")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU9000, tsparams.ServerIPv6IPAddress2, tsparams.MTU9000)
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
				tsparams.SriovNetworkDiffNodeMTU1280, tsparams.SriovResourcePF1MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 1280")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkDiffNodeMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 9000")

			By("Creating client and server pods for MTU 1280 (IPv6-only)")
			ipv6ClientMTU1280, err = sriovenv.CreateTestClientPod(
				"client-ipv6-diffnode-mtu1280", clientNode, tsparams.SriovNetworkDiffNodeMTU1280,
				[]string{tsparams.ClientIPv6IPAddress}, tsparams.ClientMacAddress)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 1280")

			ipv6ServerMTU1280, err = sriovenv.CreateTestServerPod(
				"server-ipv6-diffnode-mtu1280", serverNode, tsparams.SriovNetworkDiffNodeMTU1280,
				[]string{tsparams.ServerIPv6IPAddress}, tsparams.ServerIPv6Bare,
				tsparams.ServerMacAddress, tsparams.MTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 1280")

			By("Creating client and server pods for MTU 9000 (IPv6-only)")
			ipv6ClientMTU9000, err = sriovenv.CreateTestClientPod(
				"client-ipv6-diffnode-mtu9000", clientNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ClientIPv6IPAddress2}, tsparams.ClientMacAddress2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			ipv6ServerMTU9000, err = sriovenv.CreateTestServerPod(
				"server-ipv6-diffnode-mtu9000", serverNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ServerIPv6IPAddress2}, tsparams.ServerIPv6Bare2,
				tsparams.ServerMacAddress2, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

			By("Waiting for server pods to be ready")
			err = sriovenv.WaitForServerReady(ipv6ServerMTU1280, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 1280 not ready")

			err = sriovenv.WaitForServerReady(ipv6ServerMTU9000, ipv6ServerReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Server pod MTU 9000 not ready")
		})

		AfterAll(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(
				ipv6ClientMTU1280, ipv6ServerMTU1280, ipv6ClientMTU9000, ipv6ServerMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			By("Deleting SR-IOV networks for Different Node")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkDiffNodeMTU1280, tsparams.SriovNetworkDiffNodeMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv6 connectivity between different nodes",
			reportxml.ID("31806"), Label("ipv6", "static-ipam", "static-mac", "diff-node"), func() {
				By("Running traffic tests with MTU 1280")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU1280, tsparams.ServerIPv6IPAddress, tsparams.MTU1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 1280")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(
					ipv6ClientMTU9000, tsparams.ServerIPv6IPAddress2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})
	})
})

// createAllIPv6SriovPolicies creates all SR-IOV policies for both PFs upfront.
// This causes a single node reboot instead of multiple reboots per context.
// Note: IPv6 requires minimum MTU of 1280, so we use MTU 1280 instead of MTU 500.
func createAllIPv6SriovPolicies(pf1, pf2 string) {
	By("Creating SR-IOV policy for PF1 MTU 1280 (VFs 0-2)")

	err := sriovenv.CreateSriovPolicyWithMTU(
		"policy-ipv6-pf1-mtu1280", tsparams.SriovResourcePF1MTU1280, pf1,
		tsparams.MTU1280, tsparams.TotalVFs, tsparams.VFStartMTU1280, tsparams.VFEndMTU1280)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF1 MTU 1280")

	By("Creating SR-IOV policy for PF1 MTU 9000 (VFs 3-5)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-ipv6-pf1-mtu9000", tsparams.SriovResourcePF1MTU9000, pf1,
		tsparams.MTU9000, tsparams.TotalVFs, tsparams.VFStartMTU9000, tsparams.VFEndMTU9000)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF1 MTU 9000")

	By("Creating SR-IOV policy for PF2 MTU 1280 (VFs 0-2)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-ipv6-pf2-mtu1280", tsparams.SriovResourcePF2MTU1280, pf2,
		tsparams.MTU1280, tsparams.TotalVFs, tsparams.VFStartMTU1280, tsparams.VFEndMTU1280)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF2 MTU 1280")

	By("Creating SR-IOV policy for PF2 MTU 9000 (VFs 3-5)")

	err = sriovenv.CreateSriovPolicyWithMTU(
		"policy-ipv6-pf2-mtu9000", tsparams.SriovResourcePF2MTU9000, pf2,
		tsparams.MTU9000, tsparams.TotalVFs, tsparams.VFStartMTU9000, tsparams.VFEndMTU9000)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for PF2 MTU 9000")
}
