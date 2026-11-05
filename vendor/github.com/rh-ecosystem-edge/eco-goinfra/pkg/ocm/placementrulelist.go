package ocm

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPlacementrulesInAllNamespaces returns a cluster-wide placementrule inventory.
func ListPlacementrulesInAllNamespaces(apiClient *clients.Settings,
	options ...runtimeclient.ListOptions) (
	[]*PlacementRuleBuilder, error) {
	return common.List[placementrulev1.PlacementRule, placementrulev1.PlacementRuleList, PlacementRuleBuilder](
		context.TODO(), apiClient, placementrulev1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
}
