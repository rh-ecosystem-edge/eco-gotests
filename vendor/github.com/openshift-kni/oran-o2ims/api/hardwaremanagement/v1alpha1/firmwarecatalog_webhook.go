/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var firmwarecataloglog = logf.Log.WithName("firmwarecatalog-webhook")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *FirmwareCatalog) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// nolint:wrapcheck
	return ctrl.NewWebhookManagedBy(mgr, &FirmwareCatalog{}).
		WithValidator(&firmwareCatalogValidator{Client: mgr.GetClient()}).
		Complete()
}

//+kubebuilder:webhook:path=/validate-clcm-openshift-io-v1alpha1-firmwarecatalog,mutating=false,failurePolicy=fail,sideEffects=None,groups=clcm.openshift.io,resources=firmwarecatalogs,verbs=update,versions=v1alpha1,name=firmwarecatalogs.clcm.openshift.io,admissionReviewVersions=v1

type firmwareCatalogValidator struct {
	client.Client
}

var _ admission.Validator[*FirmwareCatalog] = &firmwareCatalogValidator{}

// ValidateCreate implements admission.Validator
func (v *firmwareCatalogValidator) ValidateCreate(_ context.Context, _ *FirmwareCatalog) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements admission.Validator
func (v *firmwareCatalogValidator) ValidateUpdate(ctx context.Context, oldCatalog, newCatalog *FirmwareCatalog) (admission.Warnings, error) {
	firmwarecataloglog.Info("validate update", "name", oldCatalog.Name)

	if modified := findModifiedImmutableFields(oldCatalog.Spec.Images, newCatalog.Spec.Images); len(modified) > 0 {
		return nil, fmt.Errorf("firmware catalog entries are immutable: %s", strings.Join(modified, "; "))
	}

	removed := findRemovedEntries(oldCatalog.Spec.Images, newCatalog.Spec.Images)
	if len(removed) == 0 {
		return nil, nil
	}

	hwProfiles := &HardwareProfileList{}
	if err := v.Client.List(ctx, hwProfiles, client.InNamespace(oldCatalog.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list HardwareProfiles: %w", err)
	}

	var referenced []string
	for _, name := range removed {
		if isEntryReferencedByAnyProfile(name, hwProfiles.Items) {
			referenced = append(referenced, name)
		}
	}

	if len(referenced) > 0 {
		return nil, fmt.Errorf("cannot remove firmware catalog entries still referenced by HardwareProfiles: %s",
			strings.Join(referenced, ", "))
	}

	return nil, nil
}

// ValidateDelete implements admission.Validator
func (v *firmwareCatalogValidator) ValidateDelete(_ context.Context, _ *FirmwareCatalog) (admission.Warnings, error) {
	return nil, nil
}

// findRemovedEntries returns the names of entries present in old but absent from updated.
func findRemovedEntries(old, updated []FirmwareImage) []string {
	newNames := make(map[string]struct{}, len(updated))
	for _, img := range updated {
		newNames[img.Name] = struct{}{}
	}

	var removed []string
	for _, img := range old {
		if _, exists := newNames[img.Name]; !exists {
			removed = append(removed, img.Name)
		}
	}
	return removed
}

// findModifiedImmutableFields compares entries that exist in both old and updated lists
// and returns descriptions of any immutable field changes (component, url, version, vendor).
func findModifiedImmutableFields(old, updated []FirmwareImage) []string {
	oldByName := make(map[string]FirmwareImage, len(old))
	for _, img := range old {
		oldByName[img.Name] = img
	}

	var violations []string
	for _, cur := range updated {
		prev, exists := oldByName[cur.Name]
		if !exists {
			continue
		}
		if cur.Component != prev.Component {
			violations = append(violations, fmt.Sprintf("%q: component is immutable", cur.Name))
		}
		if cur.URL != prev.URL {
			violations = append(violations, fmt.Sprintf("%q: url is immutable", cur.Name))
		}
		if cur.Version != prev.Version {
			violations = append(violations, fmt.Sprintf("%q: version is immutable", cur.Name))
		}
		if cur.Vendor != prev.Vendor {
			violations = append(violations, fmt.Sprintf("%q: vendor is immutable", cur.Name))
		}
	}
	return violations
}

// isEntryReferencedByAnyProfile checks whether the given catalog entry name is
// referenced by any HardwareProfile's firmware fields. In Phase 1 of the
// FirmwareCatalog rollout, HardwareProfile firmware fields are structs (not
// string references), so this always returns false. When Phase 2 changes those
// fields to string references, this function will need to be updated.
func isEntryReferencedByAnyProfile(_ string, _ []HardwareProfile) bool {
	return false
}
