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

// DeviceAttributesBuilder provides read access to operator-managed DeviceAttributes resources.
type DeviceAttributesBuilder struct {
	common.EmbeddableBuilder[drav1.DeviceAttributes, *drav1.DeviceAttributes]
}

// GetGVK returns the DeviceAttributes resource's group, version and kind.
func (builder *DeviceAttributesBuilder) GetGVK() schema.GroupVersionKind {
	return drav1.SchemeGroupVersion.WithKind("DeviceAttributes")
}

// PullDeviceAttributes fetches an existing DeviceAttributes by name and namespace.
func PullDeviceAttributes(apiClient *clients.Settings, name, namespace string) (*DeviceAttributesBuilder, error) {
	return common.PullNamespacedBuilder[drav1.DeviceAttributes, DeviceAttributesBuilder](
		context.TODO(), apiClient, drav1.AddToScheme, name, namespace)
}

// ListDeviceAttributes returns matching DeviceAttributes resources in the specified namespace.
func ListDeviceAttributes(
	apiClient *clients.Settings,
	namespace string,
	options ...runtimeclient.ListOption) ([]*DeviceAttributesBuilder, error) {
	if namespace == "" {
		return nil, commonerrors.NewBuilderFieldEmpty(
			commonkey.NewResourceKey("DeviceAttributes", "", ""), commonerrors.BuilderFieldNamespace)
	}

	allOptions := append(append([]runtimeclient.ListOption{}, options...), runtimeclient.InNamespace(namespace))

	return common.List[drav1.DeviceAttributes, drav1.DeviceAttributesList, DeviceAttributesBuilder](
		context.TODO(), apiClient, drav1.AddToScheme, allOptions...)
}
