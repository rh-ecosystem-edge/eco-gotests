package secret

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// Builder provides struct for secret object containing connection to the cluster and the secret definitions.
type Builder struct {
	// Secret definition. Used to store the secret object.
	Definition *corev1.Secret
	// Created secret object.
	Object *corev1.Secret
	// Used in functions that define or mutate secret definitions. errorMsg is processed before the secret
	// object is created.
	errorMsg string
	// api client to interact with the cluster.
	apiClient *clients.Settings
}

// AdditionalOptions additional options for Secret object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string, secretType corev1.SecretType) *Builder {
	klog.V(100).Infof(
		"Initializing new secret structure with the following params: %s, %s, %s",
		name, nsname, string(secretType))

	if apiClient == nil {
		klog.V(100).Info("secret 'apiClient' cannot be empty")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Type: secretType,
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the secret is empty")

		builder.errorMsg = "secret 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the secret is empty")

		builder.errorMsg = "secret 'nsname' cannot be empty"

		return builder
	}

	if secretType == "" {
		klog.V(100).Info("The secretType of the secret is empty")

		builder.errorMsg = "secret 'secretType' cannot be empty"

		return builder
	}

	return builder
}

// Pull loads an existing secret into Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing secret name: %s under namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("secret 'apiClient' cannot be empty")
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("secret name is empty")

		return nil, fmt.Errorf("secret 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the secret is empty")

		return nil, fmt.Errorf("secret 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("secret object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a secret in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the secret %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.Secrets(builder.Definition.Namespace).Create(
			logging.DiscardContext(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a secret from the cluster.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the secret %s from namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("Secret %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Secrets(builder.Definition.Namespace).Delete(
		logging.DiscardContext(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given secret exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if secret %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Secrets(builder.Definition.Namespace).Get(
		logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update modifies the existing secret in the cluster.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating secret %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Secrets(builder.Definition.Namespace).Update(
		logging.DiscardContext(), builder.Definition, metav1.UpdateOptions{})

	return builder, err
}

// WithData defines the data placed in the secret.
func (builder *Builder) WithData(data map[string][]byte) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Defining secret %s in namespace %s with this data: %s",
		builder.Definition.Name, builder.Definition.Namespace, data)

	if len(data) == 0 {
		klog.V(100).Info("The data of the secret is empty")

		builder.errorMsg = "'data' cannot be empty"

		return builder
	}

	builder.Definition.Data = data

	return builder
}

// WithStringData defines the stringData placed in the secret.
func (builder *Builder) WithStringData(data map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Defining secret %s in namespace %s with this stringData: %s",
		builder.Definition.Name, builder.Definition.Namespace, data)

	if len(data) == 0 {
		klog.V(100).Info("The stringData of the secret is empty")

		builder.errorMsg = "'stringData' cannot be empty"

		return builder
	}

	builder.Definition.StringData = data

	return builder
}

// WithAnnotations defines the annotations in the secret.
func (builder *Builder) WithAnnotations(annotations map[string]string) *Builder {
	klog.V(100).Infof("Adding annotations %v to the secret %s in namespace %s",
		annotations, builder.Definition.Name, builder.Definition.Namespace)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(annotations) == 0 {
		klog.V(100).Info("'annotations' argument cannot be empty")

		builder.errorMsg = "'annotations' argument cannot be empty"

		return builder
	}

	for key := range annotations {
		if key == "" {
			klog.V(100).Info("The 'annotations' key cannot be empty")

			builder.errorMsg = "can not apply an annotations with an empty key"

			return builder
		}
	}

	builder.Definition.Annotations = annotations

	return builder
}

// WithOptions creates secret with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting secret additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "Secret"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
