package bmh

import (
	"context"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HardwareDataBuilder provides a struct for the HardwareData resource containing a connection to the cluster and the
// HardwareData definition.
type HardwareDataBuilder struct {
	common.EmbeddableBuilder[bmhv1alpha1.HardwareData, *bmhv1alpha1.HardwareData]
}

// GetGVK returns the HardwareData GVK for this builder.
func (builder *HardwareDataBuilder) GetGVK() schema.GroupVersionKind {
	return bmhv1alpha1.GroupVersion.WithKind("HardwareData")
}

// PullHardwareData fetches an existing HardwareData from the cluster by name and namespace.
func PullHardwareData(apiClient *clients.Settings, name, nsname string) (*HardwareDataBuilder, error) {
	return common.PullNamespacedBuilder[bmhv1alpha1.HardwareData, HardwareDataBuilder](
		context.TODO(), apiClient, bmhv1alpha1.AddToScheme, name, nsname)
}

// ListHardwareData returns all HardwareData CRs across all namespaces.
func ListHardwareData(apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*HardwareDataBuilder, error) {
	return common.List[bmhv1alpha1.HardwareData, bmhv1alpha1.HardwareDataList, HardwareDataBuilder](
		context.TODO(), apiClient, bmhv1alpha1.AddToScheme, options...)
}
