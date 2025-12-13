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
		var testPods []*pod.Builder

		BeforeAll(func() {
			testNode = workerNodeList[0].Definition.Name

			By("Creating SR-IOV Networks")
			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkSamePFMTU500, tsparams.SriovResourcePF1MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 500")

			err = sriovenv.CreateSriovNetworkWithStaticIPAM(
				tsparams.SriovNetworkSamePFMTU9000, tsparams.SriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for MTU 9000")

			// VLAN network for Same PF VLAN test (Whereabouts IPAM + Dynamic MAC).
			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				tsparams.SriovNetworkVlanSamePFMTU500, tsparams.SriovResourcePF1MTU500, vlanID,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway)
			Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN network for Same PF")
		})

		AfterEach(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(testPods...)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")
			testPods = nil
		})

		AfterAll(func() {
			By("Deleting SR-IOV networks")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkSamePFMTU500, tsparams.SriovNetworkSamePFMTU9000,
				tsparams.SriovNetworkVlanSamePFMTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity with Static IPAM and Static MAC",
			reportxml.ID("31801"), Label("ipv4", "static-ipam", "static-mac"), func() {
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

				testPods = []*pod.Builder{clientMTU500, serverMTU500, clientMTU9000, serverMTU9000}

				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4Bare, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4Bare2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})

		FIt("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM, Dynamic MAC, and VLAN",
			reportxml.ID("31808"), Label("ipv4", "whereabouts-ipam", "dynamic-mac", "vlan", "same-pf"), func() {
				By("Creating client pod with VLAN tagging and dynamic IP/MAC")
				vlanClient, err := sriovenv.CreateTestClientPod(
					tsparams.ClientPodVlan, testNode, tsparams.SriovNetworkVlanSamePFMTU500,
					nil, "")
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN client pod")

				By("Creating server pod with VLAN tagging and dynamic IP/MAC")
				vlanServer, err := sriovenv.CreateTestServerPod(
					tsparams.ServerPodVlan, testNode, tsparams.SriovNetworkVlanSamePFMTU500,
					nil, "", "", tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN server pod")

				testPods = []*pod.Builder{vlanClient, vlanServer}

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(vlanServer, "net1")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod")

				By("Running traffic tests over VLAN with dynamic IP")
				err = sriovenv.RunTrafficTest(vlanClient, serverIP, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN")
			})
	})

	// Context for Same Node, Different PF connectivity tests.
	Context("Same Node Different PF", Label("samenode-diffpf"), func() {
		var testNode string
		var testPods []*pod.Builder

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

			// Whereabouts networks for dynamic IP/MAC test.
			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				tsparams.SriovNetworkWhereaboutsDiffPFClientMTU500, tsparams.SriovResourcePF1MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway)
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts client network")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				tsparams.SriovNetworkWhereaboutsDiffPFServerMTU500, tsparams.SriovResourcePF2MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway)
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts server network")
		})

		AfterEach(func() {
			By("Deleting test pods")
			err = sriovenv.DeleteTestPods(testPods...)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")
			testPods = nil
		})

		AfterAll(func() {
			By("Deleting SR-IOV networks")
			err = sriovenv.DeleteSriovNetworks(
				tsparams.SriovNetworkDiffPFClientMTU500, tsparams.SriovNetworkDiffPFServerMTU500,
				tsparams.SriovNetworkDiffPFClientMTU9000, tsparams.SriovNetworkDiffPFServerMTU9000,
				tsparams.SriovNetworkWhereaboutsDiffPFClientMTU500, tsparams.SriovNetworkWhereaboutsDiffPFServerMTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity between different PFs on same node",
			reportxml.ID("31802"), Label("ipv4", "static-ipam", "static-mac", "diff-pf"), func() {
				By("Creating client and server pods for MTU 500")
				clientMTU500, err = sriovenv.CreateTestClientPod(
					"client-diffpf-mtu500", testNode, tsparams.SriovNetworkDiffPFClientMTU500,
					[]string{tsparams.ClientIPv4IPAddressCtx2}, tsparams.ClientMacAddressCtx2)
				Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 500")

				serverMTU500, err = sriovenv.CreateTestServerPod(
					"server-diffpf-mtu500", testNode, tsparams.SriovNetworkDiffPFServerMTU500,
					[]string{tsparams.ServerIPv4IPAddressCtx2}, tsparams.ServerIPv4BareCtx2,
					tsparams.ServerMacAddressCtx2, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 500")

				By("Creating client and server pods for MTU 9000")
				clientMTU9000, err = sriovenv.CreateTestClientPod(
					"client-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFClientMTU9000,
					[]string{tsparams.ClientIPv4IPAddress2Ctx2}, tsparams.ClientMacAddress2Ctx2)
				Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

				serverMTU9000, err = sriovenv.CreateTestServerPod(
					"server-diffpf-mtu9000", testNode, tsparams.SriovNetworkDiffPFServerMTU9000,
					[]string{tsparams.ServerIPv4IPAddress2Ctx2}, tsparams.ServerIPv4Bare2Ctx2,
					tsparams.ServerMacAddress2Ctx2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")

				testPods = []*pod.Builder{clientMTU500, serverMTU500, clientMTU9000, serverMTU9000}

				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4BareCtx2, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4Bare2Ctx2, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})

		It("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("31807"), Label("ipv4", "whereabouts-ipam", "dynamic-mac", "diff-pf"), func() {
				By("Creating client pod with whereabouts IPAM (dynamic IP)")
				whereaboutsClient, err := sriovenv.CreateTestClientPod(
					tsparams.ClientPodWhereabouts, testNode, tsparams.SriovNetworkWhereaboutsDiffPFClientMTU500,
					nil, "")
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts client pod")

				By("Creating server pod with whereabouts IPAM (dynamic IP)")
				whereaboutsServer, err := sriovenv.CreateTestServerPod(
					tsparams.ServerPodWhereabouts, testNode, tsparams.SriovNetworkWhereaboutsDiffPFServerMTU500,
					nil, "", "", tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts server pod")

				testPods = []*pod.Builder{whereaboutsClient, whereaboutsServer}

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, "net1")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")
				err = sriovenv.RunTrafficTest(whereaboutsClient, serverIP, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
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
				[]string{tsparams.ClientIPv4IPAddressCtx3}, tsparams.ClientMacAddressCtx3)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 500")

			serverMTU500, err = sriovenv.CreateTestServerPod(
				"server-diffnode-mtu500", serverNode, tsparams.SriovNetworkDiffNodeMTU500,
				[]string{tsparams.ServerIPv4IPAddressCtx3}, tsparams.ServerIPv4BareCtx3,
				tsparams.ServerMacAddressCtx3, tsparams.MTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 500")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, err = sriovenv.CreateTestClientPod(
				"client-diffnode-mtu9000", clientNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ClientIPv4IPAddress2Ctx3}, tsparams.ClientMacAddress2Ctx3)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client pod for MTU 9000")

			serverMTU9000, err = sriovenv.CreateTestServerPod(
				"server-diffnode-mtu9000", serverNode, tsparams.SriovNetworkDiffNodeMTU9000,
				[]string{tsparams.ServerIPv4IPAddress2Ctx3}, tsparams.ServerIPv4Bare2Ctx3,
				tsparams.ServerMacAddress2Ctx3, tsparams.MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server pod for MTU 9000")
		})

		AfterAll(func() {
			By("Cleaning up Different Node test resources")
			err = sriovenv.CleanupTestResources(
				[]string{tsparams.SriovNetworkDiffNodeMTU500, tsparams.SriovNetworkDiffNodeMTU9000},
				clientMTU500, serverMTU500, clientMTU9000, serverMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to cleanup test resources")
		})

		It("Verify SR-IOV IPv4 connectivity between different nodes",
			reportxml.ID("31803"), Label("ipv4", "static-ipam", "static-mac", "diff-node"), func() {
				By("Running traffic tests with MTU 500")
				err = sriovenv.RunTrafficTest(clientMTU500, tsparams.ServerIPv4BareCtx3, tsparams.MTU500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 500")

				By("Running traffic tests with MTU 9000")
				err = sriovenv.RunTrafficTest(clientMTU9000, tsparams.ServerIPv4Bare2Ctx3, tsparams.MTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for MTU 9000")
			})
	})
})

// createAllSriovPolicies creates all SR-IOV policies for both PFs upfront.
// This causes a single node reboot instead of multiple reboots per context.
func createAllSriovPolicies(pf1, pf2 string) {
	By("Creating all SR-IOV policies for IPv4 tests")

	policies := []sriovenv.PolicyConfig{
		{
			Name:         "policy-pf1-mtu500",
			ResourceName: tsparams.SriovResourcePF1MTU500,
			PFName:       pf1,
			MTU:          tsparams.MTU500,
			NumVFs:       tsparams.TotalVFs,
			VFStart:      tsparams.VFStartMTU500,
			VFEnd:        tsparams.VFEndMTU500,
		},
		{
			Name:         "policy-pf1-mtu9000",
			ResourceName: tsparams.SriovResourcePF1MTU9000,
			PFName:       pf1,
			MTU:          tsparams.MTU9000,
			NumVFs:       tsparams.TotalVFs,
			VFStart:      tsparams.VFStartMTU9000,
			VFEnd:        tsparams.VFEndMTU9000,
		},
		{
			Name:         "policy-pf2-mtu500",
			ResourceName: tsparams.SriovResourcePF2MTU500,
			PFName:       pf2,
			MTU:          tsparams.MTU500,
			NumVFs:       tsparams.TotalVFs,
			VFStart:      tsparams.VFStartMTU500,
			VFEnd:        tsparams.VFEndMTU500,
		},
		{
			Name:         "policy-pf2-mtu9000",
			ResourceName: tsparams.SriovResourcePF2MTU9000,
			PFName:       pf2,
			MTU:          tsparams.MTU9000,
			NumVFs:       tsparams.TotalVFs,
			VFStart:      tsparams.VFStartMTU9000,
			VFEnd:        tsparams.VFEndMTU9000,
		},
	}

	err := sriovenv.CreateSriovPolicies(policies)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies")
}
