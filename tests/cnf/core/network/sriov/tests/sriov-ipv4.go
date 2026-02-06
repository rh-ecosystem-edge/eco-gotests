package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
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
		clientPodMTU500         = "client-mtu500"
		serverPodMTU500         = "server-mtu500"
		clientPodMTU9000        = "client-mtu9000"
		serverPodMTU9000        = "server-mtu9000"
		clientPodWhereabouts    = "client-whereabouts"
		serverPodWhereabouts    = "server-whereabouts"
		clientPodVlanMTU500     = "client-vlan-mtu500"
		serverPodVlanMTU500     = "server-vlan-mtu500"
		clientPodVlanMTU9000    = "client-vlan-mtu9000"
		serverPodVlanMTU9000    = "server-vlan-mtu9000"
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
		err = createAllSriovPolicies(
			sriovInterfacePF1, sriovInterfacePF2,
			sriovResourcePF1MTU500, sriovResourcePF1MTU9000,
			sriovResourcePF2MTU500, sriovResourcePF2MTU9000,
			mtu500, mtu9000)
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
			err = createSriovNetworksForBothMTUs(
				sriovNetworkSamePFMTU500, sriovNetworkSamePFMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			// VLAN network for Same PF VLAN test.
			vlanID, err := NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU500, sriovResourcePF1MTU500, vlanID,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway)
			Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN network for Same PF MTU 500")

			err = sriovenv.CreateSriovNetworkWithVLANAndWhereabouts(
				sriovNetworkVlanSamePFMTU9000, sriovResourcePF1MTU9000, vlanID,
				tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2)
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
			clientMTU500, _, err = createPodPair(
				clientPodMTU500, serverPodMTU500,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU500, sriovNetworkSamePFMTU500,
				[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
				tsparams.ClientMacAddress, tsparams.ServerMacAddress, mtu500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

			By("Creating client and server pods for MTU 9000")
			clientMTU9000, _, err = createPodPair(
				clientPodMTU9000, serverPodMTU9000,
				workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovNetworkSamePFMTU9000, sriovNetworkSamePFMTU9000,
				[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
				ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2, mtu9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

			err = runTrafficTestsForBothMTUs(clientMTU500, clientMTU9000)
			Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
		})

		It("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM, Dynamic MAC, and VLAN",
			reportxml.ID("87399"), func() {
				By("Creating VLAN pods for MTU 500")
				vlanClientMTU500, vlanServerMTU500, err := createPodPair(
					clientPodVlanMTU500, serverPodVlanMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU500, sriovNetworkVlanSamePFMTU500,
					nil, nil, "", "", "", mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 500")

				By("Creating VLAN pods for MTU 9000")
				vlanClientMTU9000, vlanServerMTU9000, err := createPodPair(
					clientPodVlanMTU9000, serverPodVlanMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkVlanSamePFMTU9000, sriovNetworkVlanSamePFMTU9000,
					nil, nil, "", "", "", mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN pods for MTU 9000")

				By("Getting server IPs from pod interfaces")
				serverIPMTU500, err := sriovenv.GetPodIPFromInterface(vlanServerMTU500, tsparams.Net1Interface)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 500")

				serverIPMTU9000, err := sriovenv.GetPodIPFromInterface(vlanServerMTU9000, tsparams.Net1Interface)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from VLAN pod for MTU 9000")

				By("Running traffic tests over VLAN with dynamic IP for MTU 500")
				err = runTrafficTest(vlanClientMTU500, serverIPMTU500, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for VLAN MTU 500")

				By("Running traffic tests over VLAN with dynamic IP for MTU 9000")
				err = runTrafficTest(vlanClientMTU9000, serverIPMTU9000, mtu9000)
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
			err = createSriovNetworksForBothMTUs(
				sriovNetworkDiffPFClientMTU500, sriovNetworkDiffPFClientMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client networks")

			// Server networks use PF2 resources.
			err = createSriovNetworksForBothMTUs(
				sriovNetworkDiffPFServerMTU500, sriovNetworkDiffPFServerMTU9000,
				sriovResourcePF2MTU500, sriovResourcePF2MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create server networks")

			// Whereabouts networks for dynamic IP/MAC test.
			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFClientMTU500, sriovResourcePF1MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU500)
			Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts client network")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffPFServerMTU500, sriovResourcePF2MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffPFClientMTU500)
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
				clientMTU500, _, err = createPodPair(
					clientPodMTU500, serverPodMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU500, sriovNetworkDiffPFServerMTU500,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress), tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

				By("Creating client and server pods for MTU 9000")
				clientMTU9000, _, err = createPodPair(
					clientPodMTU9000, serverPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkDiffPFClientMTU9000, sriovNetworkDiffPFServerMTU9000,
					[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2), tsparams.ClientMacAddress2, tsparams.ServerMacAddress2,
					mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = runTrafficTestsForBothMTUs(clientMTU500, clientMTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv4 connectivity with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87401"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")
				whereaboutsClient, whereaboutsServer, err := createPodPair(
					clientPodWhereabouts, serverPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
					sriovNetworkWhereaboutsDiffPFClientMTU500, sriovNetworkWhereaboutsDiffPFServerMTU500,
					nil, nil, "", "", "", mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")
				err = runTrafficTest(whereaboutsClient, serverIP, mtu500)
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
			err = createSriovNetworksForBothMTUs(
				sriovNetworkDiffNodeMTU500, sriovNetworkDiffNodeMTU9000,
				sriovResourcePF1MTU500, sriovResourcePF1MTU9000)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV networks")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				sriovNetworkWhereaboutsDiffNodeMTU500, sriovResourcePF1MTU500,
				tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
				sriovNetworkWhereaboutsDiffNodeMTU500)
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
				clientMTU500, _, err = createPodPair(
					clientPodMTU500, serverPodMTU500,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU500, sriovNetworkDiffNodeMTU500,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress},
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress),
					tsparams.ClientMacAddress, tsparams.ServerMacAddress, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 500")

				By("Creating client and server pods for MTU 9000")
				clientMTU9000, _, err = createPodPair(
					clientPodMTU9000, serverPodMTU9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkDiffNodeMTU9000, sriovNetworkDiffNodeMTU9000,
					[]string{tsparams.ClientIPv4IPAddress2}, []string{tsparams.ServerIPv4IPAddress2},
					ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2),
					tsparams.ClientMacAddress2, tsparams.ServerMacAddress2, mtu9000)
				Expect(err).ToNot(HaveOccurred(), "Failed to create pods for MTU 9000")

				err = runTrafficTestsForBothMTUs(clientMTU500, clientMTU9000)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed")
			})

		It("Verify SR-IOV IPv4 connectivity between different nodes with Whereabouts IPAM and Dynamic MAC",
			reportxml.ID("87403"), func() {
				By("Creating whereabouts pods with dynamic IP/MAC")
				whereaboutsClient, whereaboutsServer, err := createPodPair(
					clientPodWhereabouts, serverPodWhereabouts,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					sriovNetworkWhereaboutsDiffNodeMTU500, sriovNetworkWhereaboutsDiffNodeMTU500,
					nil, nil, "", "", "", mtu500)
				Expect(err).ToNot(HaveOccurred(), "Failed to create whereabouts pods")

				By("Getting server IP from pod interface")
				serverIP, err := sriovenv.GetPodIPFromInterface(whereaboutsServer, tsparams.Net1Interface)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server IP from whereabouts pod")

				By("Running traffic tests with whereabouts IPAM")
				err = runTrafficTest(whereaboutsClient, serverIP, mtu500)
				Expect(err).ToNot(HaveOccurred(), "Traffic tests failed for whereabouts IPAM")
			})
	})
})

// createPodPair creates a client and server pod pair for traffic testing.
func createPodPair(
	clientName string,
	serverName string,
	clientNode string,
	serverNode string,
	clientNetwork string,
	serverNetwork string,
	clientIPs []string,
	serverIPs []string,
	serverBindIP string,
	clientMAC string,
	serverMAC string,
	mtu int,
) (*pod.Builder, *pod.Builder, error) {
	By(fmt.Sprintf("Creating client pod %s and server pod %s", clientName, serverName))

	client, err := createTestClientPod(clientName, clientNode, clientNetwork, clientIPs, clientMAC)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client pod: %w", err)
	}

	server, err := createTestServerPod(
		serverName, serverNode, serverNetwork, serverIPs, serverBindIP, serverMAC, mtu)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server pod: %w", err)
	}

	return client, server, nil
}

// createAllSriovPolicies creates all SR-IOV policies for IPv4 testing.
// It creates policies for PF1 and PF2 at MTU 500 and MTU 9000.
// VF allocation: 10 total VFs per PF, VFs 0-4 for MTU 500, VFs 5-9 for MTU 9000.
func createAllSriovPolicies(
	pf1 string,
	pf2 string,
	resourcePF1MTU500 string,
	resourcePF1MTU9000 string,
	resourcePF2MTU500 string,
	resourcePF2MTU9000 string,
	mtu500 int,
	mtu9000 int,
) error {
	By("Creating SR-IOV policies for IPv4 testing")

	const (
		vfStartMTU500  = 0
		vfEndMTU500    = 4
		vfStartMTU9000 = 5
		vfEndMTU9000   = 9
	)

	// Create policy for PF1 with MTU 500
	if err := createSriovPolicy(
		"ipv4-policy-pf1-mtu500",
		resourcePF1MTU500, pf1, mtu500,
		vfStartMTU500, vfEndMTU500); err != nil {
		return fmt.Errorf("failed to create PF1 MTU500 policy: %w", err)
	}

	// Create policy for PF1 with MTU 9000
	if err := createSriovPolicy(
		"ipv4-policy-pf1-mtu9000",
		resourcePF1MTU9000, pf1, mtu9000,
		vfStartMTU9000, vfEndMTU9000); err != nil {
		return fmt.Errorf("failed to create PF1 MTU9000 policy: %w", err)
	}

	// Create policy for PF2 with MTU 500
	if err := createSriovPolicy(
		"ipv4-policy-pf2-mtu500",
		resourcePF2MTU500, pf2, mtu500,
		vfStartMTU500, vfEndMTU500); err != nil {
		return fmt.Errorf("failed to create PF2 MTU500 policy: %w", err)
	}

	// Create policy for PF2 with MTU 9000
	if err := createSriovPolicy(
		"ipv4-policy-pf2-mtu9000",
		resourcePF2MTU9000, pf2, mtu9000,
		vfStartMTU9000, vfEndMTU9000); err != nil {
		return fmt.Errorf("failed to create PF2 MTU9000 policy: %w", err)
	}

	if err := sriovoperator.WaitForSriovAndMCPStable(
		APIClient,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration,
		NetConfig.WorkerLabelEnvVar,
		NetConfig.SriovOperatorNamespace); err != nil {
		return fmt.Errorf("failed to wait for SR-IOV and MCP stability: %w", err)
	}

	return nil
}

// createSriovPolicy creates a single SR-IOV policy without waiting for MCP stability.
func createSriovPolicy(
	name string,
	resourceName string,
	pfName string,
	mtu int,
	vfStart int,
	vfEnd int,
) error {
	By(fmt.Sprintf("Creating SR-IOV policy %s", name))

	const totalVFs = 10

	policy := sriov.NewPolicyBuilder(
		APIClient,
		name,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		totalVFs,
		[]string{pfName},
		NetConfig.WorkerLabelMap).
		WithMTU(mtu).
		WithVFRange(vfStart, vfEnd)

	_, err := policy.Create()
	if err != nil {
		return err
	}

	return nil
}

// createTestClientPod creates a client pod with SR-IOV interface.
func createTestClientPod(
	name string,
	nodeName string,
	networkName string,
	ipAddresses []string,
	macAddress string,
) (*pod.Builder, error) {
	By(fmt.Sprintf("Creating client pod %s on node %s", name, nodeName))

	secNetwork := []*types.NetworkSelectionElement{{Name: networkName}}

	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	command := []string{"bash", "-c", "sleep infinity"}

	container, err := pod.NewContainerBuilder("test", NetConfig.CnfNetTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	return pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		RedefineDefaultContainer(*container).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
}

// createTestServerPod creates a server pod with testcmd listeners for TCP, UDP, SCTP, and multicast.
func createTestServerPod(
	name string,
	nodeName string,
	networkName string,
	ipAddresses []string,
	serverBindIP string,
	macAddress string,
	mtu int,
) (*pod.Builder, error) {
	By(fmt.Sprintf("Creating server pod %s on node %s", name, nodeName))

	secNetwork := []*types.NetworkSelectionElement{{Name: networkName}}

	if macAddress != "" {
		secNetwork[0].MacRequest = macAddress
	}

	if len(ipAddresses) > 0 {
		secNetwork[0].IPRequest = ipAddresses
	}

	ipv4MulticastGroup := "239.100.0.250"
	interfaceName := tsparams.Net1Interface

	if mtu > 1500 {
		ipv4MulticastGroup = "239.100.100.250"
	}

	command := buildServerCommand(serverBindIP, interfaceName, ipv4MulticastGroup, mtu)

	container, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, command).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create container config: %w", err)
	}

	serverPod, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		RedefineDefaultContainer(*container).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	if err != nil {
		return nil, err
	}

	// Wait for testcmd listeners to be ready.
	if err := waitForServerReady(serverPod, tsparams.WaitTimeout); err != nil {
		return nil, fmt.Errorf("server pod %s not ready: %w", name, err)
	}

	return serverPod, nil
}

// waitForServerReady waits for the server pod's testcmd listeners to be ready.
func waitForServerReady(serverPod *pod.Builder, timeout time.Duration) error {
	By(fmt.Sprintf("Waiting for server pod %s to be ready", serverPod.Definition.Name))

	var lastErr error

	Eventually(func() error {
		_, lastErr = serverPod.ExecCommand([]string{"bash", "-c", "pgrep -f testcmd"})

		return lastErr
	}, timeout, tsparams.RetryInterval).Should(Succeed(),
		fmt.Sprintf("testcmd listeners not ready on pod %s", serverPod.Definition.Name))

	return lastErr
}

func buildServerCommand(
	serverBindIP, interfaceName, ipv4MulticastGroup string, packetSize int) []string {
	// All protocols need smaller packet size to account for protocol headers (IP + UDP/TCP/SCTP).
	// Subtract 100 bytes to provide headroom for headers and avoid "message too long" errors.
	packetSize -= 100

	if serverBindIP == "" {
		// Dynamic IP: discover IPv4 from net1 interface at runtime.
		// Use ip -4 to ensure only IPv4 addresses are returned, avoiding link-local IPv6.
		return []string{"bash", "-c", fmt.Sprintf(
			"for _ in $(seq 1 10); do "+
				"SERVER_IP=$(ip -4 -o addr show %s | awk '{print $4}' | cut -d'/' -f1 | head -1); "+
				"[ -n \"$SERVER_IP\" ] && break; "+
				"sleep 1; "+
				"done; "+
				"[ -n \"$SERVER_IP\" ] || { echo \"Failed to discover server IP\"; exit 1; }; "+
				"echo \"Discovered server IP: $SERVER_IP\"; "+
				"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
				"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
				"testcmd -listen -protocol sctp -port 5003 -interface %s -server $SERVER_IP -mtu %d & "+
				"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & "+
				"sleep infinity",
			interfaceName,
			interfaceName, packetSize,
			interfaceName, packetSize,
			interfaceName, packetSize,
			interfaceName, ipv4MulticastGroup, packetSize)}
	}

	// Static IP: use provided serverBindIP.
	multicastGroup := ipv4MulticastGroup

	if strings.Contains(serverBindIP, ":") {
		multicastGroup = "ff02::1"
	}

	return []string{"bash", "-c", fmt.Sprintf(
		"sleep 5; "+
			"testcmd -listen -protocol tcp -port 5001 -interface %s -mtu %d & "+
			"testcmd -listen -protocol udp -port 5002 -interface %s -mtu %d & "+
			"testcmd -listen -protocol sctp -port 5003 -interface %s -server %s -mtu %d & "+
			"testcmd -listen -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d & "+
			"sleep infinity",
		interfaceName, packetSize,
		interfaceName, packetSize,
		interfaceName, serverBindIP, packetSize,
		interfaceName, multicastGroup, packetSize)}
}

// runTrafficTestsForBothMTUs runs traffic tests for both MTU 500 and MTU 9000.
func runTrafficTestsForBothMTUs(clientMTU500, clientMTU9000 *pod.Builder) error {
	By("Running traffic tests with MTU 500")

	err := runTrafficTest(clientMTU500, ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress), 500)
	if err != nil {
		return fmt.Errorf("traffic tests failed for MTU 500: %w", err)
	}

	By("Running traffic tests with MTU 9000")

	err = runTrafficTest(clientMTU9000, ipaddr.RemovePrefix(tsparams.ServerIPv4IPAddress2), 9000)
	if err != nil {
		return fmt.Errorf("traffic tests failed for MTU 9000: %w", err)
	}

	return nil
}

// createSriovNetworksForBothMTUs creates SR-IOV networks for both MTU 500 and MTU 9000.
func createSriovNetworksForBothMTUs(
	networkNameMTU500,
	networkNameMTU9000,
	resourceMTU500,
	resourceMTU9000 string,
) error {
	By("Creating SR-IOV networks for MTU 500 and MTU 9000")

	err := sriovenv.CreateSriovNetworkWithStaticIPAM(networkNameMTU500, resourceMTU500)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for MTU 500: %w", err)
	}

	err = sriovenv.CreateSriovNetworkWithStaticIPAM(networkNameMTU9000, resourceMTU9000)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for MTU 9000: %w", err)
	}

	return nil
}

// runTrafficTest runs all traffic type tests (ICMP, TCP, UDP, SCTP, multicast) between client and server pods.
func runTrafficTest(clientPod *pod.Builder, serverIP string, mtu int) error {
	By(fmt.Sprintf("Running traffic tests against %s with MTU %d", serverIP, mtu))
	serverIPAddress := ipaddr.RemovePrefix(serverIP)

	// All protocols need smaller packet size to account for protocol headers (IP + UDP/TCP/SCTP).
	// Subtract 100 bytes to provide headroom for headers and avoid "message too long" errors.
	packetSize := mtu - 100

	var failedProtocols []string

	serverIPWithPrefix := serverIPAddress + "/32"
	if strings.Contains(serverIPAddress, ":") {
		serverIPWithPrefix = serverIPAddress + "/128"
	}

	if err := cmd.ICMPConnectivityCheck(
		clientPod, []string{serverIPWithPrefix}, tsparams.Net1Interface); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("ICMP: %v", err))
	}

	if err := runProtocolTest(clientPod, "TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("TCP: %v", err))
	}

	if err := runProtocolTest(clientPod, "UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("UDP: %v", err))
	}

	if err := runProtocolTest(clientPod, "SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, serverIPAddress, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("SCTP: %v", err))
	}

	// Multicast group selection:
	// - MTU 500 uses 239.100.0.250
	// - MTU 9000 uses 239.100.100.250
	// - IPv6 uses ff02::1 regardless of MTU
	multicastGroup := "239.100.0.250"
	if strings.Contains(serverIPAddress, ":") {
		multicastGroup = "ff02::1"
	} else if mtu == 9000 {
		multicastGroup = "239.100.100.250"
	}

	if err := runProtocolTest(clientPod, "multicast",
		fmt.Sprintf("testcmd -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d",
			tsparams.Net1Interface, multicastGroup, packetSize)); err != nil {
		failedProtocols = append(failedProtocols, fmt.Sprintf("multicast: %v", err))
	}

	if len(failedProtocols) > 0 {
		return fmt.Errorf("traffic tests failed: %s", strings.Join(failedProtocols, "; "))
	}

	return nil
}

// runProtocolTest executes a protocol-specific connectivity test command.
func runProtocolTest(clientPod *pod.Builder, protocol, cmdStr string) error {
	By(fmt.Sprintf("Running %s connectivity test", protocol))

	output, err := clientPod.ExecCommand([]string{"bash", "-c", cmdStr})
	if err != nil {
		return fmt.Errorf("%s connectivity check failed (output: %s): %w", protocol, output.String(), err)
	}

	return nil
}
