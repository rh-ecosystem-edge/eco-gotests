package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ORAN Pre-provision Tests", Label(tsparams.LabelPreProvision), func() {
	var o2imsAPIClient runtimeclient.Client

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")
	})

	// 77392 - Apply a ProvisioningRequest referencing an invalid ClusterTemplate
	It("fails to create ProvisioningRequest with invalid ClusterTemplate", reportxml.ID("77392"), func() {
		By("attempting to create a ProvisioningRequest")

		prBuilder := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateInvalid)
		_, err := prBuilder.Create()
		Expect(err).To(HaveOccurred(), "Creating a ProvisioningRequest with an invalid ClusterTemplate should fail")
	})

	// 78245 - ClusterTemplate validation fails when inline BMC schema is missing without hwMgmtDefaults
	It("fails ClusterTemplate validation when inline BMC schema is missing without hwMgmtDefaults",
		reportxml.ID("78245"), func() {
			clusterTemplateName := fmt.Sprintf("%s.%s-%s",
				tsparams.ClusterTemplateName, RANConfig.ClusterTemplateAffix, tsparams.TemplateInlineBMCMissingSchema)
			clusterTemplateNamespace := tsparams.ClusterTemplateName + "-" + RANConfig.ClusterTemplateAffix

			By("pulling the ClusterTemplate that omits hwMgmtDefaults and inline BMC schema")

			clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate with missing inline BMC schema")

			By("verifying the ClusterTemplate omits hwMgmtDefaults and hwMgmtParameters")
			Expect(clusterTemplate.Definition.Spec.TemplateDefaults.HwMgmtDefaults.NodeGroupData).To(BeEmpty(),
				"ClusterTemplate defines hwMgmtDefaults nodeGroupData when it should not")
			Expect(provisioningv1alpha1.SchemaDefinesHwMgmtParameters(clusterTemplate.Definition)).To(BeFalse(),
				"ClusterTemplate defines hwMgmtParameters in its schema when it should not")

			By("waiting for ClusterTemplate validation to fail due to missing inline BMC fields in the schema")

			_, err = clusterTemplate.WaitForCondition(tsparams.CTInvalidInlineBMCSchemaCondition, time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to verify the ClusterTemplate validation failed due to missing inline BMC schema")
		})

	When("a ProvisioningRequest is created", func() {
		AfterEach(func() {
			By("deleting the ProvisioningRequest if it exists")

			prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
			if err == nil {
				err := prBuilder.DeleteAndWait(10 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete the ProvisioningRequest")
			}
		})

		// 83880 - Failed provisioning due to no hardware matching resource selector
		It("fails when no hardware matches resource selector", reportxml.ID("83880"), func() {
			By("creating a ProvisioningRequest with non-matching resource selector")

			prBuilder := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateNoHardwareMatch)
			_, err := prBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create ProvisioningRequest with non-matching resource selector")

			By("waiting for ProvisioningRequest to fail due to no matching hardware")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to fail due to no matching hardware")

			By("verifying failure reason indicates no suitable hardware found")

			currentPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ProvisioningRequest status")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).
				To(Equal(provisioningv1alpha1.StateFailed))
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningDetails).
				To(ContainSubstring(tsparams.PRNoHardwareMatchDetailsSubstring),
					"Expected provisioning details to report no matching free hardware")
		})

		// 83881 - Failed provisioning due to missing boot interface label
		It("fails when boot interface label is missing", reportxml.ID("83881"), func() {
			By("creating a ProvisioningRequest with missing boot interface label")

			prBuilder := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateMissingBootInterface)
			_, err := prBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create ProvisioningRequest with missing boot interface label")

			By("waiting for ProvisioningRequest to fail due to missing boot interface")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to fail due to missing boot interface")

			By("verifying failure reason indicates missing boot interface label")

			currentPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ProvisioningRequest status")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).
				To(Equal(provisioningv1alpha1.StateFailed))
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningDetails).
				To(ContainSubstring(tsparams.PRMissingBootInterfaceDetailsSubstring),
					"Expected provisioning details to report a missing boot interface MAC assignment")
		})

		// 83882 - Failed provisioning due to nonexistent hardware profile
		It("fails when hardware profile does not exist", reportxml.ID("83882"), func() {
			By("attempting to create a ProvisioningRequest with nonexistent hardware profile")

			prBuilder := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateNonexistentHWProfile)
			_, err := prBuilder.Create()
			Expect(err).To(HaveOccurred(),
				"Creating a ProvisioningRequest with a nonexistent hardware profile should be rejected by the admission webhook")
			Expect(err.Error()).To(ContainSubstring("does not exist"),
				"Admission webhook error should indicate the hardware profile does not exist")
		})
	})
})
