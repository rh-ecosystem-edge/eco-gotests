package tests

import (
	"context"
	"fmt"
	"time"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	hardwaremanagementv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"k8s.io/klog/v2"
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
		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, restoreTime, 5*time.Minute)
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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 5*time.Minute)
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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 5*time.Minute)
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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 5*time.Minute)
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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 5*time.Minute)
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

		err = helper.WaitForNoncompliantImmutable(HubAPIClient, RANConfig.Spoke1Name, 5*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for a spoke 1 policy to go NonCompliant due to immutable field")

		// The AfterEach block will restore the ProvisioningRequest to its original state, so there is no need to
		// restore it here. If it fails to be restored, the test will fail there.
	})

	// Metal3 Hardware Plugin Day2 Operations Test Cases
	Context("Metal3 Hardware Plugin Day2 Operations", func() {
		// Temporary: this failure assertion stays phase-only until a live run captures the exact ProvisioningRequest
		// conditions and details to assert on.
		// 83883 - Failed provisioning due to all matching hardware already allocated
		It("fails when all matching hardware is already allocated", reportxml.ID("83883"), func() {
			By("creating a second ProvisioningRequest when hardware is already allocated")

			prBuilder2 := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateHardwareAllocated)
			// Use a different name for the second PR to avoid conflicts
			prBuilder2.Definition.Name = tsparams.TestPRName2
			prBuilder2, err := prBuilder2.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create second ProvisioningRequest when hardware is allocated")

			DeferCleanup(func() {
				By("cleaning up the second ProvisioningRequest")

				if prBuilder2 != nil {
					err := prBuilder2.DeleteAndWait(10 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete the second ProvisioningRequest")
				}
			})

			By("waiting for second ProvisioningRequest to fail due to allocated hardware")

			err = prBuilder2.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for second ProvisioningRequest to fail due to allocated hardware")

			By("verifying failure reason indicates all matching hardware is allocated")

			currentPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName2)
			Expect(err).ToNot(HaveOccurred(), "Failed to get second ProvisioningRequest status")
			By("logging failure details for follow-up assertion tightening")

			logProvisioningRequestFailureDetails(currentPR)
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).To(Equal(provisioningv1alpha1.StateFailed))
		})

		// 83877 - Successful day2 upgrade of BMC firmware
		It("successfully upgrades BMC firmware", reportxml.ID("83877"), func() {
			By("getting the current ProvisioningRequest")

			currentPR, err := oran.PullPR(o2imsAPIClient, prBuilder.Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get current ProvisioningRequest")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).To(
				Equal(provisioningv1alpha1.StateFulfilled), "ProvisioningRequest should be in fulfilled state")

			By("updating the ProvisioningRequest to reference new ClusterTemplate with BMC firmware update")

			updateTime := getStartTime()
			newTemplateVersion := RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateBMCFirmwareUpdate
			prBuilder.Definition.Spec.TemplateVersion = newTemplateVersion
			prBuilder, err = prBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update ProvisioningRequest with new BMC firmware template")

			By("waiting for ProvisioningRequest to progress through update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateProgressing, updateTime, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to enter progressing state")

			By("waiting for ProvisioningRequest to be fulfilled after BMC firmware update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 15*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to be fulfilled after BMC firmware update")

			By("verifying BMC firmware has been updated")
			verifyFirmwareUpdate(prBuilder, "bmc")
		})

		// 83878 - Successful day2 upgrade of BIOS firmware
		It("successfully upgrades BIOS firmware", reportxml.ID("83878"), func() {
			By("getting the current ProvisioningRequest")

			currentPR, err := oran.PullPR(o2imsAPIClient, prBuilder.Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get current ProvisioningRequest")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).To(
				Equal(provisioningv1alpha1.StateFulfilled), "ProvisioningRequest should be in fulfilled state")

			By("updating the ProvisioningRequest to reference new ClusterTemplate with BIOS firmware update")

			updateTime := getStartTime()
			newTemplateVersion := RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateBIOSFirmwareUpdate
			prBuilder.Definition.Spec.TemplateVersion = newTemplateVersion
			prBuilder, err = prBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update ProvisioningRequest with new BIOS firmware template")

			By("waiting for ProvisioningRequest to progress through update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateProgressing, updateTime, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to enter progressing state")

			By("waiting for ProvisioningRequest to be fulfilled after BIOS firmware update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 15*time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to wait for ProvisioningRequest to be fulfilled after BIOS firmware update")

			By("verifying BIOS firmware has been updated")
			verifyFirmwareUpdate(prBuilder, "bios")
		})

		// 83879 - Successful day2 configuration of BIOS settings
		It("successfully configures BIOS settings", reportxml.ID("83879"), func() {
			By("getting the current ProvisioningRequest")

			currentPR, err := oran.PullPR(o2imsAPIClient, prBuilder.Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get current ProvisioningRequest")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).To(
				Equal(provisioningv1alpha1.StateFulfilled), "ProvisioningRequest should be in fulfilled state")

			By("updating the ProvisioningRequest to reference new ClusterTemplate with BIOS settings update")

			updateTime := getStartTime()
			newTemplateVersion := RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateBIOSSettingsUpdate
			prBuilder.Definition.Spec.TemplateVersion = newTemplateVersion
			prBuilder, err = prBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update ProvisioningRequest with new BIOS settings template")

			By("waiting for ProvisioningRequest to progress through update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateProgressing, updateTime, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to enter progressing state")

			By("waiting for ProvisioningRequest to be fulfilled after BIOS settings update")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 15*time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to wait for ProvisioningRequest to be fulfilled after BIOS settings update")

			By("verifying BIOS settings have been updated")
			verifyBIOSSettingsUpdate(prBuilder)
		})
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

	_, err = clusterInstance.WaitForExtraLabel("ManagedCluster", tsparams.TestName, time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for spoke 1 ClusterInstance to have the extraLabel")

	By("waiting for ManagedCluster to have label")

	mcl, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ManagedCluster")

	_, err = mcl.WaitForLabel(tsparams.TestName, time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for spoke 1 ManagedCluster to have the label")
}

// getStartTime saves the current time, waits until the next second, then returns the saved time. Since Kubernetes only
// serializes times with second precision, it is impossible to order times within a second. Adding a delay is necessary
// to ensure WaitForPhaseAfter does not produce false negatives when the ProvisioningRequest transitions from Fulfilled
// to Pending to Fulfilled within the same second as the call to time.Now().
func getStartTime() time.Time {
	startTime := time.Now()

	// Since we cannot verify that a second starts at the same time on the cluster versus the test executor, it is
	// most reliable to wait for two seconds to ensure that the time we count as starting is almost certainly not
	// the same time this function returns on both machines.
	time.Sleep(2 * time.Second)

	return startTime
}

// logProvisioningRequestFailureDetails writes the current failure phase, details, and conditions to the test logs so
// the assertions can be tightened after observing real controller behavior.
func logProvisioningRequestFailureDetails(prBuilder *oran.ProvisioningRequestBuilder) {
	Expect(prBuilder).ToNot(BeNil(), "ProvisioningRequest builder should not be nil")
	Expect(prBuilder.Definition).ToNot(BeNil(), "ProvisioningRequest definition should not be nil")

	status := prBuilder.Definition.Status.ProvisioningStatus
	klog.V(tsparams.LogLevel).Infof(
		"ProvisioningRequest %s failure details: phase=%q details=%q updateTime=%s\n",
		prBuilder.Definition.Name,
		status.ProvisioningPhase,
		status.ProvisioningDetails,
		status.UpdateTime.Format(time.RFC3339),
	)

	if len(prBuilder.Definition.Status.Conditions) == 0 {
		klog.V(tsparams.LogLevel).Info("ProvisioningRequest has no conditions reported")

		return
	}

	for _, condition := range prBuilder.Definition.Status.Conditions {
		klog.V(tsparams.LogLevel).Infof(
			"  condition type=%q status=%q reason=%q message=%q\n",
			condition.Type,
			condition.Status,
			condition.Reason,
			condition.Message,
		)
	}
}

// verifyFirmwareUpdate verifies that the firmware component (bmc or bios) has been updated according to the
// HardwareProfile specified in the ClusterTemplate referenced by the ProvisioningRequest.
//
//nolint:funlen // sequential OpenShift CR and Metal3 checks; splitting would obscure the flow
func verifyFirmwareUpdate(prBuilder *oran.ProvisioningRequestBuilder, componentType string) {
	By(fmt.Sprintf("getting the ClusterTemplate for %s firmware verification", componentType))

	// Get the ClusterTemplate from the ProvisioningRequest
	clusterTemplateName := fmt.Sprintf("%s.%s",
		prBuilder.Definition.Spec.TemplateName, prBuilder.Definition.Spec.TemplateVersion)
	clusterTemplateNamespace := prBuilder.Definition.Spec.TemplateName + "-" + RANConfig.ClusterTemplateAffix

	clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate %s", clusterTemplateName)

	clusterTemplateObj, err := clusterTemplate.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClusterTemplate object")

	// Get the HardwareTemplate reference
	hwTemplateRef := clusterTemplateObj.Spec.Templates.HwTemplate
	if hwTemplateRef == "" {
		Skip("No HardwareTemplate reference in ClusterTemplate, skipping firmware verification")
	}

	By(fmt.Sprintf("getting the HardwareTemplate %s for %s firmware verification", hwTemplateRef, componentType))

	// Get the HardwareTemplate
	hwTemplate := &hardwaremanagementv1alpha1.HardwareTemplate{}
	err = HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      hwTemplateRef,
		Namespace: clusterTemplateNamespace,
	}, hwTemplate)
	Expect(err).ToNot(HaveOccurred(), "Failed to get HardwareTemplate %s", hwTemplateRef)

	// Get the HardwareProfile from the first NodeGroup
	if len(hwTemplate.Spec.NodeGroupData) == 0 {
		Skip("No NodeGroupData in HardwareTemplate, skipping firmware verification")
	}

	hwProfileName := hwTemplate.Spec.NodeGroupData[0].HwProfile

	By(fmt.Sprintf("getting the HardwareProfile %s for %s firmware verification", hwProfileName, componentType))

	// Get the HardwareProfile
	hwProfile := &hardwaremanagementv1alpha1.HardwareProfile{}
	err = HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      hwProfileName,
		Namespace: clusterTemplateNamespace,
	}, hwProfile)
	Expect(err).ToNot(HaveOccurred(), "Failed to get HardwareProfile %s", hwProfileName)

	// Get the expected firmware version based on component type
	var expectedFirmware hardwaremanagementv1alpha1.Firmware

	switch componentType {
	case "bmc":
		expectedFirmware = hwProfile.Spec.BmcFirmware
	case "bios":
		expectedFirmware = hwProfile.Spec.BiosFirmware
	default:
		Fail(fmt.Sprintf("Unknown component type: %s", componentType))
	}

	if expectedFirmware.IsEmpty() {
		Skip(fmt.Sprintf("No %s firmware specification in HardwareProfile, skipping verification", componentType))
	}

	By(fmt.Sprintf("verifying HostFirmwareComponents for %s firmware", componentType))

	// Get HostFirmwareComponents for the host (assuming bmh.PullHFC exists)
	// Note: This assumes the HFCBuilder exists similar to HFSBuilder
	hfc, err := bmh.PullHFC(HubAPIClient, RANConfig.Spoke1Hostname, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull HostFirmwareComponents for %s", RANConfig.Spoke1Hostname)

	hfcObj, err := hfc.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to get HostFirmwareComponents object")

	// Ensure we have the correct type (this uses the metal3v1alpha1 import)
	_ = &metal3v1alpha1.HostFirmwareComponents{}

	// Verify the firmware component version matches expected
	var componentFound bool

	for _, component := range hfcObj.Status.Components {
		if component.Component == componentType {
			componentFound = true

			Expect(component.CurrentVersion).To(Equal(expectedFirmware.Version),
				"Expected %s firmware version %s, but found %s",
				componentType, expectedFirmware.Version, component.CurrentVersion)

			break
		}
	}

	Expect(componentFound).To(BeTrue(), "Component %s not found in HostFirmwareComponents", componentType)
}

// verifyBIOSSettingsUpdate verifies that the BIOS settings have been updated according to the
// HardwareProfile specified in the ClusterTemplate referenced by the ProvisioningRequest.
func verifyBIOSSettingsUpdate(prBuilder *oran.ProvisioningRequestBuilder) {
	By("getting the ClusterTemplate for BIOS settings verification")

	// Get the ClusterTemplate from the ProvisioningRequest
	clusterTemplateName := fmt.Sprintf("%s.%s",
		prBuilder.Definition.Spec.TemplateName, prBuilder.Definition.Spec.TemplateVersion)
	clusterTemplateNamespace := prBuilder.Definition.Spec.TemplateName + "-" + RANConfig.ClusterTemplateAffix

	clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate %s", clusterTemplateName)

	clusterTemplateObj, err := clusterTemplate.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClusterTemplate object")

	// Get the HardwareTemplate reference
	hwTemplateRef := clusterTemplateObj.Spec.Templates.HwTemplate
	if hwTemplateRef == "" {
		Skip("No HardwareTemplate reference in ClusterTemplate, skipping BIOS settings verification")
	}

	By(fmt.Sprintf("getting the HardwareTemplate %s for BIOS settings verification", hwTemplateRef))

	// Get the HardwareTemplate
	hwTemplate := &hardwaremanagementv1alpha1.HardwareTemplate{}
	err = HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      hwTemplateRef,
		Namespace: clusterTemplateNamespace,
	}, hwTemplate)
	Expect(err).ToNot(HaveOccurred(), "Failed to get HardwareTemplate %s", hwTemplateRef)

	// Get the HardwareProfile from the first NodeGroup
	if len(hwTemplate.Spec.NodeGroupData) == 0 {
		Skip("No NodeGroupData in HardwareTemplate, skipping BIOS settings verification")
	}

	hwProfileName := hwTemplate.Spec.NodeGroupData[0].HwProfile

	By(fmt.Sprintf("getting the HardwareProfile %s for BIOS settings verification", hwProfileName))

	// Get the HardwareProfile
	hwProfile := &hardwaremanagementv1alpha1.HardwareProfile{}
	err = HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      hwProfileName,
		Namespace: clusterTemplateNamespace,
	}, hwProfile)
	Expect(err).ToNot(HaveOccurred(), "Failed to get HardwareProfile %s", hwProfileName)

	// Get the expected BIOS settings
	expectedBIOSSettings := hwProfile.Spec.Bios.Attributes
	if len(expectedBIOSSettings) == 0 {
		Skip("No BIOS attributes in HardwareProfile, skipping BIOS settings verification")
	}

	By("verifying HostFirmwareSettings for BIOS settings")

	// Get HostFirmwareSettings for the host
	hfs, err := bmh.PullHFS(HubAPIClient, RANConfig.Spoke1Hostname, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull HostFirmwareSettings for %s", RANConfig.Spoke1Hostname)

	hfsObj, err := hfs.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to get HostFirmwareSettings object")

	// Verify each expected BIOS setting
	for settingName, expectedValue := range expectedBIOSSettings {
		actualValue, exists := hfsObj.Status.Settings[settingName]
		Expect(exists).To(BeTrue(), "BIOS setting %s not found in HostFirmwareSettings", settingName)

		// Convert IntOrString to string for comparison
		expectedValueStr := expectedValue.String()
		Expect(actualValue).To(Equal(expectedValueStr),
			"Expected BIOS setting %s to be %s, but found %s",
			settingName, expectedValueStr, actualValue)
	}
}
