package tests

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelNadVRFDualTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
	)

	const (
		dualRedNetName  = "test-vrf-dual-red"
		dualBlueNetName = "test-vrf-dual-blue"
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

		By("Adding NADs")

		createVRFNad(dualBlueNetName, sriovInterfacesUnderTest[0], vrfBlueName, nad.IPAMStatic())
		createVRFNad(dualRedNetName, sriovInterfacesUnderTest[1], vrfRedName, nad.IPAMStatic())
	})

	AfterAll(func() {
		By("Cleaning up NADs")

		err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean NADs from namespace")
	})

	AfterEach(func() {
		By("Cleaning up pods from the previous test")

		err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean pods from namespace")
	})

	// 36306
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs network overlap",
		reportxml.ID("36306"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, ipStack)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig,
				vrfIPAMStatic)
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

	// 36314
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs Different IP networks",
		reportxml.ID("36314"),
		func(sameNode bool, ipStack string) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(false, ipStack)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig,
				vrfIPAMStatic)
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

	// 36300
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		reportxml.ID("36300"),
		func(sameNode bool) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPOverlapConfig(workerNodeList, sameNode)
			runVRFScenario(workerNodeList, sameNode, dualRedNetName, dualBlueNetName, clientNetConfig, serverNetConfig,
				vrfIPAMStatic)
		},
		Entry("SameNode IPv4", true,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
	)
})
