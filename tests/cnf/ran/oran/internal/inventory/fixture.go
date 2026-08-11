package inventory

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// CreateReadyTestLocation creates a Location CR and waits until it reports Ready.
func CreateReadyTestLocation(
	hubClient *clients.Settings,
	name, namespace, address string,
	readyCondition metav1.Condition,
	timeout time.Duration,
) (*oran.LocationBuilder, error) {
	location := oran.NewLocationBuilder(hubClient, name, namespace).
		WithDescription(fmt.Sprintf("test location %s", name)).
		WithAddress(address)

	created, err := location.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create Location %s: %w", name, err)
	}

	created, err = created.WaitForCondition(readyCondition, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for Location %s to become Ready: %w", name, err)
	}

	return created, nil
}

// WaitForLocationInAPI polls until a Location with the given globalLocationId appears in the inventory API.
func WaitForLocationInAPI(
	inventoryClient *oranapi.InventoryClient,
	globalLocationID string,
	timeout time.Duration,
) error {
	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			locations, err := inventoryClient.ListLocations()
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list Locations while waiting for %s: %v",
					globalLocationID, err)

				return false, nil
			}

			return slices.IndexFunc(locations, func(loc oranapi.LocationInfo) bool {
				return loc.GlobalLocationId == globalLocationID
			}) != -1, nil
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for Location %s to appear in API: %w", globalLocationID, err)
	}

	return nil
}
