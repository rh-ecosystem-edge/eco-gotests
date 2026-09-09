package helper

import (
	"errors"
	"fmt"

	hardwaremanagementv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// AllocatedNodeLabel is the BareMetalHost label that maps a BMH to its AllocatedNode name.
const AllocatedNodeLabel = "clcm.openshift.io/allocated-node"

// VerifyAllocatedHardwareForPR validates NAR completeness, unique AllocatedNodes, BMH allocated-node labels, and that
// each AllocatedNode references the expected Day0 HardwareProfile for its group. Errors are accumulated so every check
// runs.
func VerifyAllocatedHardwareForPR(prBuilder *oran.ProvisioningRequestBuilder) error {
	var accumulatedErrors []error

	resourceStatuses := prBuilder.Object.Status.Extensions.InfrastructureResourceStatuses
	expectedHostnames, err := GetSpokeHostnames()
	if err != nil {
		return fmt.Errorf("resolve expected spoke hostnames: %w", err)
	}

	if len(resourceStatuses) != len(expectedHostnames) {
		accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
			"expected %d infrastructure resource statuses, got %d",
			len(expectedHostnames), len(resourceStatuses)))
	}

	allocatedByID := make(map[string]*oran.AllocatedNodeBuilder, len(resourceStatuses))
	bmhToAllocated := make(map[string]string)
	narName := ""

	for _, resourceStatus := range resourceStatuses {
		allocatedNode, err := oran.PullAllocatedNode(HubAPIClient, resourceStatus.ResourceId, tsparams.O2IMSNamespace)
		if err != nil {
			accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
				"pull AllocatedNode %s: %w", resourceStatus.ResourceId, err))

			continue
		}

		allocatedByID[resourceStatus.ResourceId] = allocatedNode

		if narName == "" {
			narName = allocatedNode.Object.Spec.NodeAllocationRequest
		} else if allocatedNode.Object.Spec.NodeAllocationRequest != narName {
			accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
				"AllocatedNode %s references NAR %q, expected %q",
				resourceStatus.ResourceId, allocatedNode.Object.Spec.NodeAllocationRequest, narName))
		}

		bmhKey := allocatedNode.Object.Spec.HwMgrNodeNs + "/" + allocatedNode.Object.Spec.HwMgrNodeId
		if existing, exists := bmhToAllocated[bmhKey]; exists {
			accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
				"BMH %s claimed by both AllocatedNode %s and %s", bmhKey, existing, resourceStatus.ResourceId))
		} else {
			bmhToAllocated[bmhKey] = resourceStatus.ResourceId
		}
	}

	if narName != "" {
		if err := verifyNARProvisioned(narName, len(expectedHostnames)); err != nil {
			accumulatedErrors = append(accumulatedErrors, err)
		}
	}

	expectedProfiles, profileErr := expectedHwProfilesByGroup(prBuilder)
	if profileErr != nil {
		accumulatedErrors = append(accumulatedErrors, profileErr)
	}

	for resourceID, allocatedNode := range allocatedByID {
		if err := verifyBMHAllocatedNodeLabel(allocatedNode); err != nil {
			accumulatedErrors = append(accumulatedErrors, err)
		}

		if expectedProfiles != nil {
			groupName := allocatedNode.Object.Spec.GroupName
			expectedProfile, ok := expectedProfiles[groupName]

			if !ok && len(expectedProfiles) == 1 {
				for _, profile := range expectedProfiles {
					expectedProfile = profile
					ok = true

					break
				}
			}

			if !ok {
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
					"AllocatedNode %s group %q has no matching HardwareProfile in ClusterTemplate",
					resourceID, groupName))
			} else if allocatedNode.Object.Spec.HwProfile != expectedProfile {
				accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
					"AllocatedNode %s has hwProfile %q, expected %q for group %q",
					resourceID, allocatedNode.Object.Spec.HwProfile, expectedProfile, groupName))
			}
		}
	}

	return joinErrors(accumulatedErrors)
}

func verifyNARProvisioned(narName string, expectedNodeCount int) error {
	nars, err := oran.ListNodeAllocationRequests(HubAPIClient)
	if err != nil {
		return fmt.Errorf("list NodeAllocationRequests: %w", err)
	}

	var nar *oran.NARBuilder

	for _, candidate := range nars {
		if candidate.Object.Name == narName {
			nar = candidate

			break
		}
	}

	if nar == nil {
		return fmt.Errorf("NodeAllocationRequest %s not found", narName)
	}

	if len(nar.Object.Status.Properties.NodeNames) != expectedNodeCount {
		return fmt.Errorf("NAR %s has %d nodeNames, expected %d",
			narName, len(nar.Object.Status.Properties.NodeNames), expectedNodeCount)
	}

	provisioned := false

	for _, condition := range nar.Object.Status.Conditions {
		if condition.Type == string(hardwaremanagementv1alpha1.Provisioned) &&
			condition.Status == metav1.ConditionTrue &&
			condition.Reason == string(hardwaremanagementv1alpha1.Completed) {
			provisioned = true

			break
		}
	}

	if !provisioned {
		return fmt.Errorf("NAR %s Provisioned condition is not Completed", narName)
	}

	return nil
}

func verifyBMHAllocatedNodeLabel(allocatedNode *oran.AllocatedNodeBuilder) error {
	bmhName := allocatedNode.Object.Spec.HwMgrNodeId
	bmhNamespace := allocatedNode.Object.Spec.HwMgrNodeNs

	if bmhName == "" || bmhNamespace == "" {
		return fmt.Errorf("AllocatedNode %s missing HwMgrNodeId/HwMgrNodeNs", allocatedNode.Object.Name)
	}

	bareMetalHost, err := bmh.Pull(HubAPIClient, bmhName, bmhNamespace)
	if err != nil {
		return fmt.Errorf("pull BMH %s/%s for AllocatedNode %s: %w",
			bmhNamespace, bmhName, allocatedNode.Object.Name, err)
	}

	labelValue, ok := bareMetalHost.Object.Labels[AllocatedNodeLabel]
	if !ok {
		return fmt.Errorf("BMH %s/%s missing label %s", bmhNamespace, bmhName, AllocatedNodeLabel)
	}

	if labelValue != allocatedNode.Object.Name {
		return fmt.Errorf("BMH %s/%s label %s=%q, expected %q",
			bmhNamespace, bmhName, AllocatedNodeLabel, labelValue, allocatedNode.Object.Name)
	}

	return nil
}

// expectedHwProfilesByGroup returns a map of nodeGroupData name (and role) to HardwareProfile name from the PR's
// ClusterTemplate.
func expectedHwProfilesByGroup(prBuilder *oran.ProvisioningRequestBuilder) (map[string]string, error) {
	clusterTemplateName := fmt.Sprintf("%s.%s",
		prBuilder.Object.Spec.TemplateName, prBuilder.Object.Spec.TemplateVersion)
	clusterTemplateNamespace := prBuilder.Object.Spec.TemplateName + "-" + RANConfig.ClusterTemplateAffix

	clusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
	if err != nil {
		return nil, fmt.Errorf("pull ClusterTemplate %s/%s: %w", clusterTemplateNamespace, clusterTemplateName, err)
	}

	nodeGroupData := clusterTemplate.Object.Spec.TemplateDefaults.HwMgmtDefaults.NodeGroupData
	if len(nodeGroupData) == 0 {
		klog.V(tsparams.LogLevel).Infof(
			"ClusterTemplate %s has no hwMgmtDefaults.nodeGroupData; skipping Day0 hwProfile checks",
			clusterTemplateName)

		return nil, nil
	}

	profiles := make(map[string]string, len(nodeGroupData)*2)

	for _, group := range nodeGroupData {
		if group.HwProfile == "" {
			continue
		}

		if group.Name != "" {
			profiles[group.Name] = group.HwProfile
		}

		if group.Role != "" {
			profiles[group.Role] = group.HwProfile
		}
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("ClusterTemplate %s hwMgmtDefaults.nodeGroupData has no hwProfile entries",
			clusterTemplateName)
	}

	return profiles, nil
}

func joinErrors(errs []error) error {
	return errors.Join(errs...)
}
