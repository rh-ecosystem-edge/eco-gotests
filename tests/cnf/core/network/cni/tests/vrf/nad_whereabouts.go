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

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelNadVRFWhereaboutsTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
	)

	const (
		blueNetNameIPv4 = "test-vrf-blue-1"
		redNetNameIPv4  = "test-vrf-red-1"
		blueNetNameIPv6 = "test-vrf-blue-2"
		redNetNameIPv6  = "test-vrf-red-2"
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

		By("Adding NADs")

		createVRFNad(blueNetNameIPv4, sriovInterfacesUnderTest[0], vrfBlueName,
			ipamWhereabouts(fmt.Sprintf("%s-%s/24", vrfBlueClientIPv4, vrfBlueServerIPv4)))
		createVRFNad(redNetNameIPv4, sriovInterfacesUnderTest[0], vrfRedName,
			ipamWhereabouts(fmt.Sprintf("%s-%s/24", vrfRedClientIPv4Overlap, vrfRedServerIPv4Overlap)))
		createVRFNad(blueNetNameIPv6, sriovInterfacesUnderTest[0], vrfBlueName,
			ipamWhereabouts(fmt.Sprintf("%s-%s/64", vrfBlueClientIPv6, vrfBlueServerIPv6)))
		createVRFNad(redNetNameIPv6, sriovInterfacesUnderTest[0], vrfRedName,
			ipamWhereabouts(fmt.Sprintf("%s-%s/64", vrfRedClientIPv6Overlap, vrfRedServerIPv6Overlap)))
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

		By("Waiting for whereabouts IPPools to release allocations")

		waitUntilIPPoolIsEmpty(vrfWhereaboutsIPPoolIPv4)
		waitUntilIPPoolIsEmpty(vrfWhereaboutsIPPoolIPv6)
	})

	// 49731
	DescribeTable("Integration: NAD, IPAM: Whereabouts, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		reportxml.ID("49731"),
		func(sameNode bool, ipStack string) {
			redNetName := redNetNameIPv4
			blueNetName := blueNetNameIPv4

			if ipStack == netparam.IPV6Family {
				redNetName = redNetNameIPv6
				blueNetName = blueNetNameIPv6
			}

			clientNetConfig, serverNetConfig := defineClientServerVRFsIPConfig(true, ipStack)
			runVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig,
				vrfIPAMWhereabouts)
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
