package kmm

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// ModuleLoaderContainerBuilder provides struct for the module object containing the ModuleLoaderContainerSpec
// definitions.
type ModuleLoaderContainerBuilder struct {
	// ModuleLoaderContainerBuilder definition. Used to create a Module object.
	definition *moduleV1Beta1.ModuleLoaderContainerSpec
	// errorMsg is processed before the Module object is created.
	errorMsg string
}

// ModuleLoaderContainerAdditionalOptions additional options for ModuleLoaderContainer object.
type ModuleLoaderContainerAdditionalOptions func(
	builder *ModuleLoaderContainerBuilder) (*ModuleLoaderContainerBuilder, error)

// NewModLoaderContainerBuilder creates a new instance of ModuleLoaderContainerBuilder.
func NewModLoaderContainerBuilder(modName string) *ModuleLoaderContainerBuilder {
	klog.V(100).Infof(
		"Initializing new ModuleLoaderContainerBuilder structure with following params: %s", modName)

	builder := &ModuleLoaderContainerBuilder{
		definition: &moduleV1Beta1.ModuleLoaderContainerSpec{
			Modprobe: moduleV1Beta1.ModprobeSpec{
				ModuleName: modName,
			},
		},
	}

	if modName == "" {
		klog.V(100).Info("The modName of the NewModLoaderContainerBuilder is empty")

		builder.errorMsg = "'modName' cannot be empty"

		return builder
	}

	return builder
}

// WithModprobeSpec adds the specified Modprobe to the ModuleLoaderContainerBuilder.
func (builder *ModuleLoaderContainerBuilder) WithModprobeSpec(dirName, fwPath string,
	parameters, args, rawargs, moduleLoadingOrder []string) *ModuleLoaderContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new ModuleLoaderContainerBuilder structure with following modprob params. "+
			"DirName: %s, FirmwarePath: %s, Parameters: %v, ModuleLoadingOrder: %v",
		dirName, fwPath, parameters, moduleLoadingOrder)

	builder.definition.Modprobe.DirName = dirName
	builder.definition.Modprobe.FirmwarePath = fwPath
	builder.definition.Modprobe.Parameters = parameters
	builder.definition.Modprobe.ModulesLoadingOrder = moduleLoadingOrder

	if len(args) > 0 {
		if builder.definition.Modprobe.Args == nil {
			builder.definition.Modprobe.Args = &moduleV1Beta1.ModprobeArgs{}
		}

		builder.definition.Modprobe.Args.Load = args
	}

	if len(rawargs) > 0 {
		if builder.definition.Modprobe.RawArgs == nil {
			builder.definition.Modprobe.RawArgs = &moduleV1Beta1.ModprobeArgs{}
		}

		builder.definition.Modprobe.RawArgs.Load = rawargs
	}

	return builder
}

// WithKernelMapping adds the specified KernelMapping to the ModuleLoaderContainerBuilder.
func (builder *ModuleLoaderContainerBuilder) WithKernelMapping(
	mapping *moduleV1Beta1.KernelMapping) *ModuleLoaderContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new ModuleLoaderContainerBuilder structure with following KernelMapping %v", mapping)

	if mapping == nil {
		klog.V(100).Info("The mapping is undefined")

		builder.errorMsg = "'mapping' can not be empty nil"

		return builder
	}

	builder.definition.KernelMappings = append(builder.definition.KernelMappings, *mapping)

	return builder
}

// WithImagePullPolicy adds the specified ImagePullPolicy to the ModuleLoaderContainerBuilder.
func (builder *ModuleLoaderContainerBuilder) WithImagePullPolicy(policy string) *ModuleLoaderContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new ModuleLoaderContainerBuilder structure with following policy %v", policy)

	if policy == "" {
		builder.errorMsg = "'policy' can not be empty"

		return builder
	}

	builder.definition.ImagePullPolicy = corev1.PullPolicy(policy)

	return builder
}

// WithVersion adds the specified version to the ModuleLoaderContainerBuilder.
func (builder *ModuleLoaderContainerBuilder) WithVersion(version string) *ModuleLoaderContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting ModuleLoaderContainer version %v", version)

	if version == "" {
		builder.errorMsg = "'version' can not be empty"

		return builder
	}

	builder.definition.Version = version

	return builder
}

// WithOptions creates ModuleLoaderContainer with generic mutation options.
func (builder *ModuleLoaderContainerBuilder) WithOptions(
	options ...ModuleLoaderContainerAdditionalOptions) *ModuleLoaderContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting ModuleLoaderContainer additional options")

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

// BuildModuleLoaderContainerCfg returns ModuleLoaderContainerSpec struct.
func (builder *ModuleLoaderContainerBuilder) BuildModuleLoaderContainerCfg() (
	*moduleV1Beta1.ModuleLoaderContainerSpec, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Returning the ModuleLoaderContainerBuilder structure %v", builder.definition)

	return builder.definition, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ModuleLoaderContainerBuilder) validate() (bool, error) {
	resourceCRD := "ModuleLoaderContainer"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}

// DevicePluginContainerBuilder provides struct for the module object containing the DevicePluginContainerSpec
// definitions.
type DevicePluginContainerBuilder struct {
	// DevicePluginContainerBuilder definition. Used to create a Module object.
	definition *moduleV1Beta1.DevicePluginContainerSpec
	// object is created.
	errorMsg string
}

// NewDevicePluginContainerBuilder creates DevicePluginContainerSpec based on given arguments and mutation functs.
func NewDevicePluginContainerBuilder(image string) *DevicePluginContainerBuilder {
	klog.V(100).Infof(
		"Initializing new DevPluginContainerBuilder structure with the following params: %s", image)

	builder := &DevicePluginContainerBuilder{
		definition: &moduleV1Beta1.DevicePluginContainerSpec{
			Image: image,
		},
	}

	if image == "" {
		klog.V(100).Info("The image of NewDevicePluginContainerBuilder is empty")

		builder.errorMsg = "invalid parameter 'image' cannot be empty"

		return builder
	}

	return builder
}

// WithEnv adds specific env to DevicePlugin Container.
func (builder *DevicePluginContainerBuilder) WithEnv(name, value string) *DevicePluginContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new DevPluginContainerBuilder structure with following Env. Name: %s, Value: %s", name, value)

	if name == "" {
		klog.V(100).Info("The name of WithEnv is empty")

		builder.errorMsg = "'name' can not be empty for DevicePlugin Env"

		return builder
	}

	if value == "" {
		klog.V(100).Info("The value of WithEnv is empty")

		builder.errorMsg = "'value' can not be empty for DevicePlugin Env"

		return builder
	}

	builder.definition.Env = append(builder.definition.Env, corev1.EnvVar{Name: name, Value: value})

	return builder
}

// WithVolumeMount adds VolumeMount to DevicePlugin Container.
func (builder *DevicePluginContainerBuilder) WithVolumeMount(mountPath, name string) *DevicePluginContainerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new DevPluginContainerBuilder structure with mountPath Env. Name: %s, MountPath: %s",
		name, mountPath)

	if name == "" {
		klog.V(100).Info("The name of WithVolumeMount is empty")

		builder.errorMsg = "'name' can not be empty for DevicePlugin mountPath"

		return builder
	}

	if mountPath == "" {
		klog.V(100).Info("The mountPath of WithVolumeMount is empty")

		builder.errorMsg = "'mountPath' can not be empty for DevicePlugin mountPath"

		return builder
	}

	builder.definition.VolumeMounts = append(
		builder.definition.VolumeMounts, corev1.VolumeMount{Name: name, MountPath: mountPath})

	return builder
}

// GetDevicePluginContainerConfig returns DevicePluginContainerSpec with needed configuration.
func (builder *DevicePluginContainerBuilder) GetDevicePluginContainerConfig() (
	*moduleV1Beta1.DevicePluginContainerSpec, error) {
	if valid, err := builder.validate(); !valid {
		return nil, fmt.Errorf("error building DevicePluginContainerSpec config due to :%w", err)
	}

	return builder.definition, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *DevicePluginContainerBuilder) validate() (bool, error) {
	resourceCRD := "DevicePluginContainer"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", strings.ToLower(resourceCRD))

		return false, fmt.Errorf("error: received nil %s builder", strings.ToLower(resourceCRD))
	}

	if builder.definition == nil {
		klog.V(100).Infof("The %s is undefined", strings.ToLower(resourceCRD))

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", strings.ToLower(resourceCRD), builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
