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

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelSriovVRFDualTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
	)

	const (
		dualResNamePF0  = "sriovdualvf0"
		dualResNamePF1  = "sriovdualvf1"
		dualRedNetName  = "sriov-dual-network-red"
		dualBlueNetName = "sriov-dual-network-blue"
	)

	BeforeAll(func() {
		err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
		if err != nil {
			Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
		}

		By("Validating SR-IOV interfaces")

		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

		Expect(netenv.ValidateSriovInterfaces(APIClient, NetConfig, workerNodeList, 2)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Creating SR-IOV Policy for PF0 (red)")

		_, err = sriov.NewPolicyBuilder(APIClient, "sriov-dual-policy-pf0", NetConfig.SriovOperatorNamespace,
			dualResNamePF0, 6, []string{sriovInterfacesUnderTest[0]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(1500).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV Policy for PF0")

		By("Creating SR-IOV Policy for PF1 (blue)")

		_, err = sriov.NewPolicyBuilder(APIClient, "sriov-dual-policy-pf1", NetConfig.SriovOperatorNamespace,
			dualResNamePF1, 6, []string{sriovInterfacesUnderTest[1]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(1500).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV Policy for PF1")

		By("Creating SR-IOV Networks")

		redNet := sriov.NewNetworkBuilder(APIClient, dualRedNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, dualResNamePF0).
			WithStaticIpam().WithLinkState("enable").
			WithOptions(WithVRFMetaPlugin(vrfRedName))

		err = netenv.CreateSriovNetworkAndWaitForNADCreation(APIClient, redNet, netparam.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to create red VRF SR-IOV Network")

		blueNet := sriov.NewNetworkBuilder(APIClient, dualBlueNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, dualResNamePF1).
			WithStaticIpam().WithLinkState("enable").
			WithOptions(WithVRFMetaPlugin(vrfBlueName))

		err = netenv.CreateSriovNetworkAndWaitForNADCreation(APIClient, blueNet, netparam.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to create blue VRF SR-IOV Network")

		By("Waiting until cluster MCP and SR-IOV are stable")

		err = sriovoperator.WaitForSriovAndMCPStable(
			APIClient, netparam.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
	})

	AfterAll(func() {
		By("Removing SR-IOV networks and policies")

		err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
	})

	AfterEach(func() {
		By("Cleaning up pods from the previous test")

		err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean pods from namespace")
	})

	// 36299
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		reportxml.ID("36299"),
		func(sameNode bool) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPOverlapConfig(workerNodeList, sameNode)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig, false)
		},
		Entry("SameNode IPv4", true,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
	)

	// 36308
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 VRFs ip network overlap",
		reportxml.ID("36308"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, ipStack)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig, false)
		},
		Entry("SameNode IPv4", true, netparam.IPV4Family,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false, netparam.IPV4Family,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("SameNode IPv6", true, netparam.IPV6Family,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV6Family)),
		Entry("DiffNode IPv6", false, netparam.IPV6Family,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV6Family)),
	)

	// 36312
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs Different IP networks",
		reportxml.ID("36312"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, ipStack)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig, false)
		},
		Entry("SameNode IPv4", true, netparam.IPV4Family,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false, netparam.IPV4Family,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("SameNode IPv6", true, netparam.IPV6Family,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV6Family)),
		Entry("DiffNode IPv6", false, netparam.IPV6Family,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV6Family)),
	)
})
