package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("SR-IOV IPv4", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	const (
		// SR-IOV resource names for policies.
		sriovResourcePF1MTU500  = "sriovpf1mtu500"
		sriovResourcePF1MTU9000 = "sriovpf1mtu9000"
		sriovResourcePF2MTU500  = "sriovpf2mtu500"
		sriovResourcePF2MTU9000 = "sriovpf2mtu9000"
		// MTU values for testing.
		mtu500  = 500
		mtu9000 = 9000
	)

	var (
		workerNodeList    []*nodes.Builder
		sriovInterfaces   []string
		sriovInterfacePF1 string
		sriovInterfacePF2 string
		clientMTU500      *pod.Builder
		clientMTU9000     *pod.Builder
		err               error
	)

	BeforeAll(func() {
		By("Checking cluster IP family")

		clusterIPFamily, err := netenv.GetClusterIPFamily(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to detect cluster IP family")

		if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
			Skip("Cluster does not support IPv4 - skipping IPv4 SR-IOV tests")
		}

		By("Discover and list worker nodes")

		workerNodeList, err = nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")

		if len(workerNodeList) < 2 {
			Skip("Cluster needs at least 2 worker nodes for SR-IOV tests")
		}

		By("Validating SR-IOV interfaces exist on nodes")
		Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		sriovInterfaces, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		sriovInterfacePF1 = sriovInterfaces[0]
		sriovInterfacePF2 = sriovInterfaces[1]

		sriovenv.ActivateSCTPModuleOnWorkerNodes()

		By("Verifying SCTP kernel module is loaded on worker nodes")

		sctpOutput, err := cluster.ExecCmdWithStdout(APIClient, "lsmod | grep -q sctp && echo loaded",
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		if err != nil || len(sctpOutput) == 0 {
			Skip("SCTP kernel module is not loaded on worker nodes - cluster must be pre-configured with SCTP")
		}

		By("Create all test case SR-IOV policies and waiting for them to be applied")

		err = sriovenv.CreateAllSriovPolicies(
			sriovInterfacePF1, sriovInterfacePF2,
			sriovResourcePF1MTU500, sriovResourcePF1MTU9000,
			sriovResourcePF2MTU500, sriovResourcePF2MTU9000,
			"ipv4", mtu500, mtu9000)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies")
	})

	AfterAll(func() {
		By("Removing SR-IOV configuration")

		err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			tsparams.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
	})

	// Context for Same Node, Same PF connectivity tests.
	Context("Same Node Same PF", func() {
		const (
			sriovNetworkSamePFMTU500      = "sriov-net-samepf-mtu500"
			sriovNetworkSamePFMTU9000     = "sriov-net-samepf-mtu9000"
			sriovNetworkVlanSamePFMTU500  = "sriov-net-vlan-samepf-mtu500"
			sriovNetworkVlanSamePFMTU9000 = "sriov-net-vlan-samepf-mtu9000"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks")

			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkSamePFMTU500, sriovNetworkSamePFMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			// VLAN network for Same PF VLAN test.
			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU500, sriovResourcePF1MTU500, vlanID,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				"")
			Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN network for Same PF MTU 500")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU9000, sriovResourcePF1MTU9000, vlanID,
				tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2,
				"")
			Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN network for Same PF MTU 9000")
		})

		AfterEach(func() {
			By("Deleting test pods")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")
		})

		AfterAll(func() {
			By("Deleting SR-IOV networks")

			err = sriov.CleanAllNetworksByTargetNamespace(
				APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity with Static IPAM and Static MAC", reportxml.ID("87398"), func() {
			By("Creating client and server pods for MTU 500")

			clientMTU500, _, err = sriovenv.CreatePodPair(
				tsparams.ClientPodMTU500, tsparams.ServerPodMTU500,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU500, sriovNetworkSamePFMTU500,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
				tsparams.ClientMacAddress, tsparams.ServerMacAddress,
				[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
				mtu500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

			By("Creating client and server pods for MTU 9000")

			clientMTU9000, _, err = sriovenv.CreatePodPair(
				tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU9000, sriovNetworkSamePFMTU9000,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
				[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
				mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

			err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU500, clientMTU9000,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
				mtu500, mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
		})

		It("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM, Dynamic MAC, and VLAN",
			reportxml.ID("87399"), func() {
				By("Creating VLAN pods for MTU 500")

				vlanClientMTU500, vlanServerMTU500, err := sriovenv.CreatePodPair(
					tsparams.ClientPodVlanMTU500, tsparams.ServerPodVlanMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU500, sriovNetworkVlanSamePFMTU500,
					"", "", "",
					nil, nil, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 500")

				By("Creating VLAN pods for MTU 9000")

				vlanClientMTU9000, vlanServerMTU9000, err := sriovenv.CreatePodPair(
					tsparams.ClientPodVlanMTU9000, tsparams.ServerPodVlanMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU9000, sriovNetworkVlanSamePFMTU9000,
					"", "", "",
					nil, nil, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 9000")

				By("Getting server IPs from pod interfaces")

				serverIPMTU500, err := sriovenv.GetPodIPFromInterface(vlanServerMTU500, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 500")

				serverIPMTU9000, err := sriovenv.GetPodIPFromInterface(vlanServerMTU9000, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 9000")

				By("Running traffic tests over VLAN with dynamic IP for MTU 500")

				err = sriovenv.RunTrafficTest(vlanClientMTU500, serverIPMTU500, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN MTU 500")

				By("Running traffic tests over VLAN with dynamic IP for MTU 9000")

				err = sriovenv.RunTrafficTest(vlanClientMTU9000, serverIPMTU9000, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN MTU 9000")
			})
	})

	// Context for Same Node, Different PF connectivity tests.
	Context("Same Node Different PF", func() {
		const (
			sriovNetworkDiffPFClientMTU500            = "sriov-net-diffpf-client-mtu500"
			sriovNetworkDiffPFServerMTU500            = "sriov-net-diffpf-server-mtu500"
			sriovNetworkDiffPFClientMTU9000           = "sriov-net-diffpf-client-mtu9000"
			sriovNetworkDiffPFServerMTU9000           = "sriov-net-diffpf-server-mtu9000"
			sriovNetworkWhereaboutsDiffPFClientMTU500 = "sriov-net-wb-diffpf-client-mtu500"
			sriovNetworkWhereaboutsDiffPFServerMTU500 = "sriov-net-wb-diffpf-server-mtu500"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks for Same Node Different PF")
			// Client networks use PF1 resources.
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFClientMTU500, sriovNetworkDiffPFClientMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client networks")

			// Server networks use PF2 resources.
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFServerMTU500, sriovNetworkDiffPFServerMTU9000,
				sriovResourcePF2MTU500, sriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server networks")

			// Whereabouts networks for dynamic IP/MAC test.
			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFClientMTU500, sriovResourcePF1MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU500,
				"")
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts client network")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFServerMTU500, sriovResourcePF2MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU500,
				"")
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts server network")
		})

		AfterEach(func() {
			By("Deleting test pods")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")
		})

		AfterAll(func() {
			By("Deleting SR-IOV networks")

			err = sriov.CleanAllNetworksByTargetNamespace(
				APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity between different PFs on same node with Static IPAM and Static MAC",
			reportxml.ID("87400"), func() {
				By("Creating client and server pods for MTU 500")

				clientMTU500, _, err = sriovenv.CreatePodPair(
					tsparams.ClientPodMTU500, tsparams.ServerPodMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU500, sriovNetworkDiffPFServerMTU500,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress), tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
					mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

				By("Creating client and server pods for MTU 9000")

				clientMTU9000, _, err = sriovenv.CreatePodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU9000, sriovNetworkDiffPFServerMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
					[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU500, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					mtu500, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87401"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")

				whereaboutsClient, whereaboutsServer, err := sriovenv.CreatePodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkWhereaboutsDiffPFClientMTU500, sriovNetworkWhereaboutsDiffPFServerMTU500,
					"", "", "",
					nil, nil, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")

				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")

				err = sriovenv.RunTrafficTest(whereaboutsClient, serverIP, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
			})
	})

	// Context for Different Node connectivity tests.
	Context("Different Node", func() {
		const (
			sriovNetworkDiffNodeMTU500            = "sriov-net-diffnode-mtu500"
			sriovNetworkDiffNodeMTU9000           = "sriov-net-diffnode-mtu9000"
			sriovNetworkWhereaboutsDiffNodeMTU500 = "sriov-net-whereabouts-diffnode-mtu500"
		)

		BeforeAll(func() {
			By(fmt.Sprintf("Using client on node %s and server on node %s",
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name))

			By("Creating SR-IOV Networks for Different Node")

			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffNodeMTU500, sriovNetworkDiffNodeMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffNodeMTU500, sriovResourcePF1MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffNodeMTU500,
				"")
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts network for Different Node")
		})

		AfterEach(func() {
			By("Deleting test pods")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")
		})

		AfterAll(func() {
			By("Deleting SR-IOV networks")

			err = sriov.CleanAllNetworksByTargetNamespace(
				APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV networks")
		})

		It("Verify SR-IOV IPv4 connectivity between different nodes with Static IPAM and Dynamic MAC",
			reportxml.ID("87402"), func() {
				By("Creating client and server pods for MTU 500")

				clientMTU500, _, err = sriovenv.CreatePodPair(
					tsparams.ClientPodMTU500, tsparams.ServerPodMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU500, sriovNetworkDiffNodeMTU500,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
					mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

				By("Creating client and server pods for MTU 9000")

				clientMTU9000, _, err = sriovenv.CreatePodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU9000, sriovNetworkDiffNodeMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
					[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU500, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					mtu500, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv4 connectivity between different nodes with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87403"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")

				whereaboutsClient, whereaboutsServer, err := sriovenv.CreatePodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkWhereaboutsDiffNodeMTU500, sriovNetworkWhereaboutsDiffNodeMTU500,
					"", "", "",
					nil, nil, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")

				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")

				err = sriovenv.RunTrafficTest(whereaboutsClient, serverIP, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
			})
	})
})
