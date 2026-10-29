package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	labelResourcePoolName       = "resources.clcm.openshift.io/resourcePoolName"
	labelPrefixResourceSelector = "resourceselector.clcm.openshift.io/"

	// InventoryReclaimPRTimeout is how long to wait for suite ProvisioningRequests to delete.
	InventoryReclaimPRTimeout = 10 * time.Minute
	// InventoryReclaimBMHTimeout is how long to wait for one stuck BMH to delete.
	InventoryReclaimBMHTimeout = 10 * time.Minute
	// InventoryFreeWaitTimeout is how long to wait for free BMHs after reclaim.
	InventoryFreeWaitTimeout  = 10 * time.Minute
	inventoryFreePollInterval = 15 * time.Second
)

// suiteProvisioningRequestNames are ProvisioningRequest names owned by the oran provision/metal3 suites.
var suiteProvisioningRequestNames = []string{
	tsparams.TestPRName,
	tsparams.TestPRName2,
}

// IsFreeInventoryBMH reports whether a BareMetalHost is O-Cloud managed and Available for allocation.
func IsFreeInventoryBMH(host *bmhv1alpha1.BareMetalHost) bool {
	if host == nil {
		return false
	}

	if host.Status.Provisioning.State != bmhv1alpha1.StateAvailable {
		return false
	}

	return isOCloudManagedBMH(host)
}

// reclaimableStuckBMHStates are Metal3 states that indicate leftover allocated/provisioned hardware.
// Transitional inventory states (inspecting, registering, match profile, ready, available) must not
// be deleted — reclaiming those wipes free capacity while BMHs recover after HardwareManager recreate.
var reclaimableStuckBMHStates = map[bmhv1alpha1.ProvisioningState]bool{
	bmhv1alpha1.StatePreparing:             true,
	bmhv1alpha1.StateProvisioning:          true,
	bmhv1alpha1.StateProvisioned:           true,
	bmhv1alpha1.StateExternallyProvisioned: true,
	bmhv1alpha1.StateDeprovisioning:        true,
}

// ShouldReclaimStuckBMH reports whether an O-Cloud BMH should be deleted to recover free inventory.
// Available / inspecting / registering BMHs without an allocated-node label are never reclaimed.
func ShouldReclaimStuckBMH(host *bmhv1alpha1.BareMetalHost) bool {
	if host == nil || !isOCloudManagedBMH(host) {
		return false
	}

	if host.Labels != nil && host.Labels[AllocatedNodeLabel] != "" {
		return true
	}

	return reclaimableStuckBMHStates[host.Status.Provisioning.State]
}

// CountFreeInventoryBMHs returns how many O-Cloud BareMetalHosts are in Available state.
func CountFreeInventoryBMHs(apiClient *clients.Settings) (int, error) {
	hosts, err := ListFreeInventoryBMHs(apiClient)
	if err != nil {
		return 0, err
	}

	return len(hosts), nil
}

// ListFreeInventoryBMHs lists O-Cloud BareMetalHosts that are Available for hardware allocation.
func ListFreeInventoryBMHs(apiClient *clients.Settings) ([]*bmh.BmhBuilder, error) {
	allHosts, err := bmh.ListInAllNamespaces(apiClient)
	if err != nil {
		return nil, fmt.Errorf("list BareMetalHosts: %w", err)
	}

	var freeHosts []*bmh.BmhBuilder

	for _, host := range allHosts {
		if IsFreeInventoryBMH(host.Definition) {
			freeHosts = append(freeHosts, host)
		}
	}

	return freeHosts, nil
}

// RequiredFreeInventoryBMHs returns how many Available O-Cloud BMHs the spoke topology needs.
func RequiredFreeInventoryBMHs() (int, error) {
	hostnames, err := GetSpokeHostnames()
	if err != nil {
		return 0, err
	}

	if len(hostnames) == 0 {
		return 0, fmt.Errorf("ProvisioningRequest template defines no spoke hostnames")
	}

	return len(hostnames), nil
}

// ActiveSuiteProvisioningRequest reports whether a suite-owned ProvisioningRequest is already in progress
// or fulfilled. In that case inventory reclaim must not run (suggestion 3: do not wipe claimed BMHs).
func ActiveSuiteProvisioningRequest(apiClient runtimeclient.Client) (bool, error) {
	for _, name := range suiteProvisioningRequestNames {
		prBuilder, err := oran.PullPR(apiClient, name)
		if err != nil {
			continue
		}

		phase := prBuilder.Object.Status.ProvisioningStatus.ProvisioningPhase
		if phase == provisioningv1alpha1.StateProgressing || phase == provisioningv1alpha1.StateFulfilled {
			klog.V(tsparams.LogLevel).Infof(
				"Suite ProvisioningRequest %s is %s; skipping inventory reclaim", name, phase)

			return true, nil
		}
	}

	return false, nil
}

// EnsureFreeInventoryForProvision checks Available O-Cloud BMH capacity before provision tests.
// When short, it reclaims suite leftovers (failed PRs and stuck BMHs), then waits for capacity.
// It does not reclaim when a suite PR is already progressing or fulfilled.
func EnsureFreeInventoryForProvision(
	hubAPIClient *clients.Settings, provisioningAPIClient runtimeclient.Client) error {
	required, err := RequiredFreeInventoryBMHs()
	if err != nil {
		return err
	}

	active, err := ActiveSuiteProvisioningRequest(provisioningAPIClient)
	if err != nil {
		return fmt.Errorf("check active suite ProvisioningRequest: %w", err)
	}

	if active {
		return nil
	}

	free, err := CountFreeInventoryBMHs(hubAPIClient)
	if err != nil {
		return err
	}

	klog.V(tsparams.LogLevel).Infof(
		"Provision inventory preflight: freenodes=%d, required=%d", free, required)

	if free >= required {
		return nil
	}

	klog.V(tsparams.LogLevel).Infof(
		"Insufficient free inventory; reclaiming suite leftovers (freenodes=%d, required=%d)",
		free, required)

	if err := ReclaimSuiteHardwareLeftovers(hubAPIClient, provisioningAPIClient); err != nil {
		return fmt.Errorf("reclaim suite hardware leftovers: %w", err)
	}

	if err := waitForFreeInventory(hubAPIClient, required, InventoryFreeWaitTimeout); err != nil {
		return err
	}

	return nil
}

// ReclaimSuiteHardwareLeftovers deletes suite-owned ProvisioningRequests and stuck O-Cloud BMHs.
// Available / inspecting / registering BMHs without an allocated-node label are left untouched.
func ReclaimSuiteHardwareLeftovers(
	hubAPIClient *clients.Settings, provisioningAPIClient runtimeclient.Client) error {
	if err := deleteSuiteProvisioningRequests(provisioningAPIClient); err != nil {
		return err
	}

	if err := deleteStuckOCloudBMHs(hubAPIClient); err != nil {
		return err
	}

	return nil
}

func deleteSuiteProvisioningRequests(apiClient runtimeclient.Client) error {
	for _, name := range suiteProvisioningRequestNames {
		prBuilder, err := oran.PullPR(apiClient, name)
		if err != nil {
			continue
		}

		klog.V(tsparams.LogLevel).Infof("Deleting leftover suite ProvisioningRequest %s", name)

		if err := prBuilder.DeleteAndWait(InventoryReclaimPRTimeout); err != nil {
			return fmt.Errorf("delete ProvisioningRequest %s: %w", name, err)
		}
	}

	return nil
}

func deleteStuckOCloudBMHs(apiClient *clients.Settings) error {
	allHosts, err := bmh.ListInAllNamespaces(apiClient)
	if err != nil {
		return fmt.Errorf("list BareMetalHosts for reclaim: %w", err)
	}

	var reclaimed int

	for _, host := range allHosts {
		if !ShouldReclaimStuckBMH(host.Definition) {
			continue
		}

		name := host.Definition.Name
		namespace := host.Definition.Namespace
		state := host.Definition.Status.Provisioning.State

		klog.V(tsparams.LogLevel).Infof(
			"Reclaiming stuck O-Cloud BMH %s/%s (state=%s)", namespace, name, state)

		if _, err := host.DeleteAndWaitUntilDeleted(InventoryReclaimBMHTimeout); err != nil {
			return fmt.Errorf("delete BMH %s/%s: %w", namespace, name, err)
		}

		reclaimed++
	}

	klog.V(tsparams.LogLevel).Infof("Reclaimed %d stuck O-Cloud BareMetalHost(s)", reclaimed)

	return nil
}

func waitForFreeInventory(apiClient *clients.Settings, required int, timeout time.Duration) error {
	var lastFree int

	err := wait.PollUntilContextTimeout(
		context.TODO(), inventoryFreePollInterval, timeout, true, func(ctx context.Context) (bool, error) {
			free, countErr := CountFreeInventoryBMHs(apiClient)
			if countErr != nil {
				return false, countErr
			}

			lastFree = free
			klog.V(tsparams.LogLevel).Infof(
				"Waiting for free inventory: freenodes=%d, required=%d", free, required)

			return free >= required, nil
		})
	if err != nil {
		return fmt.Errorf(
			"lab inventory exhausted after reclaim: freenodes=%d, required=%d: %w",
			lastFree, required, err)
	}

	return nil
}

func isOCloudManagedBMH(host *bmhv1alpha1.BareMetalHost) bool {
	if host == nil || host.Labels == nil {
		return false
	}

	if host.Labels[labelResourcePoolName] != "" {
		return true
	}

	for label := range host.Labels {
		if strings.HasPrefix(label, labelPrefixResourceSelector) {
			return true
		}
	}

	return false
}
