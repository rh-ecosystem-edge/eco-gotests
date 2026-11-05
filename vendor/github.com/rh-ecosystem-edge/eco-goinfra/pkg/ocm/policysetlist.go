package ocm

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPolicieSetsInAllNamespaces returns a cluster-wide policySets inventory.
func ListPolicieSetsInAllNamespaces(apiClient *clients.Settings,
	options ...runtimeclient.ListOptions) (
	[]*PolicySetBuilder, error) {
	return common.List[policiesv1beta1.PolicySet, policiesv1beta1.PolicySetList, PolicySetBuilder](
		context.TODO(), apiClient, policiesv1beta1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
}
