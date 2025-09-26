package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("Application Namespace SriovNetwork", Ordered, Label(tsparams.LabelSriovNetAppNsTestCases),
	ContinueOnFailure, func() {

		var (
			workerNodeList           []*nodes.Builder
			sriovInterfacesUnderTest []string
			sriovVendorID            string
			tNs1, tNs2               *namespace.Builder
		)

		BeforeAll(func() {
			By("Verifying if Application Namespace SriovNetwork tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support Application Namespace SriovNetwork test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Fetching SR-IOV Device ID for interface under test")
			sriovVendorID = discoverInterfaceUnderTestVendorID(sriovInterfacesUnderTest[0], workerNodeList[0].Definition.Name)
			Expect(sriovVendorID).ToNot(BeEmpty(), "Expected sriovDeviceID not to be empty")

			By("Deploy Test Resources: Two Namespaces")
			tNs1, err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName1).
				WithMultipleLabels(params.PrivilegedNSLabels).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace")
			tNs2, err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName2).
				WithMultipleLabels(params.PrivilegedNSLabels).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace")

			By("Creating SriovNetworkNodePolicy")
			_, err = sriov.NewPolicyBuilder(
				APIClient,
				"policy1",
				NetConfig.SriovOperatorNamespace,
				"resource1",
				6,
				[]string{sriovInterfacesUnderTest[0]}, NetConfig.WorkerLabelMap).
				WithDevType("netdevice").Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

			_, err = sriov.NewPolicyBuilder(
				APIClient, "policy2",
				NetConfig.SriovOperatorNamespace,
				"resource2",
				6,
				[]string{sriovInterfacesUnderTest[1]}, NetConfig.WorkerLabelMap).
				WithDevType("netdevice").Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

			err = netenv.WaitForSriovAndMCPStable(
				APIClient,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration,
				NetConfig.CnfMcpLabel,
				NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the stable cluster")
		})

		AfterEach(func() {
			By("Cleaning test namespace")
			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName1).CleanObjects(
				netparam.DefaultTimeout, sriov.GetSriovNetworksGVR(), nad.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName2).CleanObjects(
				netparam.DefaultTimeout, sriov.GetSriovNetworksGVR(), nad.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		AfterAll(func() {
			By("Removing SR-IOV configuration")
			err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName1).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace 1")
			err = tNs1.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace 1")

			Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName2).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace 2")
			err = tNs2.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace 2")
		})

		It("SriovNetwork defined with 1 resource & two user namespaces without targetNamespace",
			reportxml.ID("83121"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork1)

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork2", tNs2.Object.Name, "", "resource1").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork2)
			})

		It("SriovNetwork defined in user namespace with targetNamespace defined", reportxml.ID("83123"), func() {
			By("Creating SriovNetwork in namespace 1")
			_, err := sriov.NewNetworkBuilder(
				APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").
				WithTargetNamespace(tNs1.Object.Name).
				Create()
			Expect(err).To(HaveOccurred(), "SriovNetwork should not be created")

			By("Creating SriovNetwork in namespace 2")
			_, err = sriov.NewNetworkBuilder(
				APIClient, "sriovnetwork2", tNs2.Object.Name, "", "resource2").
				WithTargetNamespace(tNs2.Object.Name).
				Create()
			Expect(err).To(HaveOccurred(), "SriovNetwork should not be created")
		})

		It("SriovNetwork update - User namespace - update networkNamespace to be same as user namespace",
			reportxml.ID("83125"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork1)

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork2", tNs2.Object.Name, "", "resource2").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork2)

				By("Updating SriovNetwork in namespace 1")
				sriovNetwork1, err = sriov.PullNetwork(APIClient, "sriovnetwork1", tNs1.Object.Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovNetwork")

				sriovNetwork1.Definition.Spec.ResourceName = "resource2"
				sriovNetwork1, err = sriovNetwork1.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SriovNetwork")

				By("Updating SriovNetwork in namespace 2")
				sriovNetwork2, err = sriov.PullNetwork(APIClient, "sriovnetwork2", tNs2.Object.Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovNetwork")

				sriovNetwork2.Definition.Spec.ResourceName = "resource1"
				sriovNetwork2, err = sriovNetwork2.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SriovNetwork")

				validateNADOwnerReference(sriovNetwork1)
				validateNADOwnerReference(sriovNetwork2)
			})

		It("SriovNetwork defined with 2 resources & 2 user namespaces without targetNamespace",
			reportxml.ID("83124"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork1)

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, "sriovnetwork2", tNs2.Object.Name, "", "resource2").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				validateNADOwnerReference(sriovNetwork2)
			})

		It("SriovNetwork Delete in user namespace - NAD deletion", reportxml.ID("83142"), func() {
			By("Creating SriovNetwork in namespace 1")
			sriovNetwork1, err := sriov.NewNetworkBuilder(
				APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

			By("Waiting for NAD creation in namespace 1")
			err = sriovenv.WaitForNADCreation(sriovNetwork1, tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD creation")

			By("Deleting SriovNetwork in namespace 1")
			err = sriovNetwork1.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovNetwork")

			By("Waiting for NAD deletion in namespace 1")
			err = sriovenv.WaitForNADDeletion("sriovnetwork1", tNs1.Object.Name, tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD deletion")
		})
	})

func validateNADOwnerReference(sriovNetwork *sriov.NetworkBuilder) {
	By("Fetching NAD")

	nadBuilder, err := nad.Pull(APIClient, sriovNetwork.Object.Name, sriovenv.TargetNamespaceOf(sriovNetwork))
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch NAD")

	By("Validating NAD owner reference")
	Expect(nadBuilder.Object.OwnerReferences).To(SatisfyAll(
		Not(BeEmpty()),
		HaveLen(1),
		ContainElement(SatisfyAll(
			HaveField("Kind", Equal("SriovNetwork")),
			HaveField("Name", Equal(sriovNetwork.Object.Name)),
			HaveField("UID", Equal(sriovNetwork.Object.UID)),
		)),
	), "NAD owner reference should contain the correct SriovNetwork reference")
}
