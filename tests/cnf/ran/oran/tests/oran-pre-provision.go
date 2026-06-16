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

		// 78246 - Successful ClusterInstance generation with inline BMC without hwMgmtDefaults
		It("successfully generates ClusterInstance with inline BMC without hwMgmtDefaults", reportxml.ID("78246"), func() {
			clusterTemplateName := fmt.Sprintf("%s.%s-%s",
				tsparams.ClusterTemplateName, RANConfig.ClusterTemplateAffix, tsparams.TemplateInlineBMC)
			clusterTemplateNamespace := tsparams.ClusterTemplateName + "-" + RANConfig.ClusterTemplateAffix

			By("pulling the ClusterTemplate that defines inline BMC schema without hwMgmtDefaults")

			clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate with inline BMC schema")

			By("verifying the ClusterTemplate omits hwMgmtDefaults and hwMgmtParameters")
			Expect(clusterTemplate.Definition.Spec.TemplateDefaults.HwMgmtDefaults.NodeGroupData).To(BeEmpty(),
				"ClusterTemplate defines hwMgmtDefaults nodeGroupData when it should not")
			Expect(provisioningv1alpha1.SchemaDefinesHwMgmtParameters(clusterTemplate.Definition)).To(BeFalse(),
				"ClusterTemplate defines hwMgmtParameters in its schema when it should not")

			By("creating a ProvisioningRequest with inline BMC details in clusterInstanceParameters")

			prBuilder := helper.NewInlineBMCPR(o2imsAPIClient, tsparams.TemplateInlineBMC)
			_, err = prBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create a ProvisioningRequest")

			By("waiting for its ClusterInstance to be created and validated")

			err = helper.WaitForValidPRClusterInstance(HubAPIClient, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to wait for ClusterInstance to be created and have its templates applied")
		})
	})
})
