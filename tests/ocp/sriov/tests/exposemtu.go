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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("SRIOV: Expose MTU:", Ordered, Label(tsparams.LabelExposeMTUTestCases), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		err                      error
		sriovInterfacesUnderTest []string
	)

	BeforeAll(func() {
		By("Validating SR-IOV interfaces")

		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
		Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Verifying if expose MTU tests can be executed on given cluster")

		err = sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
		if err != nil {
			Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
		}
	})

	AfterEach(func() {
		By("Removing SR-IOV configuration")

		err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			SriovOcpConfig.WorkerLabelEnvVar,
			SriovOcpConfig.SriovOperatorNamespace,
			tsparams.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

		By("Cleaning test namespace")

		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	It("netdev 1500", reportxml.ID("73786"), func() {
		testExposeMTU(1500, sriovInterfacesUnderTest, "netdevice")
	})

	It("netdev 9000", reportxml.ID("73787"), func() {
		testExposeMTU(9000, sriovInterfacesUnderTest, "netdevice")
	})

	It("vfio 1500", reportxml.ID("73789"), func() {
		testExposeMTU(1500, sriovInterfacesUnderTest, "vfio-pci")
	})

	It("vfio 9000", reportxml.ID("73790"), func() {
		testExposeMTU(9000, sriovInterfacesUnderTest, "vfio-pci")
	})

	It("netdev 2 Policies with different MTU", reportxml.ID("73788"), func() {
		By("Creating 2 SR-IOV policies with 5000 and 9000 MTU for the same interface")

		const (
			sriovAndResourceName5000 = "5000mtu"
			sriovAndResourceName9000 = "9000mtu"
		)

		_, err := sriov.NewPolicyBuilder(
			APIClient,
			sriovAndResourceName5000,
			SriovOcpConfig.SriovOperatorNamespace,
			sriovAndResourceName5000,
			5,
			[]string{fmt.Sprintf("%s#0-1", sriovInterfacesUnderTest[0])}, SriovOcpConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(5000).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy with mtu 5000")

		_, err = sriov.NewPolicyBuilder(
			APIClient,
			sriovAndResourceName9000,
			SriovOcpConfig.SriovOperatorNamespace,
			sriovAndResourceName9000,
			5,
			[]string{fmt.Sprintf("%s#2-3", sriovInterfacesUnderTest[0])}, SriovOcpConfig.WorkerLabelMap).
			WithDevType("netdevice").WithMTU(9000).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy with mtu 9000")

		err = sriovenv.WaitForSriovPolicyReady(tsparams.MCOWaitTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the stable cluster")

		By("Creating 2 SR-IOV networks")

		sriovNetworkBuilder5000 := sriov.NewNetworkBuilder(APIClient, sriovAndResourceName5000,
			SriovOcpConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, sriovAndResourceName5000).
			WithStaticIpam().WithMacAddressSupport().
			WithIPAddressSupport().WithLogLevel("debug")
		_, err = sriovNetworkBuilder5000.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Network %s", sriovAndResourceName5000)
		err = sriovenv.WaitForNADCreation(sriovAndResourceName5000, tsparams.TestNamespaceName, tsparams.NADTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait NAD for Sriov Network %s", sriovAndResourceName5000)

		sriovNetworkBuilder9000 := sriov.NewNetworkBuilder(APIClient, sriovAndResourceName9000,
			SriovOcpConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, sriovAndResourceName9000).
			WithStaticIpam().WithMacAddressSupport().
			WithIPAddressSupport().WithLogLevel("debug")
		_, err = sriovNetworkBuilder9000.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Network %s", sriovAndResourceName9000)
		err = sriovenv.WaitForNADCreation(sriovAndResourceName9000, tsparams.TestNamespaceName, tsparams.NADTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait NAD for Sriov Network %s", sriovAndResourceName9000)

		By("Creating 2 pods with different VFs")

		testPod1, err := sriovenv.CreateTestPod(
			"testpod1",
			tsparams.TestNamespaceName,
			sriovAndResourceName5000,
			tsparams.ClientIPv4IPAddress,
			tsparams.TestPodClientMAC)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test pod with MTU 5000")

		testPod2, err := sriovenv.CreateTestPod(
			"testpod2",
			tsparams.TestNamespaceName,
			sriovAndResourceName9000,
			tsparams.ServerIPv4IPAddress,
			tsparams.TestPodServerMAC)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test pod with MTU 9000")

		By("Looking for MTU in the pod annotations")
		Expect(testPod1.Exists()).To(BeTrue(), "testpod1 does not exist")
		Expect(testPod1.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]).
			To(ContainSubstring(fmt.Sprintf("\"mtu\": %d", 5000)),
				fmt.Sprintf("Failed to find expected MTU 5000 in the pod annotation: %v", testPod1.Object.Annotations))

		Expect(testPod2.Exists()).To(BeTrue(), "testpod2 does not exist")
		Expect(testPod2.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]).
			To(ContainSubstring(fmt.Sprintf("\"mtu\": %d", 9000)),
				fmt.Sprintf("Failed to find expected MTU 9000 in the pod annotation: %v", testPod2.Object.Annotations))

		By("Verifying that the MTU is available in /etc/podnetinfo/ inside the test pods")
		mtuCheckInsidePod(testPod1, 5000)
		mtuCheckInsidePod(testPod2, 9000)
	})
})

func testExposeMTU(mtu int, interfacesUnderTest []string, devType string) {
	By("Creating SR-IOV policy")

	const sriovAndResourceNameExposeMTU = "exposemtu"

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		sriovAndResourceNameExposeMTU,
		SriovOcpConfig.SriovOperatorNamespace,
		sriovAndResourceNameExposeMTU,
		5,
		interfacesUnderTest, SriovOcpConfig.WorkerLabelMap).WithDevType(devType).WithMTU(mtu)

	err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
		APIClient,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.SriovOperatorNamespace,
		sriovPolicy,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	sriovNetworkBuilder := sriov.NewNetworkBuilder(APIClient, sriovAndResourceNameExposeMTU,
		SriovOcpConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, sriovAndResourceNameExposeMTU).
		WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")
	_, err = sriovNetworkBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
	err = sriovenv.WaitForNADCreation(sriovAndResourceNameExposeMTU, tsparams.TestNamespaceName, tsparams.NADTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD for SR-IOV network")

	By("Creating test pod")

	testPod, err := sriovenv.CreateTestPod(
		"testpod", tsparams.TestNamespaceName, sriovAndResourceNameExposeMTU,
		tsparams.ClientIPv4IPAddress, tsparams.TestPodClientMAC)
	Expect(err).ToNot(HaveOccurred(), "Failed to create test pod")

	By("Looking for MTU in the pod annotation")
	Expect(testPod.Exists()).To(BeTrue(), "testpod does not exist")
	Expect(testPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]).
		To(ContainSubstring(fmt.Sprintf("\"mtu\": %d", mtu)),
			fmt.Sprintf("Failed to find expected MTU %d in the pod annotation: %v", mtu, testPod.Object.Annotations))

	By("Verifying that the MTU is available in /etc/podnetinfo/ inside the test pod")
	mtuCheckInsidePod(testPod, mtu)
}

func mtuCheckInsidePod(testPod *pod.Builder, mtu int) {
	output, err := testPod.ExecCommand([]string{"bash", "-c", "cat /etc/podnetinfo/annotations"})

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command in the pod: %s", output.String()))
	Expect(output.String()).To(ContainSubstring(fmt.Sprintf("\\\"mtu\\\": %d", mtu)),
		fmt.Sprintf("Failed to find MTU %d in /etc/podnetinfo/ inside the test pod: %s", mtu, output.String()))
}
