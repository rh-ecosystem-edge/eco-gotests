package inventory

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
)

// VerifyResourceMatchesBMH checks that an API Resource matches the corresponding BareMetalHost.
func VerifyResourceMatchesBMH(
	hubClient *clients.Settings,
	apiResource oranapi.Resource,
	host *bmh.BmhBuilder,
	poolID uuid.UUID,
) error {
	var errs []error

	hwDataBuilder, hwErr := bmh.PullHardwareData(hubClient, host.Definition.Name, host.Definition.Namespace)

	errs = appendMismatch(errs, "resourceId", string(host.Definition.UID), apiResource.ResourceId.String())
	errs = appendMismatch(errs, "resourcePoolId", poolID, apiResource.ResourcePoolId)

	expectedDescription := ""
	if host.Definition.Annotations != nil {
		expectedDescription = host.Definition.Annotations[tsparams.ResourceInfoDescriptionAnnotation]
	}

	errs = appendMismatch(errs, "description", expectedDescription, apiResource.Description)

	errs = appendError(errs, verifyResourceHardwareFields(apiResource, hwDataBuilder, hwErr))
	errs = appendError(errs, verifyResourceExtensionStates(apiResource, host.Definition))

	return errors.Join(errs...)
}

// verifyResourceHardwareFields checks globalAssetId and vendor/model extensions against HardwareData.
func verifyResourceHardwareFields(
	apiResource oranapi.Resource,
	hwDataBuilder *bmh.HardwareDataBuilder,
	pullErr error,
) error {
	if pullErr != nil {
		return fmt.Errorf("failed to pull HardwareData: %w", pullErr)
	}

	if hwDataBuilder.Definition == nil || hwDataBuilder.Definition.Spec.HardwareDetails == nil {
		return fmt.Errorf("HardwareData has no hardware details")
	}

	hwData := hwDataBuilder.Definition

	var errs []error

	errs = appendMismatch(errs, "globalAssetId",
		hwData.Spec.HardwareDetails.SystemVendor.SerialNumber, apiResource.GlobalAssetId)
	errs = appendMismatch(errs, "extensions.vendor",
		hwData.Spec.HardwareDetails.SystemVendor.Manufacturer, apiResource.Extensions["vendor"])
	errs = appendMismatch(errs, "extensions.model",
		hwData.Spec.HardwareDetails.SystemVendor.ProductName, apiResource.Extensions["model"])

	return errors.Join(errs...)
}

// verifyResourceExtensionStates checks BMH-derived extension state fields on an API Resource.
func verifyResourceExtensionStates(apiResource oranapi.Resource, host *bmhv1alpha1.BareMetalHost) error {
	var errs []error

	errs = appendMismatch(errs, "extensions.adminState",
		expectedAdminState(host), apiResource.Extensions["adminState"])
	errs = appendMismatch(errs, "extensions.operationalState",
		expectedOperationalState(host), apiResource.Extensions["operationalState"])
	errs = appendMismatch(errs, "extensions.usageState",
		expectedUsageState(host), apiResource.Extensions["usageState"])
	errs = appendMismatch(errs, "extensions.powerState",
		expectedPowerState(host), apiResource.Extensions["powerState"])

	return errors.Join(errs...)
}

// expectedAdminState derives the inventory admin state from a BareMetalHost.
func expectedAdminState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Spec.Online {
		return "UNLOCKED"
	}

	return "LOCKED"
}

// expectedOperationalState derives the inventory operational state from a BareMetalHost.
func expectedOperationalState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Status.OperationalStatus == bmhv1alpha1.OperationalStatusOK &&
		host.Spec.Online &&
		host.Status.PoweredOn &&
		(host.Status.Provisioning.State == bmhv1alpha1.StateProvisioned ||
			host.Status.Provisioning.State == bmhv1alpha1.StateExternallyProvisioned) {
		return "ENABLED"
	}

	return "DISABLED"
}

// expectedUsageState derives the inventory usage state from a BareMetalHost.
func expectedUsageState(host *bmhv1alpha1.BareMetalHost) string {
	switch host.Status.Provisioning.State {
	case bmhv1alpha1.StateProvisioned, bmhv1alpha1.StateExternallyProvisioned:
		if host.Status.OperationalStatus == bmhv1alpha1.OperationalStatusOK &&
			host.Spec.Online && host.Status.PoweredOn {
			return "ACTIVE"
		}

		return "BUSY"
	case bmhv1alpha1.StateAvailable, bmhv1alpha1.StateReady:
		if host.Status.OperationalStatus == bmhv1alpha1.OperationalStatusOK {
			return "IDLE"
		}

		return "BUSY"
	case bmhv1alpha1.StateProvisioning,
		bmhv1alpha1.StatePreparing,
		bmhv1alpha1.StateDeprovisioning,
		bmhv1alpha1.StateInspecting,
		bmhv1alpha1.StatePoweringOffBeforeDelete,
		bmhv1alpha1.StateDeleting:
		return "BUSY"
	case bmhv1alpha1.StateNone,
		bmhv1alpha1.StateUnmanaged,
		bmhv1alpha1.StateRegistering,
		bmhv1alpha1.StateMatchProfile:
		return "UNKNOWN"
	}

	return "UNKNOWN"
}

// expectedPowerState derives the inventory power state from a BareMetalHost.
func expectedPowerState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Status.PoweredOn {
		return "ON"
	}

	return "OFF"
}
