package ocm

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPlacementBindingsInAllNamespaces returns a cluster-wide placementBinding inventory.
func ListPlacementBindingsInAllNamespaces(apiClient *clients.Settings,
	options ...runtimeclient.ListOptions) (
	[]*PlacementBindingBuilder, error) {
	return common.List[policiesv1.PlacementBinding, policiesv1.PlacementBindingList, PlacementBindingBuilder](
		context.TODO(), apiClient, policiesv1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
}
