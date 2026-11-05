package ocm

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	operatorv1 "open-cluster-management.io/api/operator/v1"
)

var klusterletGVK = schema.GroupVersion{Group: operatorv1.GroupName, Version: "v1"}.WithKind("Klusterlet")

// KlusterletName is the name of the klusterlet created by importing a ManagedCluster.
const KlusterletName = "klusterlet"

// KlusterletBuilder provides a struct to interface with Klusterlet resources on a specific cluster.
type KlusterletBuilder struct {
	common.EmbeddableBuilder[operatorv1.Klusterlet, *operatorv1.Klusterlet]
	common.EmbeddableCreator[operatorv1.Klusterlet, KlusterletBuilder, *operatorv1.Klusterlet, *KlusterletBuilder]
	common.EmbeddableDeleter[operatorv1.Klusterlet, *operatorv1.Klusterlet]
	common.EmbeddableUpdater[operatorv1.Klusterlet, KlusterletBuilder, *operatorv1.Klusterlet, *KlusterletBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *KlusterletBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
}

// GetGVK returns the Klusterlet GVK for this builder.
func (builder *KlusterletBuilder) GetGVK() schema.GroupVersionKind {
	return klusterletGVK
}

// NewKlusterletBuilder creates a new instance of a Klusterlet builder.
func NewKlusterletBuilder(apiClient *clients.Settings, name string) *KlusterletBuilder {
	return common.NewClusterScopedBuilder[operatorv1.Klusterlet, KlusterletBuilder](
		apiClient, operatorv1.Install, name)
}

// PullKlusterlet pulls an existing Klusterlet into a Builder struct.
func PullKlusterlet(apiClient *clients.Settings, name string) (*KlusterletBuilder, error) {
	return common.PullClusterScopedBuilder[operatorv1.Klusterlet, KlusterletBuilder](
		context.TODO(), apiClient, operatorv1.Install, name)
}
