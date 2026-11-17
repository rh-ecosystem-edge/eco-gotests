package sriovdra

import (
	"context"

	drav1 "github.com/k8snetworkplumbingwg/dra-driver-sriov/pkg/api/sriovdra/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	commonerrors "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/errors"
	commonkey "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/key"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SriovResourcePolicyBuilder provides read access to operator-managed SriovResourcePolicy resources.
type SriovResourcePolicyBuilder struct {
	common.EmbeddableBuilder[drav1.SriovResourcePolicy, *drav1.SriovResourcePolicy]
}

// GetGVK returns the SriovResourcePolicy resource's group, version and kind.
func (builder *SriovResourcePolicyBuilder) GetGVK() schema.GroupVersionKind {
	return drav1.SchemeGroupVersion.WithKind("SriovResourcePolicy")
}

// PullSriovResourcePolicy fetches an existing SriovResourcePolicy by name and namespace.
func PullSriovResourcePolicy(apiClient *clients.Settings, name, namespace string) (*SriovResourcePolicyBuilder, error) {
	return common.PullNamespacedBuilder[drav1.SriovResourcePolicy, SriovResourcePolicyBuilder](
		context.TODO(), apiClient, drav1.AddToScheme, name, namespace)
}

// ListSriovResourcePolicies returns matching SriovResourcePolicy resources in the specified namespace.
func ListSriovResourcePolicies(
	apiClient *clients.Settings,
	namespace string,
	options ...runtimeclient.ListOption) ([]*SriovResourcePolicyBuilder, error) {
	if namespace == "" {
		return nil, commonerrors.NewBuilderFieldEmpty(
			commonkey.NewResourceKey("SriovResourcePolicy", "", ""), commonerrors.BuilderFieldNamespace)
	}

	allOptions := append(append([]runtimeclient.ListOption{}, options...), runtimeclient.InNamespace(namespace))

	return common.List[drav1.SriovResourcePolicy, drav1.SriovResourcePolicyList, SriovResourcePolicyBuilder](
		context.TODO(), apiClient, drav1.AddToScheme, allOptions...)
}
