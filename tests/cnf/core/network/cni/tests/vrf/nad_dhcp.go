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

var _ = Describe("CNF VRF", Ordered, Label(tsparams.LabelNadVRFDhcpTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		sriovInterfacesUnderTest []string
		dhcpShimNetworkAdded     bool
	)

	const (
		redNetName  = "test-vrf-dhcp-red"
		blueNetName = "test-vrf-dhcp-blue"
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

		By("Adding NADs")

		createVRFNad(blueNetName, sriovInterfacesUnderTest[0], vrfBlueName, &nad.IPAM{Type: "dhcp"})
		createVRFNad(redNetName, sriovInterfacesUnderTest[0], vrfRedName, &nad.IPAM{Type: "dhcp"})
	})

	AfterAll(func() {
		By("Cleaning up NADs")

		err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean NADs from namespace")

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

	// 36325
	DescribeTable("Integration: NAD, IPAM: dynamic, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		reportxml.ID("36325"),
		func(sameNode bool) {
			clientNetConfig, serverNetConfig := defineClientServerVRFsIPOverlapWithStaticMac()
			runVRFScenario(workerNodeList, sameNode, redNetName, blueNetName, clientNetConfig, serverNetConfig, vrfIPAMDHCP)
		},
		Entry("SameNode IPv4", true,
			reportxml.SetProperty("Node", "SameNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
		Entry("DiffNode IPv4", false,
			reportxml.SetProperty("Node", "DiffNode"),
			reportxml.SetProperty("IPStack", netparam.IPV4Family)),
	)
})
