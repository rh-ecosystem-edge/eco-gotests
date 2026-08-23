package resource

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	commonerrors "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/errors"
	commonkey "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/key"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceClaimBuilder provides a struct for the ResourceClaim resource containing a connection
// to the cluster and the ResourceClaim definitions.
type ResourceClaimBuilder struct {
	common.EmbeddableBuilder[resourcev1.ResourceClaim, *resourcev1.ResourceClaim]
	common.EmbeddableDeleter[resourcev1.ResourceClaim, *resourcev1.ResourceClaim]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *ResourceClaimBuilder) AttachMixins() {
	builder.SetBase(builder)
}

// GetGVK returns the ResourceClaim GVK for this builder.
func (builder *ResourceClaimBuilder) GetGVK() schema.GroupVersionKind {
	return resourcev1.SchemeGroupVersion.WithKind("ResourceClaim")
}

// NewResourceClaimBuilder creates a new instance of ResourceClaimBuilder.
func NewResourceClaimBuilder(
	apiClient *clients.Settings, name, namespace string) *ResourceClaimBuilder {
	return common.NewNamespacedBuilder[resourcev1.ResourceClaim, ResourceClaimBuilder](
		apiClient, resourcev1.AddToScheme, name, namespace)
}

// PullResourceClaim fetches an existing ResourceClaim from the cluster by name and namespace.
func PullResourceClaim(
	apiClient *clients.Settings, name, namespace string) (*ResourceClaimBuilder, error) {
	return common.PullNamespacedBuilder[resourcev1.ResourceClaim, ResourceClaimBuilder](
		context.TODO(), apiClient, resourcev1.AddToScheme, name, namespace)
}

// ListResourceClaims returns ResourceClaim builders matching the given options in the specified namespace.
func ListResourceClaims(
	apiClient *clients.Settings,
	namespace string,
	options ...runtimeclient.ListOption) ([]*ResourceClaimBuilder, error) {
	klog.V(100).Infof("Listing ResourceClaims in namespace %s", namespace)

	if namespace == "" {
		klog.V(100).Info("ResourceClaim 'namespace' parameter can not be empty")

		return nil, commonerrors.NewBuilderFieldEmpty(
			commonkey.NewResourceKey("ResourceClaim", "", ""), commonerrors.BuilderFieldNamespace)
	}

	allOptions := append(
		append([]runtimeclient.ListOption{}, options...),
		runtimeclient.InNamespace(namespace))

	return common.List[resourcev1.ResourceClaim, resourcev1.ResourceClaimList, ResourceClaimBuilder](
		context.TODO(), apiClient, resourcev1.AddToScheme, allOptions...)
}
