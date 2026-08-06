package inventory

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"k8s.io/apimachinery/pkg/labels"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// VendorModelPair identifies a distinct hardware vendor/model combination.
type VendorModelPair struct {
	Vendor string
	Model  string
}

// FindResourcePoolWithInventoryBMHs returns the first Ready ResourcePool that has at least one inventory-eligible BMH.
func FindResourcePoolWithInventoryBMHs(
	hubClient *clients.Settings,
	namespace string,
) (*oran.ResourcePoolBuilder, []*bmh.BmhBuilder, error) {
	readyPools, err := oran.ListReadyResourcePools(hubClient, runtimeclient.InNamespace(namespace))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list Ready ResourcePool CRs: %w", err)
	}

	for _, pool := range readyPools {
		eligible, err := bmh.ListInventoryEligibleBMH(hubClient, runtimeclient.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set{
				tsparams.ResourcePoolNameLabel: pool.Definition.Name,
			}),
		})
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to list inventory-eligible BMHs for ResourcePool %s: %w", pool.Definition.Name, err)
		}

		if len(eligible) > 0 {
			return pool, eligible, nil
		}
	}

	return nil, nil, fmt.Errorf("no ResourcePool with inventory-eligible BMHs found")
}

// CollectResourceTypePairs returns distinct vendor/model pairs from inventory-eligible BMHs.
func CollectResourceTypePairs(hubClient *clients.Settings) (map[VendorModelPair]struct{}, error) {
	hosts, err := bmh.ListInventoryEligibleBMH(hubClient)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory-eligible BareMetalHosts: %w", err)
	}

	pairs := make(map[VendorModelPair]struct{})

	for _, host := range hosts {
		hwDataBuilder, err := bmh.PullHardwareData(hubClient, host.Definition.Name, host.Definition.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to pull HardwareData for BareMetalHost %s/%s: %w",
				host.Definition.Namespace, host.Definition.Name, err)
		}

		if hwDataBuilder.Definition == nil || hwDataBuilder.Definition.Spec.HardwareDetails == nil {
			continue
		}

		pair := VendorModelPair{
			Vendor: hwDataBuilder.Definition.Spec.HardwareDetails.SystemVendor.Manufacturer,
			Model:  hwDataBuilder.Definition.Spec.HardwareDetails.SystemVendor.ProductName,
		}
		pairs[pair] = struct{}{}
	}

	return pairs, nil
}
