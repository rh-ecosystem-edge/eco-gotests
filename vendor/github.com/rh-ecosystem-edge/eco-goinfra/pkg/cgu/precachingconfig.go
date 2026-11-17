package cgu

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/cgu/clustergroupupgrades/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// PreCachingConfigBuilder provides a struct for the PreCachingConfig object containing a connection to the cluster and
// the PreCachingConfig definition.
type PreCachingConfigBuilder struct {
	common.EmbeddableBuilder[v1alpha1.PreCachingConfig, *v1alpha1.PreCachingConfig]
	common.EmbeddableCreator[v1alpha1.PreCachingConfig, PreCachingConfigBuilder, *v1alpha1.PreCachingConfig, *PreCachingConfigBuilder]
	common.EmbeddableDeleter[v1alpha1.PreCachingConfig, *v1alpha1.PreCachingConfig]
	common.EmbeddableForceUpdater[v1alpha1.PreCachingConfig, PreCachingConfigBuilder, *v1alpha1.PreCachingConfig, *PreCachingConfigBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *PreCachingConfigBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the PreCachingConfig GVK for this builder.
func (builder *PreCachingConfigBuilder) GetGVK() schema.GroupVersionKind {
	return v1alpha1.SchemeGroupVersion.WithKind("PreCachingConfig")
}

// NewPreCachingConfigBuilder creates a new instance of PreCachingConfig.
func NewPreCachingConfigBuilder(apiClient *clients.Settings, name, nsname string) *PreCachingConfigBuilder {
	klog.V(100).Infof(
		"Initializing new PreCachingConfig structure with the following params: name: %s, nsname: %s", name, nsname)

	return common.NewNamespacedBuilder[v1alpha1.PreCachingConfig, PreCachingConfigBuilder](
		apiClient, v1alpha1.AddToScheme, name, nsname)
}

// PullPreCachingConfig pulls an existing PreCachingConfig into a PreCachingConfigBuilder struct.
func PullPreCachingConfig(apiClient *clients.Settings, name, nsname string) (*PreCachingConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing PreCachingConfig %s under namespace %s from cluster", name, nsname)

	return common.PullNamespacedBuilder[v1alpha1.PreCachingConfig, PreCachingConfigBuilder](
		context.TODO(), apiClient, v1alpha1.AddToScheme, name, nsname)
}
