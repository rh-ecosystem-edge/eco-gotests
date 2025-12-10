package kmm

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	bmcV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BootModuleConfigBuilder provides struct for the bootmoduleconfig object containing connection to
// the cluster and the bootmoduleconfig definitions.
type BootModuleConfigBuilder struct {
	// BootModuleConfig definition. Used to create a BootModuleConfig object.
	Definition *bmcV1Beta1.BootModuleConfig
	// Created BootModuleConfig object.
	Object *bmcV1Beta1.BootModuleConfig
	// Used in functions that define or mutate BootModuleConfig definition. errorMsg is processed before the
	// BootModuleConfig object is created.
	apiClient goclient.Client
	errorMsg  string
}

// BootModuleConfigAdditionalOptions additional options for bootmoduleconfig object.
type BootModuleConfigAdditionalOptions func(builder *BootModuleConfigBuilder) (*BootModuleConfigBuilder, error)

// NewBootModuleConfigBuilder creates a new instance of BootModuleConfigBuilder.
func NewBootModuleConfigBuilder(
	apiClient *clients.Settings, name, nsname string) *BootModuleConfigBuilder {
	klog.V(100).Infof(
		"Initializing new BootModuleConfig structure with following params: %s, %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(bmcV1Beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add bootmoduleconfig v1beta1 scheme to client schemes")

		return nil
	}

	builder := &BootModuleConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &bmcV1Beta1.BootModuleConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the BootModuleConfig is empty")

		builder.errorMsg = "bootmoduleconfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the bootmoduleconfig is empty")

		builder.errorMsg = "bootmoduleconfig 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithOptions creates BootModuleConfig with generic mutation options.
func (builder *BootModuleConfigBuilder) WithOptions(
	options ...BootModuleConfigAdditionalOptions) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting BootModuleConfig additional options")

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

// WithMachineConfigName sets the MachineConfig name that is targeted by the BMC.
func (builder *BootModuleConfigBuilder) WithMachineConfigName(mcName string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if mcName == "" {
		builder.errorMsg = "bootmoduleconfig 'machineConfigName' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig MachineConfigName to: %s", mcName)

	builder.Definition.Spec.MachineConfigName = mcName

	return builder
}

// WithMachineConfigPoolName sets the MachineConfigPool name linked to the targeted MachineConfig.
func (builder *BootModuleConfigBuilder) WithMachineConfigPoolName(mcpName string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if mcpName == "" {
		builder.errorMsg = "bootmoduleconfig 'machineConfigPoolName' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig MachineConfigPoolName to: %s", mcpName)

	builder.Definition.Spec.MachineConfigPoolName = mcpName

	return builder
}

// WithKernelModuleImage sets the container image that contains the kernel module .ko file.
func (builder *BootModuleConfigBuilder) WithKernelModuleImage(image string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if image == "" {
		builder.errorMsg = "bootmoduleconfig 'kernelModuleImage' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig KernelModuleImage to: %s", image)

	builder.Definition.Spec.KernelModuleImage = image

	return builder
}

// WithKernelModuleName sets the name of the kernel module to be loaded.
func (builder *BootModuleConfigBuilder) WithKernelModuleName(moduleName string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if moduleName == "" {
		builder.errorMsg = "bootmoduleconfig 'kernelModuleName' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig KernelModuleName to: %s", moduleName)

	builder.Definition.Spec.KernelModuleName = moduleName

	return builder
}

// WithInTreeModulesToRemove sets the in-tree kernel module list to remove prior to loading the OOT kernel module.
func (builder *BootModuleConfigBuilder) WithInTreeModulesToRemove(modules []string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig InTreeModulesToRemove to: %v", modules)

	builder.Definition.Spec.InTreeModulesToRemove = modules

	return builder
}

// WithFirmwareFilesPath sets the path of the firmware files in the kernel module container image.
func (builder *BootModuleConfigBuilder) WithFirmwareFilesPath(path string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig FirmwareFilesPath to: %s", path)

	builder.Definition.Spec.FirmwareFilesPath = path

	return builder
}

// WithWorkerImage sets the KMM worker image.
func (builder *BootModuleConfigBuilder) WithWorkerImage(image string) *BootModuleConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting BootModuleConfig WorkerImage to: %s", image)

	builder.Definition.Spec.WorkerImage = image

	return builder
}

// PullBootModuleConfig pulls existing bootmoduleconfig from cluster.
func PullBootModuleConfig(apiClient *clients.Settings, name, nsname string) (*BootModuleConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing bootmoduleconfig name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("bootmoduleconfig 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(bmcV1Beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add bootmoduleconfig v1beta1 scheme to client schemes")

		return nil, err
	}

	builder := &BootModuleConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &bmcV1Beta1.BootModuleConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the bootmoduleconfig is empty")

		return nil, fmt.Errorf("bootmoduleconfig 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the bootmoduleconfig is empty")

		return nil, fmt.Errorf("bootmoduleconfig 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("bootmoduleconfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds bootmoduleconfig in the cluster and stores object in struct.
func (builder *BootModuleConfigBuilder) Create() (*BootModuleConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating bootmoduleconfig %s in namespace %s",
		builder.Definition.Name,
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

// Update modifies the existing bootmoduleconfig in the cluster.
func (builder *BootModuleConfigBuilder) Update() (*BootModuleConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating bootmoduleconfig %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given bootmoduleconfig exists.
func (builder *BootModuleConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if bootmoduleconfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes the bootmoduleconfig.
func (builder *BootModuleConfigBuilder) Delete() (*BootModuleConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting bootmoduleconfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Info("bootmoduleconfig cannot be deleted because it does not exist")

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""

	return builder, nil
}

// Get fetches the defined bootmoduleconfig from the cluster.
func (builder *BootModuleConfigBuilder) Get() (*bmcV1Beta1.BootModuleConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting bootmoduleconfig %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	bootmoduleconfig := &bmcV1Beta1.BootModuleConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, bootmoduleconfig)
	if err != nil {
		return nil, err
	}

	return bootmoduleconfig, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BootModuleConfigBuilder) validate() (bool, error) {
	resourceCRD := "BootModuleConfig"

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
