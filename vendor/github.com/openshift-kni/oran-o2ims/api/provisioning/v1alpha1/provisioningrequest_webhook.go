/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const HardwareConfigInProgress = "Hardware configuring is in progress"

// log is for logging in this package.
var provisioningrequestlog = logf.Log.WithName("provisioningrequest-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *ProvisioningRequest) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ProvisioningRequest{}).
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

var _ webhook.CustomValidator = &provisioningRequestValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *provisioningRequestValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pr, casted := obj.(*ProvisioningRequest)
	if !casted {
		return nil, fmt.Errorf("expected a ProvisioningRequest but got a %T", obj)
	}
	provisioningrequestlog.Info("validate create", "name", pr.Spec.Name)

	// Validate that metadata.name is a valid UUID
	if _, err := uuid.Parse(pr.Name); err != nil {
		return nil, fmt.Errorf("metadata.name must be a valid UUID: %v", err)
	}

	if err := v.validateCreateOrUpdate(ctx, nil, pr); err != nil {
		provisioningrequestlog.Error(err, "failed to validate the ProvisioningRequest")
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *provisioningRequestValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	oldPr, casted := oldObj.(*ProvisioningRequest)
	if !casted {
		return nil, fmt.Errorf("expected a ProvisioningRequest but got a %T", oldObj)
	}
	newPr, casted := newObj.(*ProvisioningRequest)
	if !casted {
		return nil, fmt.Errorf("expected a ProvisioningRequest but got a %T", newObj)
	}
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

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *provisioningRequestValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {

	pr, casted := obj.(*ProvisioningRequest)
	if !casted {
		return nil, fmt.Errorf("expected a ProvisioningRequest but got a %T", obj)
	}

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

	return nil, nil
}

func (v *provisioningRequestValidator) validateCreateOrUpdate(ctx context.Context, oldPr *ProvisioningRequest, newPr *ProvisioningRequest) error {
	clusterTemplate, err := newPr.GetClusterTemplateRef(ctx, v.Client)
	if err != nil {
		return err
	}

	if err = newPr.ValidateTemplateInputMatchesSchema(clusterTemplate); err != nil {
		return err
	}

	// We only validate the ClusterInstance input here, not the PolicyTemplate input since
	// its schema is not just for ProvisioningRequest.
	newPrClusterInstanceInput, err := newPr.ValidateClusterInstanceInputMatchesSchema(clusterTemplate)
	if err != nil {
		return err
	}

	if oldPr == nil {
		// ProvisioningRequest is being created, no immutable fields to check
		return nil
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
		oldPr.Spec.TemplateParameters.Raw, TemplateParamClusterInstance)
	if err != nil {
		return fmt.Errorf(
			"failed to extract matching input for subSchema %s: %w", TemplateParamClusterInstance, err)
	}

	allowedFields := [][]string{}
	if crProvisionedCond.Reason == string(CRconditionReasons.Completed) {
		allowedFields = AllowedClusterInstanceFields
	}
	disallowedFields, scalingNodes, err := FindClusterInstanceImmutableFieldUpdates(
		oldPrClusterInstanceInput.(map[string]any), newPrClusterInstanceInput.(map[string]any), [][]string{}, allowedFields)
	if err != nil {
		return fmt.Errorf("failed to find immutable field updates for ClusterInstance (%s): %w", newPr.Name, err)
	}

	if len(disallowedFields) > 0 && crProvisionedCond.Reason == string(CRconditionReasons.Completed) {
		return fmt.Errorf("only \"%s\" and/or \"%s\" changes in spec.TemplateParameters.ClusterInstanceParameters "+
			"are allowed after cluster installation is completed, detected changes in immutable fields: %s",
			AllowedClusterInstanceFields[0], AllowedClusterInstanceFields[1], strings.Join(disallowedFields, ", "))
	}

	disallowedFields = append(disallowedFields, scalingNodes...)
	if len(disallowedFields) > 0 && crProvisionedCond.Reason == string(CRconditionReasons.InProgress) {
		return fmt.Errorf("updates to spec.TemplateParameters.ClusterInstanceParameters are "+
			"disallowed during cluster installation, detected changes in fields: %s", strings.Join(disallowedFields, ", "))
	}

	return nil
}
