package secret

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// AdditionalOptions additional options for Secret object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// Builder provides struct for secret object containing connection to the cluster and the secret definitions.
type Builder struct {
	common.EmbeddableBuilder[corev1.Secret, *corev1.Secret]
	common.EmbeddableCreator[corev1.Secret, Builder, *corev1.Secret, *Builder]
	common.EmbeddableDeleter[corev1.Secret, *corev1.Secret]
	common.EmbeddableUpdater[corev1.Secret, Builder, *corev1.Secret, *Builder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *Builder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
}

// GetGVK returns the Secret GVK for this builder.
func (builder *Builder) GetGVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Secret")
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string, secretType corev1.SecretType) *Builder {
	klog.V(100).Infof(
		"Initializing new secret structure with the following params: %s, %s, %s",
		name, nsname, string(secretType))

	builder := common.NewNamespacedBuilder[corev1.Secret, Builder](apiClient, corev1.AddToScheme, name, nsname)

	if builder.GetError() != nil {
		return builder
	}

	if secretType == "" {
		klog.V(100).Info("The secretType of the secret is empty")

		builder.SetError(fmt.Errorf("secret 'secretType' cannot be empty"))

		return builder
	}

	builder.Definition.Type = secretType

	return builder
}

// Pull loads an existing secret into Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	return common.PullNamespacedBuilder[corev1.Secret, Builder](
		context.TODO(), apiClient, corev1.AddToScheme, name, nsname)
}

// WithData defines the data placed in the secret.
func (builder *Builder) WithData(data map[string][]byte) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof(
		"Defining secret %s in namespace %s with data (redacted)",
		builder.Definition.Name, builder.Definition.Namespace)

	if len(data) == 0 {
		klog.V(100).Info("The data of the secret is empty")

		builder.SetError(fmt.Errorf("'data' cannot be empty"))

		return builder
	}

	builder.Definition.Data = data

	return builder
}

// WithStringData defines the stringData placed in the secret.
func (builder *Builder) WithStringData(data map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof(
		"Defining secret %s in namespace %s with stringData (redacted)",
		builder.Definition.Name, builder.Definition.Namespace)

	if len(data) == 0 {
		klog.V(100).Info("The stringData of the secret is empty")

		builder.SetError(fmt.Errorf("'stringData' cannot be empty"))

		return builder
	}

	builder.Definition.StringData = data

	return builder
}

// WithAnnotations defines the annotations in the secret.
func (builder *Builder) WithAnnotations(annotations map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding annotations %v to the secret %s in namespace %s",
		annotations, builder.Definition.Name, builder.Definition.Namespace)

	if len(annotations) == 0 {
		klog.V(100).Info("'annotations' argument cannot be empty")

		builder.SetError(fmt.Errorf("'annotations' argument cannot be empty"))

		return builder
	}

	for key := range annotations {
		if key == "" {
			klog.V(100).Info("The 'annotations' key cannot be empty")

			builder.SetError(fmt.Errorf("can not apply an annotations with an empty key"))

			return builder
		}
	}

	builder.Definition.Annotations = annotations

	return builder
}

// WithOptions creates secret with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Info("Setting secret additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.SetError(err)

				return builder
			}
		}
	}

	return builder
}
