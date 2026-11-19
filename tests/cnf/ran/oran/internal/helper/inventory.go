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

	// InventoryDeletePRTimeout is how long to wait for suite ProvisioningRequests to delete.
	InventoryDeletePRTimeout = 10 * time.Minute
	// InventoryFreeWaitTimeout is how long to wait for free BMHs to become Available.
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

// CountFreeInventoryBMHs returns how many O-Cloud BareMetalHosts are in Available state.
func CountFreeInventoryBMHs(apiClient *clients.Settings) (int, error) {
	allBMHs, err := bmh.ListInAllNamespaces(apiClient)
	if err != nil {
		return 0, fmt.Errorf("failed to count free inventory BMHs: %w", err)
	}

	freeCount := 0

	for _, host := range allBMHs {
		if IsFreeInventoryBMH(host.Definition) {
			freeCount++
		}
	}

	return freeCount, nil
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
// or fulfilled. In that case inventory capacity is already claimed and preflight must not fail the suite.
func ActiveSuiteProvisioningRequest(apiClient runtimeclient.Client) (bool, error) {
	for _, name := range suiteProvisioningRequestNames {
		prBuilder, err := oran.PullPR(apiClient, name)
		if err != nil {
			continue
		}

		phase := prBuilder.Object.Status.ProvisioningStatus.ProvisioningPhase
		if phase == provisioningv1alpha1.StateProgressing || phase == provisioningv1alpha1.StateFulfilled {
			klog.V(tsparams.LogLevel).Infof(
				"Suite ProvisioningRequest %s is %s; skipping inventory preflight", name, phase)

			return true, nil
		}
	}

	return false, nil
}

// EnsureFreeInventoryForProvision checks Available O-Cloud BMH capacity before provision tests.
// It fails fast when the lab does not have enough free inventory. BMH lifecycle is owned by
// o-cloud-manager / Argo CD / CI setup — this helper does not delete BareMetalHosts.
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

	// BMHs may still be transitioning to Available after HardwareManager recreate; wait briefly
	// without deleting anything, then fail with a clear lab/CI precondition error.
	klog.V(tsparams.LogLevel).Infof(
		"Insufficient free inventory; waiting up to %s for Available O-Cloud BMHs (freenodes=%d, required=%d)",
		InventoryFreeWaitTimeout, free, required)

	return waitForFreeInventory(hubAPIClient, required, InventoryFreeWaitTimeout)
}

// DeleteSuiteProvisioningRequests deletes suite-owned ProvisioningRequests used by pre-provision /
// metal3 tests. It does not touch BareMetalHosts.
func DeleteSuiteProvisioningRequests(apiClient runtimeclient.Client) error {
	for _, name := range suiteProvisioningRequestNames {
		prBuilder, err := oran.PullPR(apiClient, name)
		if err != nil {
			continue
		}

		klog.V(tsparams.LogLevel).Infof("Deleting leftover suite ProvisioningRequest %s", name)

		if err := prBuilder.DeleteAndWait(InventoryDeletePRTimeout); err != nil {
			return fmt.Errorf("delete ProvisioningRequest %s: %w", name, err)
		}
	}

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
			"lab inventory exhausted: freenodes=%d, required=%d "+
				"(BMH lifecycle is owned by o-cloud-manager/Argo/CI; suite does not reclaim BMHs): %w",
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
