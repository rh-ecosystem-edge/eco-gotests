package ocm

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/kacv1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// KACBuilder provides a struct for the KlusterletAddonConfig resource containing a connection to the cluster and the
// KlusterletAddonConfig definition.
type KACBuilder struct {
	common.EmbeddableBuilder[kacv1.KlusterletAddonConfig, *kacv1.KlusterletAddonConfig]
	common.EmbeddableCreator[kacv1.KlusterletAddonConfig, KACBuilder, *kacv1.KlusterletAddonConfig, *KACBuilder]
	common.EmbeddableDeleter[kacv1.KlusterletAddonConfig, *kacv1.KlusterletAddonConfig]
	common.EmbeddableForceUpdater[kacv1.KlusterletAddonConfig, KACBuilder, *kacv1.KlusterletAddonConfig, *KACBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *KACBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the KlusterletAddonConfig GVK for this builder.
func (builder *KACBuilder) GetGVK() schema.GroupVersionKind {
	return kacv1.SchemeGroupVersion.WithKind("KlusterletAddonConfig")
}

// NewKACBuilder creates a new instance of a KlusterletAddonConfig builder.
func NewKACBuilder(apiClient *clients.Settings, name, nsname string) *KACBuilder {
	return common.NewNamespacedBuilder[kacv1.KlusterletAddonConfig, KACBuilder](
		apiClient, kacv1.SchemeBuilder.AddToScheme, name, nsname)
}

// PullKAC pulls an existing KlusterletAddonConfig into a Builder struct.
func PullKAC(apiClient *clients.Settings, name, nsname string) (*KACBuilder, error) {
	return common.PullNamespacedBuilder[kacv1.KlusterletAddonConfig, KACBuilder](
		context.TODO(), apiClient, kacv1.SchemeBuilder.AddToScheme, name, nsname)
}

// WaitUntilSearchCollectorEnabled waits up to the specified timeout until the search collector config has been enabled.
func (builder *KACBuilder) WaitUntilSearchCollectorEnabled(timeout time.Duration) (*KACBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof(
		"Waiting until KlusterletAddonConfig %s in namespace %s has search collector enabled",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"klusterletAddonConfig object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			return builder.Object.Spec.SearchCollectorConfig.Enabled, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}
