package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ORAN Post-provision Tests", Label(tsparams.LabelPostProvision), func() {
	var (
		prBuilder      *oran.ProvisioningRequestBuilder
		originalPRSpec *provisioningv1alpha1.ProvisioningRequestSpec
		o2imsAPIClient runtimeclient.Client
	)

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")
		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")

		By("saving the original ProvisioningRequest spec")
		prBuilder, err = oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

		copiedSpec := prBuilder.Definition.Spec
		originalPRSpec = &copiedSpec

		By("verifying ProvisioningRequest is fulfilled to start")
		prBuilder, err = prBuilder.WaitUntilFulfilled(2 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to verify spoke 1 ProvisioningRequest is fulfilled")
	})

	AfterEach(func() {
		// If saving the original spec failed, skip restoring it to prevent unnecessary panics.
		if originalPRSpec == nil {
			return
		}

		By("pulling the ProvisioningRequest again to ensure valid builder")
		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull the ProvisioningRequest again")

		restoreTime := getStartTime()

		By("restoring the original ProvisioningRequest spec")
		prBuilder.Definition.Spec = *originalPRSpec
		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to restore spoke 1 ProvisioningRequest")

		By("waiting for ProvisioningRequest to be fulfilled")
		// Since all of the post-provision tests end with the ProvisioningRequest being updated, successful
		// cleanup should always ensure the ProvisioningRequest is fulfilled only after the previous step
		// restores it.
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, restoreTime, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to become fulfilled")

		By("deleting the second test ConfigMap if it exists")
		err = configmap.NewBuilder(Spoke1APIClient, tsparams.TestName2, tsparams.TestName).Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete second test ConfigMap if it exists")

		By("deleting the test label if it exists")
		removeTestLabelIfExists()
	})

	// 77373 - Successful update to ProvisioningRequest clusterInstanceParameters
	It("successfully updates clusterInstanceParameters", reportxml.ID("77373"), func() {
		By("verifying the test label does not already exist")
		verifyTestLabelDoesNotExist()

		By("updating the extraLabels in clusterInstanceParameters")
		templateParameters, err := prBuilder.GetTemplateParameters()
		Expect(err).ToNot(HaveOccurred(), "Failed to get spoke 1 TemplateParameters")
		Expect(tsparams.ClusterInstanceParamsKey).
			To(BeKeyOf(templateParameters), "Spoke 1 TemplateParameters is missing clusterInstanceParameters")

		clusterInstanceParams, ok := templateParameters[tsparams.ClusterInstanceParamsKey].(map[string]any)
		Expect(ok).To(BeTrue(), "Spoke 1 clusterInstanceParameters is not a map[string]any")

		clusterInstanceParams["extraLabels"] = map[string]any{"ManagedCluster": map[string]string{tsparams.TestName: ""}}
		prBuilder = prBuilder.WithTemplateParameters(templateParameters)

		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		waitForLabels()
	})

	// 77374 - Successful update to ProvisioningRequest policyTemplateParameters
	It("successfully updates policyTemplateParameters", reportxml.ID("77374"), func() {
		By("verifying the test ConfigMap exists and has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		By("updating the policyTemplateParameters")
		prBuilder = prBuilder.WithTemplateParameter(tsparams.PolicyTemplateParamsKey, map[string]string{
			tsparams.TestName: tsparams.TestNewValue,
		})

		updateTime := getStartTime()
		prBuilder, err := prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		By("waiting for ProvisioningRequest to be fulfilled again")
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to become fulfilled")

		By("verifying the test ConfigMap has the new value")
		verifyCM(tsparams.TestName, tsparams.TestNewValue)
	})

	// 77375 - Successful update to ClusterInstance defaults ConfigMap
	It("successfully updates ClusterInstance defaults", reportxml.ID("77375"), func() {
		By("verifying the test label does not already exist")
		verifyTestLabelDoesNotExist()

		By("updating the ProvisioningRequest TemplateVersion")
		prBuilder.Definition.Spec.TemplateVersion = RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateUpdateDefaults
		_, err := prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		waitForLabels()
	})

	// 77376 - Successful update of existing PG manifest
	It("successfully updates existing PG manifest", reportxml.ID("77376"), func() {
		By("verifying the test ConfigMap exists and has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		updateTime := getStartTime()

		By("updating the ProvisioningRequest TemplateVersion")
		prBuilder.Definition.Spec.TemplateVersion = RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateUpdateExisting
		prBuilder, err := prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		By("waiting for the ProvisioningRequest to be fulfilled")
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to become fulfilled")

		By("verifying the test ConfigMap has the new value")
		verifyCM(tsparams.TestName, tsparams.TestNewValue)
	})

	// 77377 - Successful addition of new manifest to existing PG
	It("successfully adds new manifest to existing PG", reportxml.ID("77377"), func() {
		By("verifying the test ConfigMap exists and has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		By("verifying the second test ConfigMap does not exist")
		_, err := configmap.Pull(Spoke1APIClient, tsparams.TestName2, tsparams.TestName)
		Expect(err).To(HaveOccurred(), "Second test ConfigMap already exists on spoke 1")

		updateTime := getStartTime()

		By("updating the ProvisioningRequest TemplateVersion")
		prBuilder.Definition.Spec.TemplateVersion = RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateAddNew
		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		By("waiting for the ProvisioningRequest to be fulfilled")
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to become fulfilled")

		By("verifying the test ConfigMap has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		By("verifying the second test ConfigMap exists and has the original value")
		verifyCM(tsparams.TestName2, tsparams.TestOriginalValue)
	})

	// 77378 - Successful update of ClusterTemplate policyTemplateParameters schema
	//
	// This test will update the TemplateVersion and in doing so update the policyTemplateParameters and the
	// policyTemplateDefaults ConfigMap. Though the policyTemplateParameters are not changed directly, the policy
	// ConfigMap gets updated so the changes are can be verified. The second test ConfigMap is also added as part of
	// the change to the policies, using the new key added to the policyTemplateParameters schema.
	It("successfully updates schema of policyTemplateParameters", reportxml.ID("77378"), func() {
		By("verifying the test ConfigMap exists and has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		By("verifying the second test ConfigMap does not exist")
		_, err := configmap.Pull(Spoke1APIClient, tsparams.TestName2, tsparams.TestName)
		Expect(err).To(HaveOccurred(), "Second test ConfigMap already exists on spoke 1")

		updateTime := getStartTime()

		By("updating the ProvisioningRequest TemplateVersion")
		prBuilder.Definition.Spec.TemplateVersion = RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateUpdateSchema
		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		By("waiting for the ProvisioningRequest to be fulfilled")
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to become fulfilled")

		By("verifying the test ConfigMap has the original value")
		verifyCM(tsparams.TestName, tsparams.TestOriginalValue)

		By("verifying the second test ConfigMap has the new value")
		verifyCM(tsparams.TestName2, tsparams.TestNewValue)
	})

	// 77379 - Failed update to ProvisioningRequest and successful rollback
	It("successfully rolls back failed ProvisioningRequest update", reportxml.ID("77379"), func() {
		By("updating the policyTemplateParameters")
		prBuilder = prBuilder.WithTemplateParameter(tsparams.PolicyTemplateParamsKey, map[string]string{
			tsparams.HugePagesSizeKey: "2G",
		})
		_, err := prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ProvisioningRequest")

		By("waiting for policy to go NonCompliant")
		err = helper.WaitForNoncompliantImmutable(HubAPIClient, RANConfig.Spoke1Name, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for a spoke 1 policy to go NonCompliant due to immutable field")

		// The AfterEach block will restore the ProvisioningRequest to its original state, so there is no need to
		// restore it here. If it fails to be restored, the test will fail there.
	})
})

// verifyCM verifies that the test ConfigMap name has value for the test key.
func verifyCM(name, value string) {
	testCM, err := configmap.Pull(Spoke1APIClient, name, tsparams.TestName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test ConfigMap %s from spoke 1", name)
	Expect(tsparams.TestName).
		To(BeKeyOf(testCM.Definition.Data), "Test ConfigMap %s on spoke 1 does not have test key", name)
	Expect(testCM.Definition.Data[tsparams.TestName]).
		To(Equal(value), "Test ConfigMap %s on spoke 1 does not have value %s", name, value)
}

// removeTestLabelIfExists removes the test label from the ManagedCluster if it is present.
func removeTestLabelIfExists() {
	mcl, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ManagedCluster")

	if _, hasLabel := mcl.Definition.Labels[tsparams.TestName]; !hasLabel {
		return
	}

	delete(mcl.Definition.Labels, tsparams.TestName)

	_, err = mcl.Update()
	Expect(err).ToNot(HaveOccurred(), "Failed to update spoke 1 ManagedCluster to remove test label")
}

// verifyTestLabelDoesNotExist asserts that the spoke 1 ManagedCluster does not have the test label.
func verifyTestLabelDoesNotExist() {
	mcl, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ManagedCluster")

	_, hasLabel := mcl.Definition.Labels[tsparams.TestName]
	Expect(hasLabel).To(BeFalse(), "Spoke 1 ManagedCluster has test label when it should not")
}

// waitForLabels waits for the test label to appear on the ClusterInstance then on the ManagedCluster.
func waitForLabels() {
	By("waiting for ClusterInstance to have label")

	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ClusterInstance")

	_, err = clusterInstance.WaitForExtraLabel("ManagedCluster", tsparams.TestName, 2*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for spoke 1 ClusterInstance to have the extraLabel")

	By("waiting for ManagedCluster to have label")

	mcl, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ManagedCluster")

	_, err = mcl.WaitForLabel(tsparams.TestName, 2*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for spoke 1 ManagedCluster to have the label")
}

// getStartTime saves the current time, waits until the next second, then returns the saved time. Since Kubernetes only
// serializes times with second precision, it is impossible to order times within a second. Adding a delay is necessary
// to ensure WaitForPhaseAfter does not produce false negatives when the ProvisioningRequest transitions from Fulfilled
// to Pending to Fulfilled within the same second as the call to time.Now().
func getStartTime() time.Time {
	startTime := time.Now()

	// Truncating then adding is equivalent to rounding up the second.
	time.Sleep(time.Until(startTime.Truncate(time.Second).Add(time.Second)))

	return startTime
}
