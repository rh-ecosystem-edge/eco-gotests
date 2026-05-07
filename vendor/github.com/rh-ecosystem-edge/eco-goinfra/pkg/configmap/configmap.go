package configmap

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// AdditionalOptions are optional mutations applied via WithOptions.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// Builder provides a configmap builder backed by the shared common builder framework.
type Builder struct {
	common.EmbeddableBuilder[corev1.ConfigMap, *corev1.ConfigMap]
	common.EmbeddableWithOptions[corev1.ConfigMap, Builder, *corev1.ConfigMap, *Builder, AdditionalOptions]
	common.EmbeddableCreator[corev1.ConfigMap, Builder, *corev1.ConfigMap, *Builder]
	common.EmbeddableDeleter[corev1.ConfigMap, *corev1.ConfigMap]
	common.EmbeddableUpdater[corev1.ConfigMap, Builder, *corev1.ConfigMap, *Builder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *Builder) AttachMixins() {
	builder.EmbeddableWithOptions.SetBase(builder)
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
}

// GetGVK returns the ConfigMap GVK for this builder.
func (builder *Builder) GetGVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("ConfigMap")
}

// Pull retrieves an existing configmap object from the cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	return common.PullNamespacedBuilder[corev1.ConfigMap, Builder](
		context.TODO(), apiClient, corev1.AddToScheme, name, nsname)
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string) *Builder {
	return common.NewNamespacedBuilder[corev1.ConfigMap, Builder](apiClient, corev1.AddToScheme, name, nsname)
}

// WithData defines the data placed in the configmap.
func (builder *Builder) WithData(data map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof(
		"Creating configmap %s in namespace %s with this data: %s",
		builder.Definition.Name, builder.Definition.Namespace, data)

	if len(data) == 0 {
		builder.SetError(fmt.Errorf("'data' cannot be empty"))

		return builder
	}

	builder.Definition.Data = data

	return builder
}

// GetGVR returns configmap's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "configmaps",
	}
}
