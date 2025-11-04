package kmm

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ModuleBuilder provides struct for the module object containing connection to
// the cluster and the module definitions.
type ModuleBuilder struct {
	// Module definition. Used to create a Module object.
	Definition *moduleV1Beta1.Module
	// Created Module object.
	Object *moduleV1Beta1.Module
	// Used in functions that define or mutate Module definition. errorMsg is processed before the Module
	// object is created.
	apiClient goclient.Client
	errorMsg  string
}

// ModuleAdditionalOptions additional options for module object.
type ModuleAdditionalOptions func(builder *ModuleBuilder) (*ModuleBuilder, error)

// NewModuleBuilder creates a new instance of ModuleBuilder.
func NewModuleBuilder(
	apiClient *clients.Settings, name, nsname string) *ModuleBuilder {
	glog.V(100).Infof(
		"Initializing new Module structure with following params: %s, %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(moduleV1Beta1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add module v1beta1 scheme to client schemes")

		return nil
	}

	builder := &ModuleBuilder{
		apiClient: apiClient.Client,
		Definition: &moduleV1Beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the Module is empty")

		builder.errorMsg = "module 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the module is empty")

		builder.errorMsg = "module 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithNodeSelector adds the specified NodeSelector to the Module.
func (builder *ModuleBuilder) WithNodeSelector(nodeSelector map[string]string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating Module %s in namespace %s with this nodeSelector: %s",
		builder.Definition.Name, builder.Definition.Namespace, nodeSelector)

	if len(nodeSelector) == 0 {
		glog.V(100).Infof("Can not redefine Module with empty nodeSelector map")

		builder.errorMsg = "Module 'nodeSelector' cannot be empty map"

		return builder
	}

	builder.Definition.Spec.Selector = nodeSelector

	return builder
}

// WithLoadServiceAccount adds the specified Load ServiceAccount to the Module.
func (builder *ModuleBuilder) WithLoadServiceAccount(srvAccountName string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating Module %s in namespace %s with ModuleLoad ServiceAccount: %s",
		builder.Definition.Name, builder.Definition.Namespace, srvAccountName)

	return builder.withServiceAccount(srvAccountName, "module")
}

// WithDevicePluginServiceAccount adds the specified Device Plugin ServiceAccount to the Module.
func (builder *ModuleBuilder) WithDevicePluginServiceAccount(srvAccountName string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating Module %s in namespace %s with DevicePlugin ServiceAccount: %s",
		builder.Definition.Name, builder.Definition.Namespace, srvAccountName)

	return builder.withServiceAccount(srvAccountName, "device")
}

// WithImageRepoSecret adds the specific ImageRepoSecret to the Module.
func (builder *ModuleBuilder) WithImageRepoSecret(imageRepoSecret string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if imageRepoSecret == "" {
		builder.errorMsg = "can not redefine module with empty imageRepoSecret"

		return builder
	}

	builder.Definition.Spec.ImageRepoSecret = &corev1.LocalObjectReference{Name: imageRepoSecret}

	return builder
}

// WithDevicePluginVolume adds the specified DevicePlugin volume to the Module.
func (builder *ModuleBuilder) WithDevicePluginVolume(name string, configMapName string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if name == "" {
		builder.errorMsg = "cannot redefine with empty volume 'name'"

		return builder
	}

	if configMapName == "" {
		builder.errorMsg = "cannot redefine with empty 'configMapName'"

		return builder
	}

	if builder.Definition.Spec.DevicePlugin == nil {
		builder.Definition.Spec.DevicePlugin = &moduleV1Beta1.DevicePluginSpec{}
	}

	builder.Definition.Spec.DevicePlugin.Volumes = append(builder.Definition.Spec.DevicePlugin.Volumes, corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName},
			},
		},
	})

	return builder
}

// WithToleration adds the specified toleration to the Module.
func (builder *ModuleBuilder) WithToleration(key, operator, value, effect string,
	seconds *int64) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if key == "" {
		builder.errorMsg = "cannot redefine with empty 'key' value"

		return builder
	}

	if operator == "" {
		builder.errorMsg = "cannot redefine with empty 'operator' value"

		return builder
	}

	if effect == "" {
		builder.errorMsg = "cannot redefine with empty 'effect' value"

		return builder
	}

	glog.V(100).Infof(
		"Appending toleration with key: %s, operator: %s, value: %s, effect: %s and seconds: %s",
		key, operator, value, effect, seconds)

	builder.Definition.Spec.Tolerations = append(builder.Definition.Spec.Tolerations, []corev1.Toleration{{
		Key:               key,
		Effect:            corev1.TaintEffect(effect),
		Operator:          corev1.TolerationOperator(operator),
		Value:             value,
		TolerationSeconds: seconds,
	}}...)

	return builder
}

// WithModuleLoaderContainer adds the specified ModuleLoader container to the Module.
func (builder *ModuleBuilder) WithModuleLoaderContainer(
	container *moduleV1Beta1.ModuleLoaderContainerSpec) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if container == nil {
		builder.errorMsg = "invalid 'container' argument can not be nil"

		return builder
	}

	if builder.Definition.Spec.ModuleLoader == nil {
		builder.Definition.Spec.ModuleLoader = &moduleV1Beta1.ModuleLoaderSpec{}
	}

	builder.Definition.Spec.ModuleLoader.Container = *container

	return builder
}

// WithDevicePluginContainer adds the specified DevicePlugin container to the Module.
func (builder *ModuleBuilder) WithDevicePluginContainer(
	container *moduleV1Beta1.DevicePluginContainerSpec) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if container == nil {
		builder.errorMsg = "invalid 'container' argument can not be nil"

		return builder
	}

	if builder.Definition.Spec.DevicePlugin == nil {
		builder.Definition.Spec.DevicePlugin = &moduleV1Beta1.DevicePluginSpec{}
	}

	builder.Definition.Spec.DevicePlugin.Container = *container

	return builder
}

// BuildModuleSpec returns module spec.
func (builder *ModuleBuilder) BuildModuleSpec() (moduleV1Beta1.ModuleSpec, error) {
	if valid, err := builder.validate(); !valid {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	glog.V(100).Infof(
		"Returning the ModuleSpec structure %v", builder.Definition.Spec)

	return builder.Definition.Spec, nil
}

// WithOptions creates Module with generic mutation options.
func (builder *ModuleBuilder) WithOptions(options ...ModuleAdditionalOptions) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting Module additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// Pull pulls existing module from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*ModuleBuilder, error) {
	glog.V(100).Infof("Pulling existing module name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("module 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(moduleV1Beta1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add module v1beta1 scheme to client schemes")

		return nil, err
	}

	builder := &ModuleBuilder{
		apiClient: apiClient.Client,
		Definition: &moduleV1Beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the module is empty")

		return nil, fmt.Errorf("module 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the module is empty")

		return nil, fmt.Errorf("module 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("module object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds module in the cluster and stores object in struct.
func (builder *ModuleBuilder) Create() (*ModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating module %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies the existing module in the cluster.
func (builder *ModuleBuilder) Update() (*ModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating module %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given module exists.
func (builder *ModuleBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if module %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes the module.
func (builder *ModuleBuilder) Delete() (*ModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting module %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("module cannot be deleted because it does not exist")

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = nil

	return builder, nil
}

// Get fetches the defined module from the cluster.
func (builder *ModuleBuilder) Get() (*moduleV1Beta1.Module, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting module %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	module := &moduleV1Beta1.Module{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, module)
	if err != nil {
		return nil, err
	}

	return module, nil
}

func (builder *ModuleBuilder) withServiceAccount(srvAccountName string, accountType string) *ModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if srvAccountName == "" {
		builder.errorMsg = "can not redefine module with empty ServiceAccount"

		return builder
	}

	switch accountType {
	case "module":
		if builder.Definition.Spec.ModuleLoader == nil {
			builder.Definition.Spec.ModuleLoader = &moduleV1Beta1.ModuleLoaderSpec{}
		}

		builder.Definition.Spec.ModuleLoader.ServiceAccountName = srvAccountName
	case "device":
		if builder.Definition.Spec.DevicePlugin == nil {
			builder.Definition.Spec.DevicePlugin = &moduleV1Beta1.DevicePluginSpec{}
		}

		builder.Definition.Spec.DevicePlugin.ServiceAccountName = srvAccountName
	default:
		builder.errorMsg = "invalid account type parameter. Supported parameters are: 'module', 'device'"

		return builder
	}

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ModuleBuilder) validate() (bool, error) {
	resourceCRD := "Module"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
