package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	hardwaremanagementv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Metal3 day2 tests run in their own file so they do not inherit the PR-restore AfterEach from the standard
// post-provision tests. Rolling back each change would require firmware changes and a reboot, which is extra time we do
// not need to spend.
var _ = Describe("ORAN Metal3 Day2 Tests", Label(tsparams.LabelMetal3Day2), Ordered, ContinueOnFailure, func() {
	var o2imsAPIClient runtimeclient.Client

	BeforeEach(func() {
		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")

		By("verifying ProvisioningRequest is fulfilled to start")

		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

		_, err = prBuilder.WaitUntilFulfilled(3 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to verify spoke 1 ProvisioningRequest is fulfilled")
	})

	// 83883 - Failed provisioning due to all matching hardware already allocated
	It("fails when all matching hardware is already allocated", reportxml.ID("83883"), func() {
		By("creating a second ProvisioningRequest when hardware is already allocated")

		prBuilder2, err := helper.NewSecondaryProvisioningRequest(o2imsAPIClient, tsparams.TemplateHardwareAllocated)
		Expect(err).ToNot(HaveOccurred(), "Failed to build second ProvisioningRequest when hardware is allocated")

		prBuilder2, err = prBuilder2.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create second ProvisioningRequest when hardware is allocated")

		DeferCleanup(func() {
			By("cleaning up the second ProvisioningRequest")

			if prBuilder2 != nil {
				err := prBuilder2.DeleteAndWait(10 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete the second ProvisioningRequest")
			}

			By("waiting for the primary ProvisioningRequest to return to fulfilled after second PR cleanup")

			primaryPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull primary ProvisioningRequest after second PR cleanup")
			_, err = primaryPR.WaitUntilFulfilled(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to wait for primary ProvisioningRequest to return to fulfilled after second PR cleanup")
		})

		By("waiting for second ProvisioningRequest to fail due to allocated hardware")

		err = prBuilder2.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, 5*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for second ProvisioningRequest to fail due to allocated hardware")

		By("verifying failure reason indicates all matching hardware is allocated")

		currentPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName2)
		Expect(err).ToNot(HaveOccurred(), "Failed to get second ProvisioningRequest status")
		Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).
			To(Equal(provisioningv1alpha1.StateFailed))
		Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningDetails).
			To(ContainSubstring(tsparams.PRNoHardwareMatchDetailsSubstring),
				"Expected provisioning details to report no free hardware matching the resource selector")
	})

	// 83877 - Successful day2 upgrade of BMC firmware
	It("successfully upgrades BMC firmware", reportxml.ID("83877"), func() {
		By("pulling the ProvisioningRequest for BMC firmware update")

		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 120*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to be fulfilled after BMC firmware update")

		By("verifying BMC firmware has been updated")
		verifyFirmwareUpdate(prBuilder, "bmc")
	})

	// 83878 - Successful day2 upgrade of BIOS firmware
	It("successfully upgrades BIOS firmware", reportxml.ID("83878"), func() {
		By("pulling the ProvisioningRequest for BIOS firmware update")

		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

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

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 120*time.Minute)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to wait for ProvisioningRequest to be fulfilled after BIOS firmware update")

		By("verifying BIOS firmware has been updated")
		verifyFirmwareUpdate(prBuilder, "bios")
	})

	// 83879 - Successful day2 configuration of BIOS settings
	It("successfully configures BIOS settings", reportxml.ID("83879"), func() {
		By("pulling the ProvisioningRequest for BIOS settings update")

		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

		By("updating the ProvisioningRequest to reference new ClusterTemplate with BIOS settings update")

		updateTime := getStartTime()
		newTemplateVersion := RANConfig.ClusterTemplateAffix + "-" + tsparams.TemplateBIOSSettingsUpdate
		prBuilder.Definition.Spec.TemplateVersion = newTemplateVersion
		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update ProvisioningRequest with new BIOS settings template")

		By("waiting for ProvisioningRequest to be fulfilled after BIOS settings update")

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, 120*time.Minute)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to wait for ProvisioningRequest to be fulfilled after BIOS settings update")

		By("verifying BIOS settings have been updated")
		verifyBIOSSettingsUpdate(prBuilder)
	})
})

// getMetal3HostRef returns the BareMetalHost key from the spoke ClusterInstance node HostRef. It uses Spoke1Name to get
// the ClusterInstance and node. The returned pointer is nil if and only if an error occurs.
func getMetal3HostRef() (*runtimeclient.ObjectKey, error) {
	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	if err != nil {
		return nil, fmt.Errorf("pull spoke 1 ClusterInstance: %w", err)
	}

	if len(clusterInstance.Definition.Spec.Nodes) == 0 {
		return nil, fmt.Errorf("cluster instance %s has no nodes", RANConfig.Spoke1Name)
	}

	node := clusterInstance.Definition.Spec.Nodes[0]
	if node.HostRef == nil || node.HostRef.Name == "" || node.HostRef.Namespace == "" {
		return nil, fmt.Errorf("cluster instance %s node has no BareMetalHost hostRef", RANConfig.Spoke1Name)
	}

	return &runtimeclient.ObjectKey{Name: node.HostRef.Name, Namespace: node.HostRef.Namespace}, nil
}

// getHardwareProfileFromPR returns the HardwareProfileBuilder for the HardwareProfile referenced by the
// ProvisioningRequest's ClusterTemplate hwMgmtDefaults.
func getHardwareProfileFromPR(prBuilder *oran.ProvisioningRequestBuilder) *oran.HardwareProfileBuilder {
	By("getting the ClusterTemplate for hardware profile verification")

	clusterTemplateName := fmt.Sprintf("%s.%s",
		prBuilder.Definition.Spec.TemplateName, prBuilder.Definition.Spec.TemplateVersion)
	clusterTemplateNamespace := prBuilder.Definition.Spec.TemplateName + "-" + RANConfig.ClusterTemplateAffix

	clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate %s", clusterTemplateName)

	nodeGroupData := clusterTemplate.Object.Spec.TemplateDefaults.HwMgmtDefaults.NodeGroupData
	Expect(nodeGroupData).ToNot(BeEmpty(),
		"ClusterTemplate hwMgmtDefaults must include nodeGroupData")

	hwProfileName := nodeGroupData[0].HwProfile
	Expect(hwProfileName).ToNot(BeEmpty(),
		"ClusterTemplate hwMgmtDefaults nodeGroupData must specify hwProfile")

	By(fmt.Sprintf("getting the HardwareProfile %s for verification", hwProfileName))

	hwProfileBuilder, err := oran.PullHardwareProfile(HubAPIClient, hwProfileName, tsparams.O2IMSNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull HardwareProfile %s", hwProfileName)
	Expect(hwProfileBuilder.Object).ToNot(BeNil(), "HardwareProfile object should not be nil")

	return hwProfileBuilder
}

// verifyFirmwareUpdate verifies that the firmware component (bmc or bios) has been updated according to the
// HardwareProfile specified in the ClusterTemplate referenced by the ProvisioningRequest.
func verifyFirmwareUpdate(prBuilder *oran.ProvisioningRequestBuilder, componentType string) {
	hwProfileBuilder := getHardwareProfileFromPR(prBuilder)

	var expectedFirmware hardwaremanagementv1alpha1.Firmware
	if componentType == "bmc" {
		expectedFirmware = hwProfileBuilder.Object.Spec.BmcFirmware
	} else {
		expectedFirmware = hwProfileBuilder.Object.Spec.BiosFirmware
	}

	Expect(expectedFirmware.IsEmpty()).To(BeFalse(),
		"No %s firmware specification in HardwareProfile", componentType)

	By(fmt.Sprintf("verifying HostFirmwareComponents for %s firmware", componentType))

	hostRef, err := getMetal3HostRef()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Metal3 BareMetalHost from ClusterInstance")

	hfc, err := bmh.PullHFC(HubAPIClient, hostRef.Name, hostRef.Namespace)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to pull HostFirmwareComponents for %s in namespace %s", hostRef.Name, hostRef.Namespace)

	var componentFound bool

	for _, component := range hfc.Object.Status.Components {
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

// verifyBIOSSettingsUpdate verifies that the BIOS settings have been updated according to the HardwareProfile specified
// in the ClusterTemplate referenced by the ProvisioningRequest.
func verifyBIOSSettingsUpdate(prBuilder *oran.ProvisioningRequestBuilder) {
	hwProfileBuilder := getHardwareProfileFromPR(prBuilder)

	expectedBIOSSettings := hwProfileBuilder.Object.Spec.Bios.Attributes
	Expect(expectedBIOSSettings).ToNot(BeEmpty(),
		"HardwareProfile must include BIOS attributes")

	By("verifying HostFirmwareSettings for BIOS settings")

	hostRef, err := getMetal3HostRef()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Metal3 BareMetalHost from ClusterInstance")

	hfs, err := bmh.PullHFS(HubAPIClient, hostRef.Name, hostRef.Namespace)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to pull HostFirmwareSettings for %s in namespace %s", hostRef.Name, hostRef.Namespace)

	for settingName, expectedValue := range expectedBIOSSettings {
		actualValue, exists := hfs.Object.Status.Settings[settingName]
		Expect(exists).To(BeTrue(), "BIOS setting %s not found in HostFirmwareSettings", settingName)

		// To make the comparison easier, convert IntOrString to string.
		expectedValueStr := expectedValue.String()
		Expect(actualValue).To(Equal(expectedValueStr),
			"Expected BIOS setting %s to be %s, but found %s",
			settingName, expectedValueStr, actualValue)
	}
}
