package ptp

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	ptpv2alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v2alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HardwareConfigBuilder provides a struct for the HardwareConfig resource containing
// a connection to the cluster and the HardwareConfig definition.
type HardwareConfigBuilder struct {
	common.EmbeddableBuilder[ptpv2alpha1.HardwareConfig, *ptpv2alpha1.HardwareConfig]
	common.EmbeddableUpdater[ptpv2alpha1.HardwareConfig, HardwareConfigBuilder,
		*ptpv2alpha1.HardwareConfig, *HardwareConfigBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *HardwareConfigBuilder) AttachMixins() {
	builder.EmbeddableUpdater.SetBase(builder) //nolint:staticcheck // promoted method is ambiguous without explicit selector
}

// GetGVK returns the HardwareConfig GVK for this builder.
func (builder *HardwareConfigBuilder) GetGVK() schema.GroupVersionKind {
	return ptpv2alpha1.GroupVersion.WithKind("HardwareConfig")
}

// PullHardwareConfig fetches an existing HardwareConfig from the cluster by name and namespace.
func PullHardwareConfig(apiClient *clients.Settings, name, nsname string) (*HardwareConfigBuilder, error) {
	return common.PullNamespacedBuilder[ptpv2alpha1.HardwareConfig, HardwareConfigBuilder](
		context.TODO(), apiClient, ptpv2alpha1.AddToScheme, name, nsname)
}

// ListHardwareConfigs returns all HardwareConfig CRs across all namespaces.
func ListHardwareConfigs(apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*HardwareConfigBuilder, error) {
	return common.List[ptpv2alpha1.HardwareConfig, ptpv2alpha1.HardwareConfigList, HardwareConfigBuilder](
		context.TODO(), apiClient, ptpv2alpha1.AddToScheme, options...)
}
