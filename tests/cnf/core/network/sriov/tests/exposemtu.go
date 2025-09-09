package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("SRIOV: Expose MTU:", Ordered, Label(tsparams.LabelExposeMTUTestCases),
	ContinueOnFailure, func() {
		var (
			workerNodeList           []*nodes.Builder
			err                      error
			sriovInterfacesUnderTest []string
		)
		BeforeAll(func() {
			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")
			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Verifying if expose MTU tests can be executed on given cluster")
			err = netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support expose MTU test cases")
		})

		AfterEach(func() {
			By("Removing SR-IOV configuration")
			err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		It("netdev 1500", reportxml.ID("73786"), func() {
			testExposeMTU(1500, sriovInterfacesUnderTest, "netdevice", workerNodeList[0].Object.Name)
		})

		It("netdev 9000", reportxml.ID("73787"), func() {
			testExposeMTU(9000, sriovInterfacesUnderTest, "netdevice", workerNodeList[0].Object.Name)
		})

		It("vfio 1500", reportxml.ID("73789"), func() {
			testExposeMTU(1500, sriovInterfacesUnderTest, "vfio-pci", workerNodeList[0].Object.Name)
		})

		It("vfio 9000", reportxml.ID("73790"), func() {
			testExposeMTU(9000, sriovInterfacesUnderTest, "vfio-pci", workerNodeList[0].Object.Name)
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
				NetConfig.SriovOperatorNamespace,
				sriovAndResourceName5000,
				5,
				[]string{fmt.Sprintf("%s#0-1", sriovInterfacesUnderTest[0])}, NetConfig.WorkerLabelMap).
				WithDevType("netdevice").WithMTU(5000).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy with mtu 5000")

			_, err = sriov.NewPolicyBuilder(
				APIClient,
				sriovAndResourceName9000,
				NetConfig.SriovOperatorNamespace,
				sriovAndResourceName9000,
				5,
				[]string{fmt.Sprintf("%s#2-3", sriovInterfacesUnderTest[0])}, NetConfig.WorkerLabelMap).
				WithDevType("netdevice").WithMTU(9000).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy with mtu 9000")

			err = netenv.WaitForSriovAndMCPStable(
				APIClient,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration,
				NetConfig.CnfMcpLabel,
				NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the stable cluster")

			By("Creating 2 SR-IOV networks")
			_, err = sriov.NewNetworkBuilder(APIClient, sriovAndResourceName5000, NetConfig.SriovOperatorNamespace,
				tsparams.TestNamespaceName, sriovAndResourceName5000).WithStaticIpam().WithMacAddressSupport().
				WithIPAddressSupport().WithLogLevel(netparam.LogLevelDebug).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for the policy with 5000 MTU")

			_, err = sriov.NewNetworkBuilder(APIClient, sriovAndResourceName9000, NetConfig.SriovOperatorNamespace,
				tsparams.TestNamespaceName, sriovAndResourceName9000).WithStaticIpam().WithMacAddressSupport().
				WithIPAddressSupport().WithLogLevel(netparam.LogLevelDebug).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network for the policy with 9000 MTU")

			By("Creating 2 pods with different VFs")
			testPod1, err := sriovenv.CreateAndWaitTestPodWithSecondaryNetwork(
				"testpod1",
				workerNodeList[0].Object.Name,
				sriovAndResourceName5000,
				"",
				[]string{tsparams.ClientIPv4IPAddress})
			Expect(err).ToNot(HaveOccurred(), "Failed to create test pod with MTU 5000")

			testPod2, err := sriovenv.CreateAndWaitTestPodWithSecondaryNetwork(
				"testpod2",
				workerNodeList[0].Object.Name,
				sriovAndResourceName9000,
				"",
				[]string{tsparams.ServerIPv4IPAddress})
			Expect(err).ToNot(HaveOccurred(), "Failed to create test pod with MTU 9000")

			By("Looking for MTU in the pod annotations")
			testPod1.Exists()
			Expect(testPod1.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]).
				To(ContainSubstring(fmt.Sprintf("\"mtu\": %d", 5000)),
					fmt.Sprintf("Failed to find expected MTU 5000 in the pod annotation: %v", testPod1.Object.Annotations))

			testPod2.Exists()
			Expect(testPod2.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]).
				To(ContainSubstring(fmt.Sprintf("\"mtu\": %d", 9000)),
					fmt.Sprintf("Failed to find expected MTU 9000 in the pod annotation: %v", testPod2.Object.Annotations))

			By("Verifying that the MTU is available in /etc/podnetinfo/ inside the test pods")
			mtuCheckInsidePod(testPod1, 5000)
			mtuCheckInsidePod(testPod2, 9000)
		})
	})

func testExposeMTU(mtu int, interfacesUnderTest []string, devType, workerName string) {
	By("Creating SR-IOV policy")

	const sriovAndResourceNameExposeMTU = "exposemtu"

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		sriovAndResourceNameExposeMTU,
		NetConfig.SriovOperatorNamespace,
		sriovAndResourceNameExposeMTU,
		5,
		interfacesUnderTest, NetConfig.WorkerLabelMap).WithDevType(devType).WithMTU(mtu)

	err := sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(sriovPolicy, tsparams.MCOWaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	_, err = sriov.NewNetworkBuilder(APIClient, sriovAndResourceNameExposeMTU, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, sriovAndResourceNameExposeMTU).WithStaticIpam().WithMacAddressSupport().
		WithIPAddressSupport().WithLogLevel(netparam.LogLevelDebug).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")

	Eventually(func() error {
		_, err := nad.Pull(APIClient, sriovAndResourceNameExposeMTU, tsparams.TestNamespaceName)

		return err
	}, 10*time.Second, 1*time.Second).Should(BeNil(), fmt.Sprintf("Failed to pull NAD %s", sriovAndResourceNameExposeMTU))

	By("Creating test pod")

	testPod, err := sriovenv.CreateAndWaitTestPodWithSecondaryNetwork(
		"testpod", workerName, sriovAndResourceNameExposeMTU, "", []string{tsparams.ClientIPv4IPAddress})
	Expect(err).ToNot(HaveOccurred(), "Failed to create test pod")

	By("Looking for MTU in the pod annotation")
	testPod.Exists()
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
