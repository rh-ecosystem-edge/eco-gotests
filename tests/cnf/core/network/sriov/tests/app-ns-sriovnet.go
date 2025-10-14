package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
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

const (
	ownerRefAnnotationOfNAD     = "sriovnetwork.openshift.io/owner-ref"
	resourceNameAnnotationOfNAD = "k8s.v1.cni.cncf.io/resourceName"
)

var _ = Describe("Application Namespace SriovNetwork:", Ordered, Label(tsparams.LabelSriovNetAppNsTestCases),
	ContinueOnFailure, func() {

		var tNs1, tNs2 *namespace.Builder

		const (
			resourceName1 = "resource1"
			resourceName2 = "resource2"
			networkName1  = "network1"
			networkName2  = "network2"
		)

		BeforeAll(func() {
			By("Verifying if Application Namespace SriovNetwork tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support Application Namespace SriovNetwork test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

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

			By("Deleting test namespace")
			err = tNs1.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace 1")

			err = tNs2.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace 2")
		})

		It("SriovNetwork defined with one resource & two user namespaces without targetNamespace",
			reportxml.ID("83121"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, networkName1, tNs1.Object.Name, "", resourceName1).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1.Object.Name, tNs1.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")
				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, networkName2, tNs2.Object.Name, "", resourceName1).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2.Object.Name, tNs2.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")
				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")
			})

		It("SriovNetwork defined in user namespace with targetNamespace defined", reportxml.ID("83123"), func() {
			By("Creating SriovNetwork in namespace 1")
			_, err := sriov.NewNetworkBuilder(
				APIClient, networkName1, tNs1.Object.Name, "", resourceName1).
				WithTargetNamespace(tNs1.Object.Name).
				Create()
			Expect(err).To(HaveOccurred(), "SriovNetwork creation should fail")

			By("Creating SriovNetwork in namespace 2")
			_, err = sriov.NewNetworkBuilder(
				APIClient, networkName2, tNs2.Object.Name, "", resourceName2).
				WithTargetNamespace(tNs2.Object.Name).
				Create()
			Expect(err).To(HaveOccurred(), "SriovNetwork creation should fail")
		})

		It("SriovNetwork Update - User namespace - Update ResourceName",
			reportxml.ID("83125"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, networkName1, tNs1.Object.Name, "", resourceName1).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1.Object.Name, tNs1.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")

				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, networkName2, tNs2.Object.Name, "", resourceName2).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2.Object.Name, tNs2.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")

				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")

				By("Updating SriovNetwork in namespace 1")
				sriovNetwork1, err = sriov.PullNetwork(APIClient, networkName1, tNs1.Object.Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovNetwork")

				sriovNetwork1.Definition.Spec.ResourceName = resourceName2
				sriovNetwork1, err = sriovNetwork1.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SriovNetwork")

				By("Updating SriovNetwork in namespace 2")
				sriovNetwork2, err = sriov.PullNetwork(APIClient, networkName2, tNs2.Object.Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovNetwork")

				sriovNetwork2.Definition.Spec.ResourceName = resourceName1
				sriovNetwork2, err = sriovNetwork2.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SriovNetwork")

				Eventually(func() error {
					return validateNADOwnerReferenceWithSriovNetwork(sriovNetwork1)
				}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to validate NAD owner reference")

				Eventually(func() error {
					return validateNADAnnotationsWithSriovNetwork(sriovNetwork1)
				}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to validate NAD annotations")

				Eventually(func() error {
					return validateNADOwnerReferenceWithSriovNetwork(sriovNetwork2)
				}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to validate NAD owner reference")

				Eventually(func() error {
					return validateNADAnnotationsWithSriovNetwork(sriovNetwork2)
				}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to validate NAD annotations")
			})

		It("SriovNetwork defined with 2 resources & 2 user namespaces without targetNamespace",
			reportxml.ID("83124"), func() {
				By("Creating SriovNetwork in namespace 1")
				sriovNetwork1, err := sriov.NewNetworkBuilder(
					APIClient, networkName1, tNs1.Object.Name, "", resourceName1).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 1")
				err = sriovenv.WaitForNADCreation(sriovNetwork1.Object.Name, tNs1.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")
				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork1)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")

				By("Creating SriovNetwork in namespace 2")
				sriovNetwork2, err := sriov.NewNetworkBuilder(
					APIClient, networkName2, tNs2.Object.Name, "", resourceName2).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

				By("Waiting for NAD creation in namespace 2")
				err = sriovenv.WaitForNADCreation(sriovNetwork2.Object.Name, tNs2.Object.Name, tsparams.WaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NAD")

				err = validateNADOwnerReferenceWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD owner reference")
				err = validateNADAnnotationsWithSriovNetwork(sriovNetwork2)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate NAD annotations")
			})

		It("SriovNetwork Delete in user namespace - NAD deletion", reportxml.ID("83142"), func() {
			By("Creating SriovNetwork in namespace 1")
			sriovNetwork1, err := sriov.NewNetworkBuilder(
				APIClient, "sriovnetwork1", tNs1.Object.Name, "", "resource1").Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")

			By("Waiting for NAD creation in namespace 1")
			err = sriovenv.WaitForNADCreation(sriovNetwork1.Object.Name, tNs1.Object.Name, tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD creation")

			By("Deleting SriovNetwork in namespace 1")
			err = sriovNetwork1.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovNetwork")

			By("Waiting for NAD deletion in namespace 1")
			err = sriovenv.WaitForNADDeletion("sriovnetwork1", tNs1.Object.Name, tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD deletion")
		})
	})

func validateNADOwnerReferenceWithSriovNetwork(sriovNetwork *sriov.NetworkBuilder) error {
	By("Fetching NAD")

	nadBuilder, err := nad.Pull(APIClient, sriovNetwork.Object.Name, sriovenv.TargetNamespaceOf(sriovNetwork))
	if err != nil {
		return err
	}

	By("Validating NAD owner reference")

	return validateNADOwnerReference(nadBuilder, sriovNetwork)
}

func validateNADAnnotationsWithSriovNetwork(sriovNetwork *sriov.NetworkBuilder) error {
	By("Fetching NAD")

	nadBuilder, err := nad.Pull(APIClient, sriovNetwork.Object.Name, sriovenv.TargetNamespaceOf(sriovNetwork))
	if err != nil {
		return err
	}

	By("Validating NAD annotations")

	return validateNADAnnotations(nadBuilder, sriovNetwork)
}

func validateNADAnnotations(nadBuilder *nad.Builder, sriovNetwork *sriov.NetworkBuilder) error {
	By("Validating NAD annotations")

	if nadBuilder.Object.Annotations == nil {
		return fmt.Errorf("NAD annotations should not be nil")
	}

	if len(nadBuilder.Object.Annotations) == 0 {
		return fmt.Errorf("NAD annotations should not be empty")
	}

	resourceName, exists := nadBuilder.Object.Annotations[resourceNameAnnotationOfNAD]
	if !exists {
		return fmt.Errorf("NAD annotations should have %s", resourceNameAnnotationOfNAD)
	}

	ownerRef, exists := nadBuilder.Object.Annotations[ownerRefAnnotationOfNAD]
	if !exists {
		return fmt.Errorf("NAD annotations should have %s", ownerRefAnnotationOfNAD)
	}

	if resourceName != fmt.Sprintf("openshift.io/%s", sriovNetwork.Object.Spec.ResourceName) {
		return fmt.Errorf("NAD annotations should have the correct resource name, got %s", resourceName)
	}

	if ownerRef != fmt.Sprintf("SriovNetwork.sriovnetwork.openshift.io/%s/%s",
		sriovNetwork.Object.Namespace, sriovNetwork.Object.Name) {
		return fmt.Errorf("NAD annotations should have the correct owner reference, got %s", ownerRef)
	}

	return nil
}

func validateNADOwnerReference(nadBuilder *nad.Builder, sriovNetwork *sriov.NetworkBuilder) error {
	By("Validating NAD owner reference")

	if nadBuilder.Object.OwnerReferences == nil {
		return fmt.Errorf("NAD owner references should not be nil")
	}

	if len(nadBuilder.Object.OwnerReferences) == 0 {
		return fmt.Errorf("NAD owner references should not be empty")
	}

	if nadBuilder.Object.OwnerReferences[0].Kind != "SriovNetwork" {
		return fmt.Errorf("NAD owner references should have the correct SriovNetwork reference")
	}

	if nadBuilder.Object.OwnerReferences[0].Name != sriovNetwork.Object.Name {
		return fmt.Errorf("NAD owner references should have the correct SriovNetwork reference")
	}

	if nadBuilder.Object.OwnerReferences[0].UID != sriovNetwork.Object.UID {
		return fmt.Errorf("NAD owner references should have the correct SriovNetwork reference")
	}

	return nil
}
