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
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("SR-IOV IPv6", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	const (
		// SR-IOV resource names for policies.
		sriovResourcePF1MTU1280 = "sriovpf1mtu1280v6"
		sriovResourcePF1MTU9000 = "sriovpf1mtu9000v6"
		sriovResourcePF2MTU1280 = "sriovpf2mtu1280v6"
		sriovResourcePF2MTU9000 = "sriovpf2mtu9000v6"
		// MTU values for testing. 1280 is the IPv6 minimum MTU.
		mtu1280 = 1280
		mtu9000 = 9000
	)

	var (
		workerNodeList    []*nodes.Builder
		sriovInterfaces   []string
		sriovInterfacePF1 string
		sriovInterfacePF2 string
		err               error
	)

	BeforeAll(func() {
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

		By("Verifying SCTP kernel module is loaded on worker nodes")
		sctpOutput, err := cluster.ExecCmdWithStdout(APIClient, "lsmod | grep -q sctp && echo loaded",
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		if err != nil || len(sctpOutput) == 0 {
			Skip("SCTP kernel module is not loaded on worker nodes - cluster must be pre-configured with SCTP")
		}

		By("Create all test case SR-IOV policies and waiting for them to be applied")
		err = sriovenv.CreateAllSriovPolicies(
			sriovInterfacePF1, sriovInterfacePF2,
			sriovResourcePF1MTU1280, sriovResourcePF1MTU9000,
			sriovResourcePF2MTU1280, sriovResourcePF2MTU9000,
			"ipv6", mtu1280, mtu9000)
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
			sriovNetworkSamePFMTU1280     = "sriov-net-samepf-mtu1280-v6"
			sriovNetworkSamePFMTU9000     = "sriov-net-samepf-mtu9000-v6"
			sriovNetworkVlanSamePFMTU1280 = "sriov-net-vlan-samepf-mtu1280-v6"
			sriovNetworkVlanSamePFMTU9000 = "sriov-net-vlan-samepf-mtu9000-v6"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks")
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkSamePFMTU1280, sriovNetworkSamePFMTU9000,
				sriovResourcePF1MTU1280, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			// VLAN network for Same PF VLAN test.
			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU1280, sriovResourcePF1MTU1280, vlanID,
				tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway)
			Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN network for Same PF MTU 1280")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU9000, sriovResourcePF1MTU9000, vlanID,
				tsparams.WhereaboutsIPv6Range2, tsparams.WhereaboutsIPv6Gateway2)
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

		It("Verify SR-IOV IPv6 connectivity with Static IPAM and Static MAC", reportxml.ID("87522"), func() {
			By("Creating client and server pods for MTU 1280")
			clientMTU1280, _, err := sriovenv.CreatePodPair(
				tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU1280, sriovNetworkSamePFMTU1280,
				[]string{tsparams.ClientIPv6IPAddress}, []string{tsparams.ServerIPv6IPAddress},
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
				tsparams.ClientMacAddress, tsparams.ServerMacAddress, mtu1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, _, err := sriovenv.CreatePodPair(
				tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU9000, sriovNetworkSamePFMTU9000,
				[]string{tsparams.ClientIPv6IPAddress2}, []string{tsparams.ServerIPv6IPAddress2},
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2, mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

			err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
				mtu1280, mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
		})

		It("Verify SR-IOV IPv6 connectivity with Whereabouts IPAM, Dynamic MAC, and VLAN",
			reportxml.ID("87558"), func() {
				By("Creating VLAN pods for MTU 1280")
				vlanClientMTU1280, vlanServerMTU1280, err := sriovenv.CreatePodPair(
					tsparams.ClientPodVlanMTU1280, tsparams.ServerPodVlanMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU1280, sriovNetworkVlanSamePFMTU1280,
					nil, nil, "", "", "", mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 1280")

				By("Creating VLAN pods for MTU 9000")
				vlanClientMTU9000, vlanServerMTU9000, err := sriovenv.CreatePodPair(
					tsparams.ClientPodVlanMTU9000, tsparams.ServerPodVlanMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU9000, sriovNetworkVlanSamePFMTU9000,
					nil, nil, "", "", "", mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 9000")

				By("Getting server IPs from pod interfaces")
				serverIPMTU1280, err := sriovenv.GetPodIPFromInterface(vlanServerMTU1280, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 1280")

				serverIPMTU9000, err := sriovenv.GetPodIPFromInterface(vlanServerMTU9000, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 9000")

				By("Running traffic tests over VLAN with dynamic IP for MTU 1280")
				err = sriovenv.RunTrafficTest(vlanClientMTU1280, serverIPMTU1280, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN MTU 1280")

				By("Running traffic tests over VLAN with dynamic IP for MTU 9000")
				err = sriovenv.RunTrafficTest(vlanClientMTU9000, serverIPMTU9000, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN MTU 9000")
			})
	})

	// Context for Same Node, Different PF connectivity tests.
	Context("Same Node Different PF", func() {
		const (
			sriovNetworkDiffPFClientMTU1280            = "sriov-net-diffpf-client-mtu1280-v6"
			sriovNetworkDiffPFServerMTU1280            = "sriov-net-diffpf-server-mtu1280-v6"
			sriovNetworkDiffPFClientMTU9000            = "sriov-net-diffpf-client-mtu9000-v6"
			sriovNetworkDiffPFServerMTU9000            = "sriov-net-diffpf-server-mtu9000-v6"
			sriovNetworkWhereaboutsDiffPFClientMTU1280 = "sriov-net-wb-diffpf-client-mtu1280-v6"
			sriovNetworkWhereaboutsDiffPFServerMTU1280 = "sriov-net-wb-diffpf-server-mtu1280-v6"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks for Same Node Different PF")
			// Client networks use PF1 resources.
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFClientMTU1280, sriovNetworkDiffPFClientMTU9000,
				sriovResourcePF1MTU1280, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client networks")

			// Server networks use PF2 resources.
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFServerMTU1280, sriovNetworkDiffPFServerMTU9000,
				sriovResourcePF2MTU1280, sriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server networks")

			// Whereabouts networks for dynamic IP/MAC test.
			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFClientMTU1280, sriovResourcePF1MTU1280,
				tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts client network")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFServerMTU1280, sriovResourcePF2MTU1280,
				tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU1280)
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

		It("Verify SR-IOV IPv6 connectivity between different PFs on same node with Static IPAM and Static MAC",
			reportxml.ID("87559"), func() {
				By("Creating client and server pods for MTU 1280")
				clientMTU1280, _, err := sriovenv.CreatePodPair(
					tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU1280, sriovNetworkDiffPFServerMTU1280,
					[]string{tsparams.ClientIPv6IPAddress}, []string{tsparams.ServerIPv6IPAddress},
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress), tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

				By("Creating client and server pods for MTU 9000")
				clientMTU9000, _, err := sriovenv.CreatePodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU9000, sriovNetworkDiffPFServerMTU9000,
					[]string{tsparams.ClientIPv6IPAddress2}, []string{tsparams.ServerIPv6IPAddress2},
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					mtu1280, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv6 connectivity with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87560"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")
				whereaboutsClient, whereaboutsServer, err := sriovenv.CreatePodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkWhereaboutsDiffPFClientMTU1280, sriovNetworkWhereaboutsDiffPFServerMTU1280,
					nil, nil, "", "", "", mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")
				err = sriovenv.RunTrafficTest(whereaboutsClient, serverIP, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
			})
	})

	// Context for Different Node connectivity tests.
	Context("Different Node", func() {
		const (
			sriovNetworkDiffNodeMTU1280            = "sriov-net-diffnode-mtu1280-v6"
			sriovNetworkDiffNodeMTU9000            = "sriov-net-diffnode-mtu9000-v6"
			sriovNetworkWhereaboutsDiffNodeMTU1280 = "sriov-net-whereabouts-diffnode-mtu1280-v6"
		)

		BeforeAll(func() {
			By(fmt.Sprintf("Using client on node %s and server on node %s",
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name))

			By("Creating SR-IOV Networks for Different Node")
			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffNodeMTU1280, sriovNetworkDiffNodeMTU9000,
				sriovResourcePF1MTU1280, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffNodeMTU1280, sriovResourcePF1MTU1280,
				tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway,
				sriovNetworkWhereaboutsDiffNodeMTU1280)
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

		It("Verify SR-IOV IPv6 connectivity between different nodes with Static IPAM and Dynamic MAC",
			reportxml.ID("87565"), func() {
				By("Creating client and server pods for MTU 1280")
				clientMTU1280, _, err := sriovenv.CreatePodPair(
					tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU1280, sriovNetworkDiffNodeMTU1280,
					[]string{tsparams.ClientIPv6IPAddress}, []string{tsparams.ServerIPv6IPAddress},
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					"", "", mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

				By("Creating client and server pods for MTU 9000")
				clientMTU9000, _, err := sriovenv.CreatePodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU9000, sriovNetworkDiffNodeMTU9000,
					[]string{tsparams.ClientIPv6IPAddress2}, []string{tsparams.ServerIPv6IPAddress2},
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					"", "", mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = sriovenv.RunTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					mtu1280, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv6 connectivity between different nodes with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87566"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")
				whereaboutsClient, whereaboutsServer, err := sriovenv.CreatePodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkWhereaboutsDiffNodeMTU1280, sriovNetworkWhereaboutsDiffNodeMTU1280,
					nil, nil, "", "", "", mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")
				err = sriovenv.RunTrafficTest(whereaboutsClient, serverIP, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
			})
	})
})
