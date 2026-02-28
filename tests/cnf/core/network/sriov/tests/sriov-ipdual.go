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

var _ = Describe("SR-IOV DualStack", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	const (
		sriovResourcePF1MTU1280 = "sriovpf1mtu1280ds"
		sriovResourcePF1MTU9000 = "sriovpf1mtu9000ds"
		sriovResourcePF2MTU1280 = "sriovpf2mtu1280ds"
		sriovResourcePF2MTU9000 = "sriovpf2mtu9000ds"
		mtu1280                 = 1280
		mtu9000                 = 9000
	)

	var (
		workerNodeList    []*nodes.Builder
		sriovInterfaces   []string
		sriovInterfacePF1 string
		sriovInterfacePF2 string
		err               error
	)

	BeforeAll(func() {
		By("Checking cluster IP family")

		clusterIPFamily, err := netenv.GetClusterIPFamily(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to detect cluster IP family")

		if !netenv.ClusterSupportsDualStack(clusterIPFamily) {
			Skip("Cluster is not dual-stack - skipping dual-stack SR-IOV tests")
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
		Expect(err).ToNot(HaveOccurred(), "Failed to check SCTP kernel module on worker nodes")
		Expect(sctpOutput).NotTo(BeEmpty(),
			"SCTP kernel module must be loaded on workers for this suite (traffic tests require SCTP); "+
				"configure SCTP per tests/cnf/core/network/README prerequisites (e.g. MachineConfig)")

		By("Create all test case SR-IOV policies and waiting for them to be applied")

		err = sriovenv.CreateAllSriovPolicies(
			sriovInterfacePF1, sriovInterfacePF2,
			sriovResourcePF1MTU1280, sriovResourcePF1MTU9000,
			sriovResourcePF2MTU1280, sriovResourcePF2MTU9000,
			"dualstack", mtu1280, mtu9000)
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

	Context("Same Node Same PF", func() {
		const (
			sriovNetworkSamePFMTU1280     = "sriov-net-samepf-mtu1280-ds"
			sriovNetworkSamePFMTU9000     = "sriov-net-samepf-mtu9000-ds"
			sriovNetworkVlanSamePFMTU1280 = "sriov-net-vlan-samepf-mtu1280-ds"
			sriovNetworkVlanSamePFMTU9000 = "sriov-net-vlan-samepf-mtu9000-ds"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks")

			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkSamePFMTU1280, sriovNetworkSamePFMTU9000,
				sriovResourcePF1MTU1280, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU1280, sriovResourcePF1MTU1280, vlanID,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				tsparams.WhereaboutsIPv6Range)
			Expect(err).ToNot(HaveOccurred(), "Failed to create dual-stack VLAN network for Same PF MTU 1280")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU9000, sriovResourcePF1MTU9000, vlanID,
				tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2,
				tsparams.WhereaboutsIPv6Range2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create dual-stack VLAN network for Same PF MTU 9000")
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
		It("Verify SR-IOV dual-stack connectivity with Static IPAM and Static MAC", reportxml.ID("88141"), func() {
			By("Creating client and server pods for MTU 1280")

			clientMTU1280, _, err := CreateDualStackPodPair(
				tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU1280, sriovNetworkSamePFMTU1280,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
				tsparams.ClientMacAddress, tsparams.ServerMacAddress,
				[]string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress},
				[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress},
				mtu1280)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

			By("Creating client and server pods for MTU 9000")

			clientMTU9000, _, err := CreateDualStackPodPair(
				tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU9000, sriovNetworkSamePFMTU9000,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
				tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
				[]string{tsparams.ClientIPv4IPAddress2, tsparams.ClientIPv6IPAddress2},
				[]string{tsparams.ServerIPv4IPAddress2, tsparams.ServerIPv6IPAddress2},
				mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

			err = RunDualStackTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
				ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
				mtu1280, mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed")
		})

		It("Verify SR-IOV dual-stack connectivity with Whereabouts IPAM, Dynamic MAC, and VLAN",
			reportxml.ID("88142"), func() {
				By("Creating VLAN pods for MTU 1280")

				vlanClientMTU1280, vlanServerMTU1280, err := CreateDualStackPodPair(
					tsparams.ClientPodVlanMTU1280, tsparams.ServerPodVlanMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU1280, sriovNetworkVlanSamePFMTU1280,
					"", "", "", "",
					nil, nil, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 1280")

				By("Creating VLAN pods for MTU 9000")

				vlanClientMTU9000, vlanServerMTU9000, err := CreateDualStackPodPair(
					tsparams.ClientPodVlanMTU9000, tsparams.ServerPodVlanMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU9000, sriovNetworkVlanSamePFMTU9000,
					"", "", "", "",
					nil, nil, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 9000")

				By("Getting server IPs from pod interfaces")

				serverIPv4MTU1280, err := sriovenv.GetPodIPFromInterface(
					vlanServerMTU1280, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv4 server IP for MTU 1280")

				serverIPv6MTU1280, err := sriovenv.GetPodIPFromInterface(
					vlanServerMTU1280, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv6 server IP for MTU 1280")

				serverIPv4MTU9000, err := sriovenv.GetPodIPFromInterface(
					vlanServerMTU9000, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv4 server IP for MTU 9000")

				serverIPv6MTU9000, err := sriovenv.GetPodIPFromInterface(
					vlanServerMTU9000, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv6 server IP for MTU 9000")

				By("Running dual-stack traffic tests over VLAN for MTU 1280")

				err = RunDualStackTrafficTest(vlanClientMTU1280,
					serverIPv4MTU1280, serverIPv6MTU1280, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed for VLAN MTU 1280")

				By("Running dual-stack traffic tests over VLAN for MTU 9000")

				err = RunDualStackTrafficTest(vlanClientMTU9000,
					serverIPv4MTU9000, serverIPv6MTU9000, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed for VLAN MTU 9000")
			})
	})

	Context("Same Node Different PF", func() {
		const (
			sriovNetworkDiffPFClientMTU1280            = "sriov-net-diffpf-client-mtu1280-ds"
			sriovNetworkDiffPFServerMTU1280            = "sriov-net-diffpf-server-mtu1280-ds"
			sriovNetworkDiffPFClientMTU9000            = "sriov-net-diffpf-client-mtu9000-ds"
			sriovNetworkDiffPFServerMTU9000            = "sriov-net-diffpf-server-mtu9000-ds"
			sriovNetworkWhereaboutsDiffPFClientMTU1280 = "sriov-net-wb-diffpf-client-mtu1280-ds"
			sriovNetworkWhereaboutsDiffPFServerMTU1280 = "sriov-net-wb-diffpf-server-mtu1280-ds"
		)

		BeforeAll(func() {
			By("Creating SR-IOV Networks for Same Node Different PF")

			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFClientMTU1280, sriovNetworkDiffPFClientMTU9000,
				sriovResourcePF1MTU1280, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client networks")

			err = sriovenv.CreateSriovNetworksForBothMTUs(
				sriovNetworkDiffPFServerMTU1280, sriovNetworkDiffPFServerMTU9000,
				sriovResourcePF2MTU1280, sriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server networks")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFClientMTU1280, sriovResourcePF1MTU1280,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU1280,
				tsparams.WhereaboutsIPv6Range)
			Expect(err).ToNot(HaveOccurred(), "Failed to create dual-stack whereabouts client network")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFServerMTU1280, sriovResourcePF2MTU1280,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU1280,
				tsparams.WhereaboutsIPv6Range)
			Expect(err).ToNot(HaveOccurred(), "Failed to create dual-stack whereabouts server network")
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

		It("Verify SR-IOV dual-stack connectivity between different PFs with Static IPAM and Static MAC",
			reportxml.ID("88143"), func() {
				By("Creating client and server pods for MTU 1280")

				clientMTU1280, _, err := CreateDualStackPodPair(
					tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU1280, sriovNetworkDiffPFServerMTU1280,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress},
					[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress},
					mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

				By("Creating client and server pods for MTU 9000")

				clientMTU9000, _, err := CreateDualStackPodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU9000, sriovNetworkDiffPFServerMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
					[]string{tsparams.ClientIPv4IPAddress2, tsparams.ClientIPv6IPAddress2},
					[]string{tsparams.ServerIPv4IPAddress2, tsparams.ServerIPv6IPAddress2},
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = RunDualStackTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					mtu1280, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed")
			})

		It("Verify SR-IOV dual-stack connectivity with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("88144"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")

				whereaboutsClient, whereaboutsServer, err := CreateDualStackPodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkWhereaboutsDiffPFClientMTU1280, sriovNetworkWhereaboutsDiffPFServerMTU1280,
					"", "", "", "",
					nil, nil, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IPs from pod interface")

				serverIPv4, err := sriovenv.GetPodIPFromInterface(
					whereaboutsServer, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv4 server IP")

				serverIPv6, err := sriovenv.GetPodIPFromInterface(
					whereaboutsServer, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv6 server IP")

				By("Running dual-stack traffic tests with whereabouts IPAM")

				err = RunDualStackTrafficTest(whereaboutsClient, serverIPv4, serverIPv6, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed for whereabouts IPAM")
			})
	})

	Context("Different Node", func() {
		const (
			sriovNetworkDiffNodeMTU1280            = "sriov-net-diffnode-mtu1280-ds"
			sriovNetworkDiffNodeMTU9000            = "sriov-net-diffnode-mtu9000-ds"
			sriovNetworkWhereaboutsDiffNodeMTU1280 = "sriov-net-whereabouts-diffnode-mtu1280-ds"
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
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffNodeMTU1280,
				tsparams.WhereaboutsIPv6Range)
			Expect(err).ToNot(HaveOccurred(), "Failed to create dual-stack whereabouts network for Different Node")
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

		It("Verify SR-IOV dual-stack connectivity between different nodes with Static IPAM and Dynamic MAC",
			reportxml.ID("88145"), func() {
				By("Creating client and server pods for MTU 1280")

				clientMTU1280, _, err := CreateDualStackPodPair(
					tsparams.ClientPodMTU1280, tsparams.ServerPodMTU1280,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU1280, sriovNetworkDiffNodeMTU1280,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					"", "",
					[]string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress},
					[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress},
					mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 1280")

				By("Creating client and server pods for MTU 9000")

				clientMTU9000, _, err := CreateDualStackPodPair(
					tsparams.ClientPodMTU9000, tsparams.ServerPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU9000, sriovNetworkDiffNodeMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					"", "",
					[]string{tsparams.ClientIPv4IPAddress2, tsparams.ClientIPv6IPAddress2},
					[]string{tsparams.ServerIPv4IPAddress2, tsparams.ServerIPv6IPAddress2},
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = RunDualStackTrafficTestsForBothMTUs(clientMTU1280, clientMTU9000,
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress),
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					ipaddr.RemovePrefix(tsparams.ServerIPv6IPAddress2),
					mtu1280, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed")
			})

		It("Verify SR-IOV dual-stack connectivity between different nodes with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("88146"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")

				whereaboutsClient, whereaboutsServer, err := CreateDualStackPodPair(
					tsparams.ClientPodWhereabouts, tsparams.ServerPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkWhereaboutsDiffNodeMTU1280, sriovNetworkWhereaboutsDiffNodeMTU1280,
					"", "", "", "",
					nil, nil, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IPs from pod interface")

				serverIPv4, err := sriovenv.GetPodIPFromInterface(
					whereaboutsServer, tsparams.Net1Interface, "ipv4")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv4 server IP")

				serverIPv6, err := sriovenv.GetPodIPFromInterface(
					whereaboutsServer, tsparams.Net1Interface, "ipv6")
				Expect(err).ToNot(HaveOccurred(), "Failed to get IPv6 server IP")

				By("Running dual-stack traffic tests with whereabouts IPAM")

				err = RunDualStackTrafficTest(whereaboutsClient, serverIPv4, serverIPv6, mtu1280)
				Expect(err).ToNot(HaveOccurred(), "Dual-stack traffic tests failed for whereabouts IPAM")
			})
	})
})
