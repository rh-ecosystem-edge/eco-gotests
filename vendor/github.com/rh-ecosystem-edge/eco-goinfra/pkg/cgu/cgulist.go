package cgu

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/cgu/clustergroupupgrades/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListInAllNamespaces returns a cluster-wide cgu inventory.
func ListInAllNamespaces(apiClient *clients.Settings, options ...client.ListOptions) ([]*CguBuilder, error) {
	return common.List[v1alpha1.ClusterGroupUpgrade, v1alpha1.ClusterGroupUpgradeList, CguBuilder](
		context.TODO(), apiClient, v1alpha1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
}
