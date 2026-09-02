package inventory

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

const resourceInfoDescriptionAnnotation = "resourceinfo.clcm.openshift.io/description"

const (
	extVendor           = "vendor"
	extModel            = "model"
	extAdminState       = "adminState"
	extOperationalState = "operationalState"
	extUsageState       = "usageState"
	extPowerState       = "powerState"
)

type resourceView struct {
	ResourceID     string
	ResourcePoolID uuid.UUID
	Description    string
	GlobalAssetID  string
	Extensions     map[string]string
}

// VerifyResourceMatchesBMH checks that an API Resource matches the corresponding BareMetalHost.
func VerifyResourceMatchesBMH(
	hubClient *clients.Settings,
	apiResource oranapi.Resource,
	host *bmh.BmhBuilder,
	poolID uuid.UUID,
) error {
	hwBuilder, err := bmh.PullHardwareData(hubClient, host.Definition.Name, host.Definition.Namespace)
	if err != nil {
		return fmt.Errorf("pull hardware data for BMH %s/%s: %w",
			host.Definition.Namespace, host.Definition.Name, err)
	}

	if hwBuilder == nil || hwBuilder.Definition == nil || hwBuilder.Definition.Spec.HardwareDetails == nil {
		return fmt.Errorf("hardware data for BMH %s/%s is incomplete",
			host.Definition.Namespace, host.Definition.Name)
	}

	if err := verifyResourceMatchesBMH(
		apiResource, host.Definition, poolID, hwBuilder.Definition.Spec.HardwareDetails,
	); err != nil {
		return fmt.Errorf("verify inventory resource %s for BMH %s/%s in resource pool %s: %w",
			apiResource.ResourceId, host.Definition.Namespace, host.Definition.Name, poolID, err)
	}

	return nil
}

// verifyResourceMatchesBMH compares a resource projection with BareMetalHost state.
func verifyResourceMatchesBMH(
	apiResource oranapi.Resource,
	host *bmhv1alpha1.BareMetalHost,
	poolID uuid.UUID,
	hardwareDetails *bmhv1alpha1.HardwareDetails,
) error {
	if diff := cmp.Diff(
		buildResourceViewFromHost(host, poolID, hardwareDetails),
		buildResourceViewFromAPI(apiResource),
	); diff != "" {
		return fmt.Errorf("resource mismatch (-want +got):\n%s", diff)
	}

	return nil
}

// buildResourceViewFromHost creates the expected resource projection from a BareMetalHost.
func buildResourceViewFromHost(
	host *bmhv1alpha1.BareMetalHost,
	poolID uuid.UUID,
	hardwareDetails *bmhv1alpha1.HardwareDetails,
) resourceView {
	return resourceView{
		ResourceID:     string(host.UID),
		ResourcePoolID: poolID,
		Description:    host.Annotations[resourceInfoDescriptionAnnotation],
		GlobalAssetID:  hardwareDetails.SystemVendor.SerialNumber,
		Extensions: map[string]string{
			extVendor:           hardwareDetails.SystemVendor.Manufacturer,
			extModel:            hardwareDetails.SystemVendor.ProductName,
			extAdminState:       deriveAdminState(host),
			extOperationalState: deriveOperationalState(host),
			extUsageState:       deriveUsageState(host),
			extPowerState:       derivePowerState(host),
		},
	}
}

// buildResourceViewFromAPI creates a resource projection from an API resource.
func buildResourceViewFromAPI(resource oranapi.Resource) resourceView {
	return resourceView{
		ResourceID:     resource.ResourceId.String(),
		ResourcePoolID: resource.ResourcePoolId,
		Description:    resource.Description,
		GlobalAssetID:  resource.GlobalAssetId,
		Extensions: map[string]string{
			extVendor:           readExtensionString(resource.Extensions, extVendor),
			extModel:            readExtensionString(resource.Extensions, extModel),
			extAdminState:       readExtensionString(resource.Extensions, extAdminState),
			extOperationalState: readExtensionString(resource.Extensions, extOperationalState),
			extUsageState:       readExtensionString(resource.Extensions, extUsageState),
			extPowerState:       readExtensionString(resource.Extensions, extPowerState),
		},
	}
}

// readExtensionString returns a string extension value, or an empty string when unavailable.
func readExtensionString(extensions map[string]any, name string) string {
	value, exists := extensions[name]
	if !exists {
		return ""
	}

	valueString, isString := value.(string)
	if !isString {
		return ""
	}

	return valueString
}

// deriveAdminState derives the inventory admin state from a BareMetalHost.
func deriveAdminState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Spec.Online {
		return "UNLOCKED"
	}

	return "LOCKED"
}

// deriveOperationalState derives the inventory operational state from a BareMetalHost.
func deriveOperationalState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Status.OperationalStatus == bmhv1alpha1.OperationalStatusOK &&
		host.Spec.Online &&
		host.Status.PoweredOn &&
		(host.Status.Provisioning.State == bmhv1alpha1.StateProvisioned ||
			host.Status.Provisioning.State == bmhv1alpha1.StateExternallyProvisioned) {
		return "ENABLED"
	}

	return "DISABLED"
}

// deriveUsageState derives the inventory usage state from a BareMetalHost.
func deriveUsageState(host *bmhv1alpha1.BareMetalHost) string {
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

// derivePowerState derives the inventory power state from a BareMetalHost.
func derivePowerState(host *bmhv1alpha1.BareMetalHost) string {
	if host.Status.PoweredOn {
		return "ON"
	}

	return "OFF"
}
