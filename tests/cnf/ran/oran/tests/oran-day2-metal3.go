package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	hardwaremanagementv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Metal3 day2 tests run in their own file so they do not inherit the PR-restore AfterEach from the standard
// post-provision tests. Rolling back each change would require firmware changes and a reboot, which is extra time we do
// not need to spend.

// 83883 only requires hardware to be allocated to the primary PR, not a fulfilled install. It runs in a separate
// Describe so it can complete TCFE while the primary PR is still failed/progressing but hardware is claimed.
var _ = Describe("ORAN Metal3 Day2 Hardware Allocation", Label(tsparams.LabelMetal3Day2), func() {
	var o2imsAPIClient runtimeclient.Client

	BeforeEach(func() {
		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")

		By("verifying primary ProvisioningRequest has allocated hardware")

		prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke 1 ProvisioningRequest")

		resourceStatuses := prBuilder.Object.Status.Extensions.InfrastructureResourceStatuses
		expectedHostnames, err := helper.GetSpokeHostnames()
		Expect(err).ToNot(HaveOccurred(), "Failed to resolve expected spoke hostnames")
		Expect(resourceStatuses).To(HaveLen(len(expectedHostnames)),
			"Expected primary ProvisioningRequest to claim all spoke nodes before running 83883")

		for _, resourceStatus := range resourceStatuses {
			Expect(resourceStatus.ResourceProvisioningPhase).
				To(Equal(provisioningv1alpha1.ResourceProvisioningPhaseProvisioned),
					"Expected infrastructure resource %q to be provisioned before running 83883",
					resourceStatus.ResourceId)
		}
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
})

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
		waitForPRFulfilledWithMNOExtras(o2imsAPIClient, prBuilder, updateTime, 120*time.Minute)

		By("verifying BMC firmware has been updated on all nodes")
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
		waitForPRFulfilledWithMNOExtras(o2imsAPIClient, prBuilder, updateTime, 120*time.Minute)

		By("verifying BIOS firmware has been updated on all nodes")
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
		waitForPRFulfilledWithMNOExtras(o2imsAPIClient, prBuilder, updateTime, 120*time.Minute)

		By("verifying BIOS settings have been updated on all nodes")
		verifyBIOSSettingsUpdate(prBuilder)
	})
})

// metal3HostRef associates a ClusterInstance node role with its BareMetalHost key.
type metal3HostRef struct {
	Role string
	Key  runtimeclient.ObjectKey
}

// getMetal3HostRefs returns BareMetalHost keys for every ClusterInstance node with a HostRef.
func getMetal3HostRefs() ([]metal3HostRef, error) {
	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	if err != nil {
		return nil, fmt.Errorf("pull spoke 1 ClusterInstance: %w", err)
	}

	if len(clusterInstance.Definition.Spec.Nodes) == 0 {
		return nil, fmt.Errorf("cluster instance %s has no nodes", RANConfig.Spoke1Name)
	}

	hostRefs := make([]metal3HostRef, 0, len(clusterInstance.Definition.Spec.Nodes))

	for i, node := range clusterInstance.Definition.Spec.Nodes {
		if node.HostRef == nil || node.HostRef.Name == "" || node.HostRef.Namespace == "" {
			return nil, fmt.Errorf("cluster instance %s nodes[%d] has no BareMetalHost hostRef",
				RANConfig.Spoke1Name, i)
		}

		hostRefs = append(hostRefs, metal3HostRef{
			Role: node.Role,
			Key:  runtimeclient.ObjectKey{Name: node.HostRef.Name, Namespace: node.HostRef.Namespace},
		})
	}

	return hostRefs, nil
}

// getHardwareProfilesByRole returns HardwareProfile builders keyed by node group role from the PR ClusterTemplate.
func getHardwareProfilesByRole(prBuilder *oran.ProvisioningRequestBuilder) map[string]*oran.HardwareProfileBuilder {
	By("getting the ClusterTemplate for hardware profile verification")

	clusterTemplateName := fmt.Sprintf("%s.%s",
		prBuilder.Definition.Spec.TemplateName, prBuilder.Definition.Spec.TemplateVersion)
	clusterTemplateNamespace := prBuilder.Definition.Spec.TemplateName + "-" + RANConfig.ClusterTemplateAffix

	clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterTemplate %s", clusterTemplateName)

	nodeGroupData := clusterTemplate.Object.Spec.TemplateDefaults.HwMgmtDefaults.NodeGroupData
	Expect(nodeGroupData).ToNot(BeEmpty(),
		"ClusterTemplate hwMgmtDefaults must include nodeGroupData")

	profilesByRole := make(map[string]*oran.HardwareProfileBuilder, len(nodeGroupData))

	for _, group := range nodeGroupData {
		Expect(group.HwProfile).ToNot(BeEmpty(),
			"ClusterTemplate hwMgmtDefaults nodeGroupData must specify hwProfile")

		role := group.Role
		if role == "" {
			role = group.Name
		}

		By(fmt.Sprintf("getting the HardwareProfile %s for role %s", group.HwProfile, role))

		hwProfileBuilder, err := oran.PullHardwareProfile(HubAPIClient, group.HwProfile, tsparams.O2IMSNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull HardwareProfile %s", group.HwProfile)
		Expect(hwProfileBuilder.Object).ToNot(BeNil(), "HardwareProfile object should not be nil")

		profilesByRole[role] = hwProfileBuilder

		if group.Name != "" && group.Name != role {
			profilesByRole[group.Name] = hwProfileBuilder
		}
	}

	return profilesByRole
}

func hardwareProfileForRole(
	profilesByRole map[string]*oran.HardwareProfileBuilder, role string) *oran.HardwareProfileBuilder {
	if profile, ok := profilesByRole[role]; ok {
		return profile
	}

	if len(profilesByRole) == 1 {
		for _, profile := range profilesByRole {
			return profile
		}
	}

	return nil
}

// verifyFirmwareUpdate verifies firmware on every ClusterInstance BareMetalHost.
func verifyFirmwareUpdate(prBuilder *oran.ProvisioningRequestBuilder, componentType string) {
	profilesByRole := getHardwareProfilesByRole(prBuilder)
	hostRefs, err := getMetal3HostRefs()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Metal3 BareMetalHosts from ClusterInstance")

	By(fmt.Sprintf("verifying HostFirmwareComponents for %s firmware on %d nodes", componentType, len(hostRefs)))

	for _, hostRef := range hostRefs {
		hwProfileBuilder := hardwareProfileForRole(profilesByRole, hostRef.Role)
		Expect(hwProfileBuilder).ToNot(BeNil(),
			"No HardwareProfile found for node role %q on BMH %s/%s",
			hostRef.Role, hostRef.Key.Namespace, hostRef.Key.Name)

		var expectedFirmware hardwaremanagementv1alpha1.Firmware
		if componentType == "bmc" {
			expectedFirmware = hwProfileBuilder.Object.Spec.BmcFirmware
		} else {
			expectedFirmware = hwProfileBuilder.Object.Spec.BiosFirmware
		}

		Expect(expectedFirmware.IsEmpty()).To(BeFalse(),
			"No %s firmware specification in HardwareProfile for role %s", componentType, hostRef.Role)

		hfc, err := bmh.PullHFC(HubAPIClient, hostRef.Key.Name, hostRef.Key.Namespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull HostFirmwareComponents for %s in namespace %s",
			hostRef.Key.Name, hostRef.Key.Namespace)

		componentFound := false

		for _, component := range hfc.Object.Status.Components {
			if component.Component == componentType {
				componentFound = true

				Expect(component.CurrentVersion).To(Equal(expectedFirmware.Version),
					"Expected %s firmware version %s on BMH %s/%s, but found %s",
					componentType, expectedFirmware.Version,
					hostRef.Key.Namespace, hostRef.Key.Name, component.CurrentVersion)

				break
			}
		}

		Expect(componentFound).To(BeTrue(),
			"Component %s not found in HostFirmwareComponents for BMH %s/%s",
			componentType, hostRef.Key.Namespace, hostRef.Key.Name)
	}
}

// verifyBIOSSettingsUpdate verifies BIOS settings on every ClusterInstance BareMetalHost.
func verifyBIOSSettingsUpdate(prBuilder *oran.ProvisioningRequestBuilder) {
	profilesByRole := getHardwareProfilesByRole(prBuilder)
	hostRefs, err := getMetal3HostRefs()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Metal3 BareMetalHosts from ClusterInstance")

	By(fmt.Sprintf("verifying HostFirmwareSettings for BIOS settings on %d nodes", len(hostRefs)))

	for _, hostRef := range hostRefs {
		hwProfileBuilder := hardwareProfileForRole(profilesByRole, hostRef.Role)
		Expect(hwProfileBuilder).ToNot(BeNil(),
			"No HardwareProfile found for node role %q on BMH %s/%s",
			hostRef.Role, hostRef.Key.Namespace, hostRef.Key.Name)

		expectedBIOSSettings := hwProfileBuilder.Object.Spec.Bios.Attributes
		Expect(expectedBIOSSettings).ToNot(BeEmpty(),
			"HardwareProfile must include BIOS attributes for role %s", hostRef.Role)

		hfs, err := bmh.PullHFS(HubAPIClient, hostRef.Key.Name, hostRef.Key.Namespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull HostFirmwareSettings for %s in namespace %s",
			hostRef.Key.Name, hostRef.Key.Namespace)

		for settingName, expectedValue := range expectedBIOSSettings {
			actualValue, exists := hfs.Object.Status.Settings[settingName]
			Expect(exists).To(BeTrue(),
				"BIOS setting %s not found in HostFirmwareSettings for BMH %s/%s",
				settingName, hostRef.Key.Namespace, hostRef.Key.Name)

			expectedValueStr := expectedValue.String()
			Expect(actualValue).To(Equal(expectedValueStr),
				"Expected BIOS setting %s to be %s on BMH %s/%s, but found %s",
				settingName, expectedValueStr, hostRef.Key.Namespace, hostRef.Key.Name, actualValue)
		}
	}
}

// waitForPRFulfilledWithMNOExtras waits for Fulfilled. When worker nodes are present, it also observes
// master-before-worker ordering, worker maxUnavailable concurrency, and BMH OK + Ready gating for ConfigApplied.
func waitForPRFulfilledWithMNOExtras(
	apiClient runtimeclient.Client,
	prBuilder *oran.ProvisioningRequestBuilder,
	updateTime time.Time,
	timeout time.Duration) {
	if !helper.HasWorkerNodes() {
		err := prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, updateTime, timeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to be fulfilled")

		return
	}

	By("waiting for ProvisioningRequest fulfillment while observing MNO Day2 rolling constraints")

	maxUnavailable := getWorkerMaxUnavailable()
	var orderingViolation error
	var concurrencyViolation error
	var gatingViolation error
	maxConcurrentWorkers := 0

	err := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			refreshed, err := oran.PullPR(apiClient, prBuilder.Definition.Name)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to pull ProvisioningRequest while observing: %v", err)

				return false, nil
			}

			prBuilder.Object = refreshed.Object
			prBuilder.Definition = refreshed.Definition

			observeAllocatedNodeRolling(
				maxUnavailable,
				&maxConcurrentWorkers,
				&orderingViolation,
				&concurrencyViolation,
				&gatingViolation,
			)

			phase := refreshed.Object.Status.ProvisioningStatus.ProvisioningPhase
			if phase == provisioningv1alpha1.StateFulfilled {
				return true, nil
			}

			if phase == provisioningv1alpha1.StateFailed {
				return false, fmt.Errorf("ProvisioningRequest failed during Day2 update: %s",
					refreshed.Object.Status.ProvisioningStatus.ProvisioningDetails)
			}

			return false, nil
		})
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to be fulfilled with MNO extras")
	Expect(orderingViolation).ToNot(HaveOccurred(), "Master-before-worker ordering violated during Day2 update")
	Expect(concurrencyViolation).ToNot(HaveOccurred(), "Worker maxUnavailable violated during Day2 update")
	Expect(gatingViolation).ToNot(HaveOccurred(), "ConfigApplied gating violated during Day2 update")
	Expect(maxConcurrentWorkers).To(BeNumerically("<=", maxUnavailable),
		"Observed %d concurrent worker updates, maxUnavailable=%d", maxConcurrentWorkers, maxUnavailable)
}

func getWorkerMaxUnavailable() int {
	if Spoke1APIClient == nil {
		return 1
	}

	workerMCP, err := mco.Pull(Spoke1APIClient, "worker")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to pull worker MachineConfigPool: %v; defaulting maxUnavailable to 1", err)

		return 1
	}

	if workerMCP.Object.Spec.MaxUnavailable == nil {
		return 1
	}

	value := workerMCP.Object.Spec.MaxUnavailable.IntValue()
	if value > 0 {
		return value
	}

	if workerCount := helper.CountNodesByRole("worker"); workerCount > 0 {
		pctStr := strings.TrimSuffix(workerMCP.Object.Spec.MaxUnavailable.StrVal, "%")
		pct, err := strconv.Atoi(pctStr)
		if err == nil && pct > 0 {
			maxU := workerCount * pct / 100
			if maxU < 1 {
				return 1
			}

			return maxU
		}
	}

	return 1
}

func observeAllocatedNodeRolling(
	maxUnavailable int,
	maxConcurrentWorkers *int,
	orderingViolation *error,
	concurrencyViolation *error,
	gatingViolation *error,
) {
	allocatedNodes, err := oran.ListAllocatedNodes(HubAPIClient)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to list AllocatedNodes for MNO observation: %v", err)

		return
	}

	spokeReady := map[string]bool{}
	if Spoke1APIClient != nil {
		nodeList, err := nodes.List(Spoke1APIClient)
		if err == nil {
			for _, node := range nodeList {
				ready := false
				for _, condition := range node.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						ready = true

						break
					}
				}
				spokeReady[node.Object.Name] = ready
			}
		}
	}

	allMastersApplied := true
	anyWorkerUpdating := false
	activeWorkers := 0

	for _, allocatedNode := range allocatedNodes {
		if RANConfig.Spoke1Name != "" &&
			!strings.HasPrefix(allocatedNode.Object.Name, RANConfig.Spoke1Name) {
			continue
		}

		role := allocatedNode.Object.Spec.GroupName
		configuredReason := configuredConditionReason(allocatedNode)

		switch role {
		case "master":
			if configuredReason != string(hardwaremanagementv1alpha1.ConfigApplied) &&
				configuredReason != "" {
				allMastersApplied = false
			}
		case "worker":
			if isActiveHardwareUpdate(configuredReason) {
				anyWorkerUpdating = true
				activeWorkers++
			}
		}

		if configuredReason == string(hardwaremanagementv1alpha1.ConfigApplied) {
			if err := verifyConfigAppliedGating(allocatedNode, spokeReady); err != nil && *gatingViolation == nil {
				*gatingViolation = err
			}
		}
	}

	if anyWorkerUpdating && !allMastersApplied && *orderingViolation == nil {
		*orderingViolation = fmt.Errorf(
			"worker entered active Day2 update before all masters reached ConfigurationApplied")
	}

	if activeWorkers > *maxConcurrentWorkers {
		*maxConcurrentWorkers = activeWorkers
	}

	if activeWorkers > maxUnavailable && *concurrencyViolation == nil {
		*concurrencyViolation = fmt.Errorf(
			"observed %d concurrent worker updates, exceeds maxUnavailable %d", activeWorkers, maxUnavailable)
	}
}

func configuredConditionReason(allocatedNode *oran.AllocatedNodeBuilder) string {
	for _, condition := range allocatedNode.Object.Status.Conditions {
		if condition.Type == string(hardwaremanagementv1alpha1.Configured) {
			return condition.Reason
		}
	}

	return ""
}

func isActiveHardwareUpdate(reason string) bool {
	switch reason {
	case string(hardwaremanagementv1alpha1.ConfigUpdatePending),
		string(hardwaremanagementv1alpha1.ConfigUpdate),
		string(hardwaremanagementv1alpha1.InProgress):
		return true
	default:
		return false
	}
}

func verifyConfigAppliedGating(allocatedNode *oran.AllocatedNodeBuilder, spokeReady map[string]bool) error {
	bmhName := allocatedNode.Object.Spec.HwMgrNodeId
	bmhNamespace := allocatedNode.Object.Spec.HwMgrNodeNs

	if bmhName == "" || bmhNamespace == "" {
		return fmt.Errorf("AllocatedNode %s missing BMH reference while ConfigApplied", allocatedNode.Object.Name)
	}

	bareMetalHost, err := bmh.Pull(HubAPIClient, bmhName, bmhNamespace)
	if err != nil {
		return fmt.Errorf("pull BMH %s/%s for ConfigApplied gating: %w", bmhNamespace, bmhName, err)
	}

	if bareMetalHost.Object.Status.OperationalStatus != bmhv1alpha1.OperationalStatusOK {
		return fmt.Errorf("AllocatedNode %s is ConfigApplied but BMH %s/%s operationalStatus=%s",
			allocatedNode.Object.Name, bmhNamespace, bmhName, bareMetalHost.Object.Status.OperationalStatus)
	}

	nodeName := allocatedNode.Object.Status.Hostname
	if nodeName == "" {
		return nil
	}

	if ready, ok := spokeReady[nodeName]; ok && !ready {
		return fmt.Errorf("AllocatedNode %s is ConfigApplied but spoke node %s is not Ready",
			allocatedNode.Object.Name, nodeName)
	}

	return nil
}
