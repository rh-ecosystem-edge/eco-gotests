package ocm

import (
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

var mcsbGVK = schema.GroupVersion{
	Group:   clusterv1beta2.GroupVersion.Group,
	Version: clusterv1beta2.GroupVersion.Version,
}.WithKind("ManagedClusterSetBinding")

// MCSBBuilder provides a struct for the ManagedClusterSetBinding resource containing a connection to the cluster and
// the ManagedClusterSetBinding definition.
type MCSBBuilder struct {
	common.EmbeddableBuilder[clusterv1beta2.ManagedClusterSetBinding, *clusterv1beta2.ManagedClusterSetBinding]
	common.EmbeddableCreator[
		clusterv1beta2.ManagedClusterSetBinding,
		MCSBBuilder,
		*clusterv1beta2.ManagedClusterSetBinding,
		*MCSBBuilder,
	]
	common.EmbeddableDeleter[clusterv1beta2.ManagedClusterSetBinding, *clusterv1beta2.ManagedClusterSetBinding]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *MCSBBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the ManagedClusterSetBinding GVK for this builder.
func (builder *MCSBBuilder) GetGVK() schema.GroupVersionKind {
	return mcsbGVK
}

// NewMCSBBuilder creates a new instance of MCSBBuilder. The ManagedClusterSetBinding spec clusterSet is set to name
// because the binding name must match the bound ManagedClusterSet name.
func NewMCSBBuilder(apiClient *clients.Settings, name, nsname string) *MCSBBuilder {
	klog.V(100).Infof(
		"Initializing new ManagedClusterSetBinding structure with the following params: name: %s, nsname: %s",
		name, nsname)

	builder := common.NewNamespacedBuilder[clusterv1beta2.ManagedClusterSetBinding, MCSBBuilder](
		apiClient, clusterv1beta2.Install, name, nsname)

	if err := common.Validate(builder); err != nil {
		return builder
	}

	builder.Definition.Spec.ClusterSet = name

	return builder
}
