package amdgpu

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	amdgpuv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/amd/gpu-operator/api/v1alpha1"
	"k8s.io/klog/v2"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for DeviceConfig object
// from the cluster and a DeviceConfig definition.
type Builder struct {
	// Builder definition. Used to create
	// Builder object with minimum set of required elements.
	Definition *amdgpuv1.DeviceConfig
	// Created Builder object on the cluster.
	Object *amdgpuv1.DeviceConfig
	// api client to interact with the cluster.
	apiClient *clients.Settings
	// errorMsg is processed before Builder object is created.
	errorMsg string
}

// NewBuilderFromObjectString creates a Builder object from CSV alm-examples.
func NewBuilderFromObjectString(apiClient *clients.Settings, almExample string) *Builder {
	klog.V(100).Infof(
		"Initializing new Builder structure from almExample string")

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the DeviceConfig is nil")

		return nil
	}

	err := apiClient.AttachScheme(amdgpuv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add amdgpu v1 scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient,
	}

	deviceConfig, err := getDeviceConfigFromAlmExample(almExample)
	if err != nil {
		klog.V(100).Infof(
			"Error initializing DeviceConfig from alm-examples: %s", err.Error())

		builder.errorMsg = fmt.Sprintf("error initializing DeviceConfig from alm-examples: %s",
			err.Error())

		return builder
	}

	builder.Definition = deviceConfig

	klog.V(100).Infof(
		"Initializing Builder definition to DeviceConfig object")

	if builder.Definition == nil {
		klog.V(100).Info("The DeviceConfig object definition is nil")

		builder.errorMsg = "deviceConfig definition is nil"

		return builder
	}

	return builder
}

// Pull loads an existing DeviceConfig into Builder struct.
func Pull(apiClient *clients.Settings, name, namespace string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing deviceConfig name: %s in namespace: %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the Policy is nil")

		return nil, fmt.Errorf("the apiClient of the Policy is nil")
	}

	err := apiClient.AttachScheme(amdgpuv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add amdgpu v1 scheme to client schemes")

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &amdgpuv1.DeviceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("DeviceConfig name is empty")

		return nil, fmt.Errorf("DeviceConfig 'name' cannot be empty")
	}

	if namespace == "" {
		klog.V(100).Info("DeviceConfig namespace is empty")

		return nil, fmt.Errorf("DeviceConfig 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("deviceConfig object %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns DeviceConfig object if found.
func (builder *Builder) Get() (*amdgpuv1.DeviceConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting DeviceConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	deviceConfig := &amdgpuv1.DeviceConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, deviceConfig)
	if err != nil {
		klog.V(100).Infof("DeviceConfig object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return deviceConfig, nil
}

// Exists checks whether the given DeviceConfig exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if DeviceConfig %s exists in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect DeviceConfig object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a DeviceConfig.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting DeviceConfig %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("DeviceConfig '%s' in namespace '%s' cannot be deleted because it does not exist",
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

// Create makes a DeviceConfig in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the DeviceConfig %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update renovates the existing DeviceConfig object with the definition in builder.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the DeviceConfig object named: %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("DeviceConfig", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("DeviceConfig", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// getDeviceConfigFromAlmExample extracts the DeviceConfig from the alm-examples block.
func getDeviceConfigFromAlmExample(almExample string) (*amdgpuv1.DeviceConfig, error) {
	deviceConfigList := &amdgpuv1.DeviceConfigList{}

	if almExample == "" {
		return nil, fmt.Errorf("almExample is an empty string")
	}

	err := json.Unmarshal([]byte(almExample), &deviceConfigList.Items)
	if err != nil {
		return nil, err
	}

	if len(deviceConfigList.Items) == 0 {
		return nil, fmt.Errorf("failed to get alm examples")
	}

	return &deviceConfigList.Items[0], nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
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

	return true, nil
}
