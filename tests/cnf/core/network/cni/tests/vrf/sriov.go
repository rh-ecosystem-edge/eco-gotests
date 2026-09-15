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

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelVrfTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
	)

	const (
		resName     = "sriovpf0"
		redNetName  = "sriov-network-red"
		blueNetName = "sriov-network-blue"
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

		Expect(netenv.ValidateSriovInterfaces(APIClient, NetConfig, workerNodeList, 1)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Creating Sriov Policy for PF0")

		_, err = sriov.NewPolicyBuilder(APIClient, "sriov-policy-pf0", NetConfig.SriovOperatorNamespace,
			resName, 6, []string{sriovInterfacesUnderTest[0]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(1500).Create()

		Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Policy")

		By("Creating Sriov Networks")

		redNet := sriov.NewNetworkBuilder(APIClient, redNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, resName).
			WithStaticIpam().WithLinkState("enable").
			WithOptions(WithVRFMetaPlugin(vrfRedName))

		err = netenv.CreateSriovNetworkAndWaitForNADCreation(APIClient, redNet, netparam.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to create red VRF Sriov Network")

		blueNet := sriov.NewNetworkBuilder(APIClient, blueNetName, NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName, resName).
			WithStaticIpam().WithLinkState("enable").
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

		// WorkerLabelEnvVar is used by convention for MCP stabilisation in AfterAll
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

	// 36303
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		reportxml.ID("36303"),
		func(sameNode bool) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPOverlapConfig(workerNodeList, sameNode)
			runStaticVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig)
		},
		Entry("SameNode IPv4", true,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
	)

	// 36311
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		reportxml.ID("36311"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, ipStack)
			runStaticVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig)
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

	// 36319
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs Different IP networks",
		reportxml.ID("36319"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(false, ipStack)
			runStaticVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig)
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
