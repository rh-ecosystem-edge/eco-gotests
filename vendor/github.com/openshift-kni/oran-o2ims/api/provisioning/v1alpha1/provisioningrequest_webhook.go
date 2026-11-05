/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/openshift-kni/oran-o2ims/internal/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clustervalidation "github.com/openshift-kni/oran-o2ims/internal/validation"
)

const HardwareConfigInProgress = "Hardware configuring is in progress"

// log is for logging in this package.
var provisioningrequestlog = logf.Log.WithName("provisioningrequest-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *ProvisioningRequest) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// nolint:wrapcheck
	return ctrl.NewWebhookManagedBy(mgr, &ProvisioningRequest{}).
		WithValidator(&provisioningRequestValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
//+kubebuilder:webhook:path=/validate-clcm-openshift-io-v1alpha1-provisioningrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=clcm.openshift.io,resources=provisioningrequests,verbs=create;update;delete,versions=v1alpha1,name=provisioningrequests.clcm.openshift.io,admissionReviewVersions=v1

// provisioningRequestValidator is a webhook validator for ProvisioningRequest
type provisioningRequestValidator struct {
	client.Client
}

var _ admission.Validator[*ProvisioningRequest] = &provisioningRequestValidator{}

// ValidateCreate implements admission.Validator
func (v *provisioningRequestValidator) ValidateCreate(ctx context.Context, pr *ProvisioningRequest) (admission.Warnings, error) {
	provisioningrequestlog.Info("validate create", "name", pr.Spec.Name)

	// Validate that metadata.name is a valid UUID
	if _, err := uuid.Parse(pr.Name); err != nil {
		return nil, fmt.Errorf("metadata.name must be a valid UUID: %w", err)
	}

	if err := v.validateCreateOrUpdate(ctx, nil, pr); err != nil {
		provisioningrequestlog.Error(err, "failed to validate the ProvisioningRequest")
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements admission.Validator
func (v *provisioningRequestValidator) ValidateUpdate(ctx context.Context, oldPr, newPr *ProvisioningRequest) (admission.Warnings, error) {
	provisioningrequestlog.Info("validate update", "name", oldPr.Name)

	if !newPr.DeletionTimestamp.IsZero() {
		// ProvisioningRequest is being deleted, this update is triggered by finalizer removal
		return nil, nil
	}

	if err := v.validateCreateOrUpdate(ctx, oldPr, newPr); err != nil {
		provisioningrequestlog.Error(err, "failed to validate the ProvisioningRequest")
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements admission.Validator
func (v *provisioningRequestValidator) ValidateDelete(ctx context.Context, pr *ProvisioningRequest) (admission.Warnings, error) {

	// Re-fetch the object to ensure status is available
	fetched := &ProvisioningRequest{}
	key := client.ObjectKey{Name: pr.Name, Namespace: pr.Namespace}
	if err := v.Client.Get(ctx, key, fetched); err != nil {
		return nil, fmt.Errorf("failed to get latest ProvisioningRequest: %w", err)
	}

	provisioningrequestlog.Info("validate delete", "name", fetched.Name)

	if fetched.Status.ProvisioningStatus.ProvisioningDetails == HardwareConfigInProgress &&
		fetched.Status.ProvisioningStatus.ProvisioningPhase == StateProgressing {
		return nil, fmt.Errorf("deleting a ProvisioningRequest is disallowed while post-install hardware configuration is in progress")
	}

	warnings := admission.Warnings{
		"Deleting a ProvisioningRequest triggers cluster deprovisioning and resource cleanup, " +
			"which typically takes several minutes. Please be patient and do not force-delete the resource.",
	}

	return warnings, nil
}

func (v *provisioningRequestValidator) validateCreateOrUpdate(ctx context.Context, oldPr, newPr *ProvisioningRequest) error {
	clusterTemplate, err := newPr.GetClusterTemplateRef(ctx, v.Client)
	if err != nil {
		return err
	}

	if err := newPr.ValidateTemplateInputMatchesSchema(clusterTemplate); err != nil {
		return err
	}

	if err := newPr.ValidateHwMgmtHwProfiles(ctx, v.Client, clusterTemplate); err != nil {
		return err
	}

	if err := newPr.ValidateUpgradeInput(clusterTemplate); err != nil {
		return err
	}

	// We only validate the ClusterInstance input here, not the PolicyTemplate input since
	// its schema is not just for ProvisioningRequest.
	newPrClusterInstanceInput, err := newPr.ValidateClusterInstanceInputMatchesSchema(clusterTemplate)
	if err != nil {
		return err
	}

	// Best-effort clusterName validation on the raw PR input. The controller
	// performs the authoritative check on the merged value after defaults are
	// applied; this catches obvious issues at admission time.
	if inputMap, ok := newPrClusterInstanceInput.(map[string]any); ok {
		if clusterName, ok := inputMap["clusterName"].(string); ok && clusterName != "" {
			if err := clustervalidation.ValidateClusterNameFormat(clusterName); err != nil {
				return fmt.Errorf("clusterInstanceParameters.clusterName: %w", err)
			}
			if err := clustervalidation.ValidateClusterNameNotReserved(clusterName); err != nil {
				return fmt.Errorf("clusterInstanceParameters.clusterName: %w", err)
			}
		}
	}

	if oldPr == nil {
		// ProvisioningRequest is being created, no immutable fields to check
		return nil
	}

	// Check if hardware provisioning has timed out or failed
	// If so, reject any spec updates - user must delete and recreate the PR
	hwProvisionedCond := meta.FindStatusCondition(
		newPr.Status.Conditions, string(PRconditionTypes.HardwareProvisioned))
	if hwProvisionedCond != nil &&
		hwProvisionedCond.Status == "False" &&
		(hwProvisionedCond.Reason == string(CRconditionReasons.TimedOut) ||
			hwProvisionedCond.Reason == string(CRconditionReasons.Failed)) {
		// Compare specs to see if there's an actual spec change
		if !reflect.DeepEqual(oldPr.Spec, newPr.Spec) {
			return fmt.Errorf("hardware provisioning has timed out or failed. " +
				"Spec changes are not allowed. " +
				"Please delete and recreate the ProvisioningRequest to retry")
		}
	}

	crProvisionedCond := meta.FindStatusCondition(
		newPr.Status.Conditions, string(PRconditionTypes.ClusterProvisioned))
	if crProvisionedCond == nil ||
		crProvisionedCond.Reason == string(CRconditionReasons.Unknown) ||
		crProvisionedCond.Reason == string(CRconditionReasons.Failed) {
		return nil
	}

	// Validate updates for ClusterInstance input. Once cluster has started installation,
	// updates are disallowed. After cluster installation is completed, only permissible
	// fields can be updated.
	oldPrClusterInstanceInput, err := ExtractMatchingInput(
		oldPr.Spec.TemplateParameters.Raw, constants.TemplateParamClusterInstance)
	if err != nil {
		return fmt.Errorf(
			"failed to extract matching input for subSchema %s: %w", constants.TemplateParamClusterInstance, err)
	}

	var disallowedFields, scalingNodes []string

	// State-based validation with explicit logic
	if crProvisionedCond.Reason == string(CRconditionReasons.InProgress) {
		// Block all changes during active cluster installation
		// This includes field updates and node scaling to prevent interference with ongoing installation
		disallowedFields, scalingNodes, err = FindClusterInstanceImmutableFieldUpdates(
			oldPrClusterInstanceInput.(map[string]any), newPrClusterInstanceInput.(map[string]any), [][]string{}, [][]string{})
		if err != nil {
			return fmt.Errorf("failed to find immutable field updates for ClusterInstance (%s): %w", newPr.Name, err)
		}

		// Combine field updates and node scaling for rejection
		disallowedFields = append(disallowedFields, scalingNodes...)
		if len(disallowedFields) > 0 {
			return fmt.Errorf("updates to spec.TemplateParameters.ClusterInstanceParameters are "+
				"disallowed during cluster installation, detected changes in fields: %s", strings.Join(disallowedFields, ", "))
		}

	} else if crProvisionedCond.Reason == string(CRconditionReasons.Completed) {
		// Allow specific fields and node scaling after installation completes
		// This enables Day 2 operations like annotation/label updates and cluster scaling
		disallowedFields, scalingNodes, err = FindClusterInstanceImmutableFieldUpdates(
			oldPrClusterInstanceInput.(map[string]any), newPrClusterInstanceInput.(map[string]any),
			[][]string{}, AllowedClusterInstanceFields)
		if err != nil {
			return fmt.Errorf("failed to find immutable field updates for ClusterInstance (%s): %w", newPr.Name, err)
		}

		if len(scalingNodes) > 0 {
			// Reject scaling while an upgrade is active or in a non-healthy state.
			// Check Status != True (rather than Reason == InProgress) to also block
			// scaling during Pending, Unknown, Failed, TimedOut, and
			// PreconditionChecksFailed states when the cluster may not be healthy.
			upgradeCond := meta.FindStatusCondition(
				newPr.Status.Conditions, string(PRconditionTypes.UpgradeCompleted))
			if upgradeCond != nil && upgradeCond.Status != metav1.ConditionTrue {
				return fmt.Errorf("node scaling is not supported while a cluster upgrade is in progress or incomplete")
			}
		}

		// Only reject disallowed field changes; node scaling is explicitly allowed
		if len(disallowedFields) > 0 {
			return fmt.Errorf("only extraLabels/extraAnnotations and node scaling changes in "+
				"spec.TemplateParameters.ClusterInstanceParameters are allowed after cluster "+
				"installation is completed, detected changes in immutable fields: %s",
				strings.Join(disallowedFields, ", "))
		}
	}

	return nil
}
