package tests

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelSriovVRFDhcpTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
		dhcpShimNetworkAdded     bool
	)

	const (
		resName     = "sriovdhcppf0"
		redNetName  = "sriov-dhcp-network-red"
		blueNetName = "sriov-dhcp-network-blue"
		policyName  = "sriov-dhcp-policy-pf0"
	)

	BeforeAll(func() {
		err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
		if err != nil {
			Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
		}

		dhcpShimNetworkAdded = ensureDhcpDaemon()

		By("Validating SR-IOV interfaces")

		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

		Expect(netenv.ValidateSriovInterfaces(APIClient, NetConfig, workerNodeList, 1)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Creating Sriov Policy for PF0")

		_, err = sriov.NewPolicyBuilder(APIClient, policyName, NetConfig.SriovOperatorNamespace,
			resName, 6, []string{sriovInterfacesUnderTest[0]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(1500).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Policy")

		By("Creating Sriov Networks with DHCP IPAM")

		redNet := sriov.NewNetworkBuilder(APIClient, redNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, resName).
			WithOptions(WithVRFDhcpIpam()).
			WithMacAddressSupport().WithLinkState("enable").
			WithOptions(WithVRFMetaPlugin(vrfRedName))

		err = netenv.CreateSriovNetworkAndWaitForNADCreation(APIClient, redNet, netparam.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to create red VRF Sriov Network")

		blueNet := sriov.NewNetworkBuilder(APIClient, blueNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, resName).
			WithOptions(WithVRFDhcpIpam()).
			WithMacAddressSupport().WithLinkState("enable").
			WithOptions(WithVRFMetaPlugin(vrfBlueName))

		err = netenv.CreateSriovNetworkAndWaitForNADCreation(APIClient, blueNet, netparam.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to create blue VRF Sriov Network")

		By("Waiting until cluster MCP and SR-IOV are stable")

		err = sriovoperator.WaitForSriovAndMCPStable(
			APIClient, netparam.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
	})

	AfterAll(func() {
		By("Removing SR-IOV networks and policy")

		err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

		if dhcpShimNetworkAdded {
			removeDhcpShimNetwork()
		}
	})

	AfterEach(func() {
		By("Cleaning up pods from the previous test")

		err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean pods from namespace")
	})

	// 36323
	DescribeTable("Integration: SRIOV, IPAM: dynamic, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		reportxml.ID("36323"),
		func(sameNode bool) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPOverlapWithStaticMac()
			runVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig, true)
		},
		Entry("SameNode IPv4", true,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
	)
})
