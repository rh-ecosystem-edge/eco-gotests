package resource

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceSliceBuilder provides a struct for the ResourceSlice resource containing a connection
// to the cluster and the ResourceSlice definitions.
type ResourceSliceBuilder struct {
	common.EmbeddableBuilder[resourcev1.ResourceSlice, *resourcev1.ResourceSlice]
}

// GetGVK returns the ResourceSlice GVK for this builder.
func (builder *ResourceSliceBuilder) GetGVK() schema.GroupVersionKind {
	return resourcev1.SchemeGroupVersion.WithKind("ResourceSlice")
}

// ListResourceSlices returns ResourceSlice builders matching the given options.
func ListResourceSlices(
	apiClient *clients.Settings,
	options ...runtimeclient.ListOption) ([]*ResourceSliceBuilder, error) {
	klog.V(100).Info("Listing ResourceSlices")

	return common.List[resourcev1.ResourceSlice, resourcev1.ResourceSliceList, ResourceSliceBuilder](
		context.TODO(), apiClient, resourcev1.AddToScheme, options...)
}

// ListResourceSlicesByDriver returns ResourceSlice builders filtered by driver name.
func ListResourceSlicesByDriver(
	apiClient *clients.Settings,
	driverName string) ([]*ResourceSliceBuilder, error) {
	klog.V(100).Infof("Listing ResourceSlices for driver %s", driverName)

	if driverName == "" {
		return nil, fmt.Errorf("resourceSlice 'driverName' cannot be empty")
	}

	allBuilders, err := ListResourceSlices(apiClient)
	if err != nil {
		return nil, err
	}

	var filtered []*ResourceSliceBuilder

	for _, builder := range allBuilders {
		if builder.Object.Spec.Driver == driverName {
			filtered = append(filtered, builder)
		}
	}

	return filtered, nil
}
