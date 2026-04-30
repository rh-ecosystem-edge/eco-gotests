package kserve

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	kservev1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kserve/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ServingRuntimeBuilder provides struct for the ServingRuntime object
// containing connection to the cluster and the ServingRuntime definition.
type ServingRuntimeBuilder struct {
	Definition *kservev1alpha1.ServingRuntime
	Object     *kservev1alpha1.ServingRuntime
	apiClient  *clients.Settings
	errorMsg   string
}

// NewServingRuntimeBuilder creates a new instance of ServingRuntimeBuilder.
func NewServingRuntimeBuilder(
	apiClient *clients.Settings,
	name, namespace string) *ServingRuntimeBuilder {
	klog.V(100).Infof(
		"Initializing new ServingRuntime structure with name: %s, namespace: %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Infof("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(kservev1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add kserve v1alpha1 scheme to client schemes")

		return nil
	}

	multiModel := false

	builder := &ServingRuntimeBuilder{
		apiClient: apiClient,
		Definition: &kservev1alpha1.ServingRuntime{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: kservev1alpha1.ServingRuntimeSpec{
				MultiModel: &multiModel,
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The name of the ServingRuntime is empty")

		builder.errorMsg = "ServingRuntime 'name' cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Infof("The namespace of the ServingRuntime is empty")

		builder.errorMsg = "ServingRuntime 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithModelFormat adds a supported model format to the ServingRuntime.
func (builder *ServingRuntimeBuilder) WithModelFormat(
	name string, autoSelect bool) *ServingRuntimeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding model format %s to ServingRuntime %s",
		name, builder.Definition.Name)

	if name == "" {
		builder.errorMsg = "ServingRuntime model format 'name' cannot be empty"

		return builder
	}

	builder.Definition.Spec.SupportedModelFormats = append(
		builder.Definition.Spec.SupportedModelFormats,
		kservev1alpha1.SupportedModelFormat{
			Name:       name,
			AutoSelect: autoSelect,
		})

	return builder
}

// WithContainer adds a container to the ServingRuntime.
func (builder *ServingRuntimeBuilder) WithContainer(
	container corev1.Container) *ServingRuntimeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding container %s to ServingRuntime %s",
		container.Name, builder.Definition.Name)

	builder.Definition.Spec.Containers = append(
		builder.Definition.Spec.Containers, container)

	return builder
}

// WithVolume adds a volume to the ServingRuntime.
func (builder *ServingRuntimeBuilder) WithVolume(
	volume corev1.Volume) *ServingRuntimeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding volume %s to ServingRuntime %s",
		volume.Name, builder.Definition.Name)

	builder.Definition.Spec.Volumes = append(
		builder.Definition.Spec.Volumes, volume)

	return builder
}

// WithAnnotation adds an annotation to the ServingRuntime.
func (builder *ServingRuntimeBuilder) WithAnnotation(
	key, value string) *ServingRuntimeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations[key] = value

	return builder
}

// PullServingRuntime retrieves an existing ServingRuntime from the cluster.
func PullServingRuntime(
	apiClient *clients.Settings, name, namespace string) (*ServingRuntimeBuilder, error) {
	klog.V(100).Infof("Pulling ServingRuntime %s from namespace %s", name, namespace)

	if apiClient == nil {
		return nil, fmt.Errorf("servingRuntime 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(kservev1alpha1.AddToScheme)
	if err != nil {
		return nil, err
	}

	builder := &ServingRuntimeBuilder{
		apiClient: apiClient,
		Definition: &kservev1alpha1.ServingRuntime{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		return nil, fmt.Errorf("servingRuntime 'name' cannot be empty")
	}

	if namespace == "" {
		return nil, fmt.Errorf("servingRuntime 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"servingRuntime %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get retrieves the ServingRuntime from the cluster.
func (builder *ServingRuntimeBuilder) Get() (*kservev1alpha1.ServingRuntime, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	servingRuntime := &kservev1alpha1.ServingRuntime{}

	err := builder.apiClient.Get(
		logging.DiscardContext(),
		goclient.ObjectKey{
			Name:      builder.Definition.Name,
			Namespace: builder.Definition.Namespace,
		},
		servingRuntime)
	if err != nil {
		return nil, err
	}

	return servingRuntime, nil
}

// Exists checks whether the ServingRuntime exists in the cluster.
func (builder *ServingRuntimeBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	var err error

	builder.Object, err = builder.Get()

	return err == nil
}

// Create builds the ServingRuntime in the cluster.
func (builder *ServingRuntimeBuilder) Create() (*ServingRuntimeBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating ServingRuntime %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			return builder, err
		}

		builder.Object = builder.Definition
	}

	return builder, nil
}

// Delete removes the ServingRuntime from the cluster.
func (builder *ServingRuntimeBuilder) Delete() (*ServingRuntimeBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting ServingRuntime %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete ServingRuntime: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// validate checks that the builder is properly configured.
func (builder *ServingRuntimeBuilder) validate() (bool, error) {
	resourceCRD := "ServingRuntime"

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
		klog.V(100).Infof("The %s builder has error message: %s",
			resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
