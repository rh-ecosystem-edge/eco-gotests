package configmap

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1Typed "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

// Builder provides struct for configmap object containing connection to the cluster and the configmap definitions.
type Builder struct {
	// ConfigMap definition. Used to create configmap object.
	Definition *corev1.ConfigMap
	// Created configmap object.
	Object *corev1.ConfigMap
	// Used in functions that defines or mutates configmap definition. errorMsg is processed before the configmap
	// object is created.
	errorMsg  string
	apiClient corev1Typed.CoreV1Interface
}

// AdditionalOptions additional options for configmap object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// Pull retrieves an existing configmap object from the cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	builder := Builder{
		apiClient: apiClient.CoreV1Interface,
		Definition: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the configmap is empty")

		return nil, fmt.Errorf("configmap 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the configmap is empty")

		return nil, fmt.Errorf("configmap 'nsname' cannot be empty")
	}

	klog.V(100).Infof(
		"Pulling configmap object name:%s in namespace: %s", name, nsname)

	if !builder.Exists() {
		return nil, fmt.Errorf("configmap object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string) *Builder {
	klog.V(100).Infof(
		"Initializing new configmap structure with the following params: %s, %s", name, nsname)

	builder := &Builder{
		apiClient: apiClient.CoreV1Interface,
		Definition: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the configmap is empty")

		builder.errorMsg = "configmap 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the configmap is empty")

		builder.errorMsg = "configmap 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Create makes a configmap in cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the configmap %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.ConfigMaps(builder.Definition.Namespace).Create(
			logging.DiscardContext(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a configmap.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the configmap %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("configmap %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.ConfigMaps(builder.Definition.Namespace).Delete(
		logging.DiscardContext(), builder.Object.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given configmap exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if configmap %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.ConfigMaps(builder.Definition.Namespace).Get(
		logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing configmap object with configmap definition in builder.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating configmap %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.ConfigMaps(builder.Definition.Namespace).
		Update(logging.DiscardContext(), builder.Definition, metav1.UpdateOptions{})
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("configmap", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithData defines the data placed in the configmap.
func (builder *Builder) WithData(data map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating configmap %s in namespace %s with this data: %s",
		builder.Definition.Name, builder.Definition.Namespace, data)

	if len(data) == 0 {
		builder.errorMsg = "'data' cannot be empty"

		return builder
	}

	builder.Definition.Data = data

	return builder
}

// WithOptions creates configmap with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting configmap additional options")

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

// GetGVR returns configmap's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "configmaps",
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ConfigMap"

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
