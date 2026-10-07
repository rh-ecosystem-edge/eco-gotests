package ocloudcommon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// VerifySuccessfulMNOZStreamUpgrade verifies an MNO z-stream upgrade on a pre-provisioned cluster.
// CI is expected to own hub/OSUS/spoke/mirror setup and leave exactly one fulfilled ProvisioningRequest
// on the hub. The test updates that PR to the upgrade ClusterTemplate version, waits for
// UpgradeCompleted, and asserts the ManagedCluster openshiftVersion label.
func VerifySuccessfulMNOZStreamUpgrade(ctx SpecContext) {
	Expect(OCloudConfig.TemplateVersionMNOUpgrade).NotTo(BeEmpty(),
		"ECO_OCLOUD_TEMPLATE_VERSION_MNO_UPGRADE must be set")
	Expect(OCloudConfig.MNOUpgradeTargetVersion).NotTo(BeEmpty(),
		"ECO_OCLOUD_MNO_UPGRADE_TARGET_VERSION must be set")

	timeout := OCloudConfig.MNOUpgradeTimeout
	if timeout == 0 {
		timeout = ocloudparams.DefaultMNOUpgradeTimeout
	}

	provisioningRequest := GetSoleProvisioningRequest()
	Expect(provisioningRequest.Object.Status.Extensions.ClusterDetails).NotTo(BeNil(),
		"ProvisioningRequest %s is missing status.extensions.clusterDetails",
		provisioningRequest.Object.Name)

	clusterName := provisioningRequest.Object.Status.Extensions.ClusterDetails.Name
	Expect(clusterName).NotTo(BeEmpty(),
		"ProvisioningRequest %s clusterDetails.name is empty", provisioningRequest.Object.Name)

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)

	updateTime := UpdatePRForMNOUpgrade(provisioningRequest)
	VerifyMNOUpgradeCompleted(ctx, provisioningRequest, updateTime, timeout)
	VerifyManagedClusterOpenShiftVersion(ctx, clusterName, OCloudConfig.MNOUpgradeTargetVersion)
}

// GetSoleProvisioningRequest returns the single non-deleting ProvisioningRequest on the hub.
func GetSoleProvisioningRequest() *oran.ProvisioningRequestBuilder {
	By("Finding the sole ProvisioningRequest on the hub")

	err := provisioningv1alpha1.AddToScheme(HubAPIClient.Scheme())
	Expect(err).NotTo(HaveOccurred(), "Failed to add provisioning scheme to hub client")

	prList := &provisioningv1alpha1.ProvisioningRequestList{}
	err = HubAPIClient.List(context.TODO(), prList)
	Expect(err).NotTo(HaveOccurred(), "Failed to list ProvisioningRequests")

	var names []string

	for _, item := range prList.Items {
		if item.DeletionTimestamp != nil {
			continue
		}

		names = append(names, item.Name)
	}

	Expect(names).To(HaveLen(1),
		fmt.Sprintf("Expected exactly one ProvisioningRequest on the hub, found %d: %v", len(names), names))

	provisioningRequest, err := oran.PullPR(HubAPIClient, names[0])
	Expect(err).NotTo(HaveOccurred(), "Failed to pull ProvisioningRequest %s", names[0])

	klog.V(ocloudparams.OCloudLogLevel).Infof("found sole ProvisioningRequest %s", names[0])

	return provisioningRequest
}

// UpdatePRForMNOUpgrade updates the ProvisioningRequest templateVersion to the configured upgrade
// ClusterTemplate version and optionally sets upgradeParameters.clusterVersion.cvSpec.desiredUpdate.
// It returns the timestamp captured immediately before the update.
func UpdatePRForMNOUpgrade(provisioningRequest *oran.ProvisioningRequestBuilder) time.Time {
	Expect(provisioningRequest).NotTo(BeNil(), "provisioningRequest should not be nil")
	Expect(provisioningRequest.Object).NotTo(BeNil(), "provisioningRequest.Object should not be nil")

	By(fmt.Sprintf("Updating ProvisioningRequest %s to templateVersion %s",
		provisioningRequest.Object.Name, OCloudConfig.TemplateVersionMNOUpgrade))

	provisioningRequest.Definition = provisioningRequest.Object.DeepCopy()
	provisioningRequest.Definition.Spec.TemplateVersion = OCloudConfig.TemplateVersionMNOUpgrade

	if OCloudConfig.MNOUpgradeTargetVersion != "" || OCloudConfig.MNOUpgradeTargetImage != "" {
		desiredUpdate := map[string]any{}

		if OCloudConfig.MNOUpgradeTargetVersion != "" {
			desiredUpdate["version"] = OCloudConfig.MNOUpgradeTargetVersion
		}

		if OCloudConfig.MNOUpgradeTargetImage != "" {
			desiredUpdate["image"] = OCloudConfig.MNOUpgradeTargetImage
		}

		provisioningRequest = provisioningRequest.WithTemplateParameter("upgradeParameters", map[string]any{
			"clusterVersion": map[string]any{
				"cvSpec": map[string]any{
					"desiredUpdate": desiredUpdate,
				},
			},
		})
	}

	updateTime := time.Now()

	_, err := provisioningRequest.Update()
	Expect(err).NotTo(HaveOccurred(),
		"Failed to update ProvisioningRequest %s for MNO upgrade", provisioningRequest.Definition.Name)

	klog.V(ocloudparams.OCloudLogLevel).Infof(
		"updated ProvisioningRequest %s templateVersion to %s",
		provisioningRequest.Definition.Name, OCloudConfig.TemplateVersionMNOUpgrade)

	return updateTime
}

// VerifyMNOUpgradeCompleted waits for UpgradeCompleted=True/Completed after start, then for the
// ProvisioningRequest to return to the fulfilled phase.
func VerifyMNOUpgradeCompleted(
	ctx SpecContext, provisioningRequest *oran.ProvisioningRequestBuilder, start time.Time, timeout time.Duration) {
	Expect(provisioningRequest).NotTo(BeNil(), "provisioningRequest should not be nil")
	Expect(provisioningRequest.Object).NotTo(BeNil(), "provisioningRequest.Object should not be nil")

	By(fmt.Sprintf("Waiting for UpgradeCompleted on ProvisioningRequest %s", provisioningRequest.Object.Name))

	Eventually(func() bool {
		obj, err := provisioningRequest.Get()
		if err != nil {
			return false
		}

		provisioningRequest.Object = obj
		provisioningRequest.Definition = obj

		for _, condition := range obj.Status.Conditions {
			if condition.Type != string(provisioningv1alpha1.PRconditionTypes.UpgradeCompleted) {
				continue
			}

			if condition.Status != metav1.ConditionTrue {
				continue
			}

			if condition.Reason != string(provisioningv1alpha1.CRconditionReasons.Completed) {
				continue
			}

			if !start.IsZero() && !condition.LastTransitionTime.After(start) {
				continue
			}

			return true
		}

		return false
	}).WithTimeout(timeout).WithPolling(30*time.Second).WithContext(ctx).Should(BeTrue(),
		"ProvisioningRequest %s did not reach UpgradeCompleted=True/Completed after %s",
		provisioningRequest.Definition.Name, timeout)

	By(fmt.Sprintf("Waiting for ProvisioningRequest %s to be fulfilled after upgrade",
		provisioningRequest.Definition.Name))

	err := provisioningRequest.WaitForPhaseAfter(provisioningv1alpha1.StateFulfilled, start, timeout)
	Expect(err).NotTo(HaveOccurred(),
		"ProvisioningRequest %s is not fulfilled after upgrade", provisioningRequest.Definition.Name)

	klog.V(ocloudparams.OCloudLogLevel).Infof(
		"ProvisioningRequest %s upgrade completed and is fulfilled", provisioningRequest.Definition.Name)
}

// VerifyManagedClusterOpenShiftVersion asserts the ManagedCluster openshiftVersion label matches
// the expected target version.
func VerifyManagedClusterOpenShiftVersion(ctx SpecContext, clusterName, expectedVersion string) {
	By(fmt.Sprintf("Verifying ManagedCluster %s %s label is %s",
		clusterName, ocloudparams.ManagedClusterOpenShiftVersionLabel, expectedVersion))

	Eventually(func() string {
		managedCluster, err := ocm.PullManagedCluster(HubAPIClient, clusterName)
		if err != nil {
			return ""
		}

		if managedCluster.Object.Labels == nil {
			return ""
		}

		return managedCluster.Object.Labels[ocloudparams.ManagedClusterOpenShiftVersionLabel]
	}).WithTimeout(10*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(Equal(expectedVersion),
		"ManagedCluster %s %s did not reach %s",
		clusterName, ocloudparams.ManagedClusterOpenShiftVersionLabel, expectedVersion)

	klog.V(ocloudparams.OCloudLogLevel).Infof(
		"ManagedCluster %s %s is %s",
		clusterName, ocloudparams.ManagedClusterOpenShiftVersionLabel, expectedVersion)
}
