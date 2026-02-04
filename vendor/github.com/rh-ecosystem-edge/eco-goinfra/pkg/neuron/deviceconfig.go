package neuron

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	neuronv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/neuron/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides struct for DeviceConfig object
// containing connection to the cluster and the DeviceConfig definition.
type Builder struct {
	// Definition of the DeviceConfig. Used to create the object.
	Definition *neuronv1beta1.DeviceConfig
	// Created DeviceConfig object.
	Object *neuronv1beta1.DeviceConfig
	// API client to interact with the cluster.
	apiClient *clients.Settings
	// Error message for validation.
	errorMsg string
}

// AdditionalOptions additional options for DeviceConfig object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// NewBuilder creates a new instance of Builder.
func NewBuilder(
	apiClient *clients.Settings,
	name, namespace string,
	driversImage, driverVersion, devicePluginImage string) *Builder {
	klog.V(100).Infof(
		"Initializing new DeviceConfig structure with name: %s, namespace: %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Infof("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(neuronv1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add neuron v1beta1 scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &neuronv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: neuronv1beta1.DeviceConfigSpec{
				DriversImage:      driversImage,
				DriverVersion:     driverVersion,
				DevicePluginImage: devicePluginImage,
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The name of the DeviceConfig is empty")

		builder.errorMsg = "DeviceConfig 'name' cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Infof("The namespace of the DeviceConfig is empty")

		builder.errorMsg = "DeviceConfig 'namespace' cannot be empty"

		return builder
	}

	if driversImage == "" {
		klog.V(100).Infof("The driversImage of the DeviceConfig is empty")

		builder.errorMsg = "DeviceConfig 'driversImage' cannot be empty"

		return builder
	}

	if driverVersion == "" {
		klog.V(100).Infof("The driverVersion of the DeviceConfig is empty")

		builder.errorMsg = "DeviceConfig 'driverVersion' cannot be empty"

		return builder
	}

	if devicePluginImage == "" {
		klog.V(100).Infof("The devicePluginImage of the DeviceConfig is empty")

		builder.errorMsg = "DeviceConfig 'devicePluginImage' cannot be empty"

		return builder
	}

	return builder
}

// WithSelector sets the node selector for the DeviceConfig.
func (builder *Builder) WithSelector(selector map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting DeviceConfig %s in namespace %s with selector: %v",
		builder.Definition.Name, builder.Definition.Namespace, selector)

	if len(selector) == 0 {
		klog.V(100).Infof("DeviceConfig 'selector' cannot be empty map")

		builder.errorMsg = "DeviceConfig 'selector' cannot be empty map"

		return builder
	}

	builder.Definition.Spec.Selector = selector

	return builder
}

// WithScheduler adds custom scheduler configuration to the DeviceConfig.
func (builder *Builder) WithScheduler(
	schedulerImage, extensionImage string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting DeviceConfig %s in namespace %s with scheduler images",
		builder.Definition.Name, builder.Definition.Namespace)

	if schedulerImage == "" {
		klog.V(100).Infof("DeviceConfig 'schedulerImage' cannot be empty")

		builder.errorMsg = "DeviceConfig 'schedulerImage' cannot be empty"

		return builder
	}

	if extensionImage == "" {
		klog.V(100).Infof("DeviceConfig 'extensionImage' cannot be empty")

		builder.errorMsg = "DeviceConfig 'extensionImage' cannot be empty"

		return builder
	}

	builder.Definition.Spec.CustomSchedulerImage = schedulerImage
	builder.Definition.Spec.SchedulerExtensionImage = extensionImage

	return builder
}

// WithImageRepoSecret sets the image repository secret.
func (builder *Builder) WithImageRepoSecret(secretName string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting DeviceConfig %s in namespace %s with imageRepoSecret: %s",
		builder.Definition.Name, builder.Definition.Namespace, secretName)

	if secretName == "" {
		klog.V(100).Infof("DeviceConfig 'imageRepoSecret' name cannot be empty")

		builder.errorMsg = "DeviceConfig 'imageRepoSecret' name cannot be empty"

		return builder
	}

	builder.Definition.Spec.ImageRepoSecret = &corev1.LocalObjectReference{
		Name: secretName,
	}

	return builder
}

// WithDriverVersion updates the driver version (triggers rolling upgrade).
func (builder *Builder) WithDriverVersion(version string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting DeviceConfig %s in namespace %s with driverVersion: %s",
		builder.Definition.Name, builder.Definition.Namespace, version)

	if version == "" {
		klog.V(100).Infof("DeviceConfig 'driverVersion' cannot be empty")

		builder.errorMsg = "DeviceConfig 'driverVersion' cannot be empty"

		return builder
	}

	builder.Definition.Spec.DriverVersion = version

	return builder
}

// WithNodeMetricsImage sets the node metrics image for the DeviceConfig.
func (builder *Builder) WithNodeMetricsImage(image string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting DeviceConfig %s in namespace %s with nodeMetricsImage: %s",
		builder.Definition.Name, builder.Definition.Namespace, image)

	if image == "" {
		klog.V(100).Infof("DeviceConfig 'nodeMetricsImage' cannot be empty")

		builder.errorMsg = "DeviceConfig 'nodeMetricsImage' cannot be empty"

		return builder
	}

	builder.Definition.Spec.NodeMetricsImage = image

	return builder
}

// Pull retrieves an existing DeviceConfig from the cluster.
func Pull(apiClient *clients.Settings, name, namespace string) (*Builder, error) {
	klog.V(100).Infof("Pulling DeviceConfig %s from namespace %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("deviceConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(neuronv1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add neuron scheme to client schemes")

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &neuronv1beta1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The name of the DeviceConfig is empty")

		return nil, fmt.Errorf("deviceConfig 'name' cannot be empty")
	}

	if namespace == "" {
		klog.V(100).Infof("The namespace of the DeviceConfig is empty")

		return nil, fmt.Errorf("deviceConfig 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("deviceConfig object %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get retrieves the DeviceConfig from the cluster.
func (builder *Builder) Get() (*neuronv1beta1.DeviceConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting DeviceConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	deviceConfig := &neuronv1beta1.DeviceConfig{}

	err := builder.apiClient.Get(
		logging.DiscardContext(),
		goclient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		deviceConfig)
	if err != nil {
		klog.V(100).Infof("DeviceConfig object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return deviceConfig, nil
}

// Exists checks whether the DeviceConfig exists in the cluster.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if DeviceConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect DeviceConfig object due to %s", err.Error())
	}

	return err == nil
}

// Create builds the DeviceConfig in the cluster.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating DeviceConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies the existing DeviceConfig in the cluster.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating DeviceConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("DeviceConfig %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return builder, fmt.Errorf("cannot update non-existent deviceConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%s", msg.FailToUpdateNotification("DeviceConfig",
				builder.Definition.Name, builder.Definition.Namespace))

			deletedBuilder, deleteErr := builder.Delete()
			if deleteErr != nil {
				klog.V(100).Infof("%s", msg.FailToUpdateError("DeviceConfig",
					builder.Definition.Name, builder.Definition.Namespace))

				return nil, deleteErr
			}

			if deletedBuilder == nil {
				return nil, fmt.Errorf("failed to delete DeviceConfig: builder is nil")
			}

			return deletedBuilder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes the DeviceConfig from the cluster.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting DeviceConfig %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("DeviceConfig %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete DeviceConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// WithOptions creates DeviceConfig with generic mutation options.
func (builder *Builder) WithOptions(
	options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting DeviceConfig additional options")

	for _, option := range options {
		if option != nil {
			var err error

			builder, err = option(builder)
			if err != nil {
				klog.V(100).Infof("Error occurred in mutation function")

				if builder == nil {
					klog.V(100).Infof("Mutation function returned nil builder")

					return nil
				}

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// validate checks that the builder is properly configured.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "DeviceConfig"

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
