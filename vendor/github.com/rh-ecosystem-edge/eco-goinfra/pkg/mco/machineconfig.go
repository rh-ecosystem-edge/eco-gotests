package mco

import (
	"context"
	"fmt"

	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MCBuilder provides struct for MachineConfig Object which contains connection to cluster
// and MachineConfig definitions.
type MCBuilder struct {
	// MachineConfig definition. Used to create MachineConfig object with minimum set of required elements.
	Definition *mcv1.MachineConfig
	// Created MachineConfig object on the cluster.
	Object *mcv1.MachineConfig
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg is processed before MachineConfig object is created.
	errorMsg string
}

// MCAdditionalOptions for machineconfig object.
type MCAdditionalOptions func(builder *MCBuilder) (*MCBuilder, error)

// NewMCBuilder provides struct for MachineConfig object which contains connection to cluster
// and MachineConfig definition.
func NewMCBuilder(apiClient *clients.Settings, name string) *MCBuilder {
	klog.V(100).Infof("Initializing new MCBuilder structure with following params: %s", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the MachineConfig is nil")

		return nil
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil
	}

	builder := &MCBuilder{
		apiClient: apiClient.Client,
		Definition: &mcv1.MachineConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the MachineConfig is empty")

		builder.errorMsg = "machineconfig 'name' cannot be empty"

		return builder
	}

	return builder
}

// PullMachineConfig fetches existing machineconfig from cluster.
func PullMachineConfig(apiClient *clients.Settings, name string) (*MCBuilder, error) {
	klog.V(100).Infof("Pulling existing machineconfig name %s from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the MachineConfig is nil")

		return nil, fmt.Errorf("machineconfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil, err
	}

	builder := &MCBuilder{
		apiClient: apiClient.Client,
		Definition: &mcv1.MachineConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the machineconfig is empty")

		return nil, fmt.Errorf("machineconfig 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("machineconfig object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the MachineConfig object if found.
func (builder *MCBuilder) Get() (*mcv1.MachineConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting MachineConfig object %s", builder.Definition.Name)

	machineConfig := &mcv1.MachineConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, machineConfig)
	if err != nil {
		klog.V(100).Infof("MachineConfig object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return machineConfig, nil
}

// Create generates a machineconfig in the cluster and stores the created object in struct.
func (builder *MCBuilder) Create() (*MCBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating MachineConfig %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes the machineconfig.
func (builder *MCBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the MachineConfig object %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("MachineConfig %s cannot be deleted because it does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete machineconfig: %w", err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing machineconfig object with machineconfig definition in builder.
func (builder *MCBuilder) Update() (*MCBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating machineconfig %s", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given machineconfig exists.
func (builder *MCBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if the MachineConfig object %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithLabel redefines machineconfig definition with the given label.
func (builder *MCBuilder) WithLabel(key, value string) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Labeling the machineconfig %s with %s=%s", builder.Definition.Name, key, value)

	if key == "" {
		klog.V(100).Info("The key cannot be empty")

		builder.errorMsg = "'key' cannot be empty"

		return builder
	}

	if builder.Definition.Labels == nil {
		builder.Definition.Labels = map[string]string{}
	}

	builder.Definition.Labels[key] = value

	return builder
}

// WithOptions creates the machineconfig with generic mutation options.
func (builder *MCBuilder) WithOptions(options ...MCAdditionalOptions) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting machineconfig additional options")

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

// WithKernelArguments sets the specified KernelArguments to the MachineConfig.
func (builder *MCBuilder) WithKernelArguments(kernelArgs []string) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(kernelArgs) == 0 {
		klog.V(100).Info("The kernelArgs cannot be empty")

		builder.errorMsg = "'kernelArgs' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting KernelArguments: %v", kernelArgs)

	builder.Definition.Spec.KernelArguments = kernelArgs

	return builder
}

// WithExtensions sets the specified Extensions to the MachineConfig.
func (builder *MCBuilder) WithExtensions(extensions []string) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(extensions) == 0 {
		klog.V(100).Info("The extensions cannot be empty")

		builder.errorMsg = "'extensions' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting Extensions: %v", extensions)

	builder.Definition.Spec.Extensions = extensions

	return builder
}

// WithFIPS sets the specified FIPS value to the MachineConfig.
func (builder *MCBuilder) WithFIPS(fips bool) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting FIPS: %v", fips)

	builder.Definition.Spec.FIPS = fips

	return builder
}

// WithKernelType sets the specified kernelType to the MachineConfig.
func (builder *MCBuilder) WithKernelType(kernelType string) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if kernelType == "" {
		klog.V(100).Info("The kernelType cannot be empty")

		builder.errorMsg = "'kernelType' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting KernelType: %v", kernelType)

	builder.Definition.Spec.KernelType = kernelType

	return builder
}

// WithRawConfig sets the specified Config to the MachineConfig.
func (builder *MCBuilder) WithRawConfig(config []byte) *MCBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(config) == 0 {
		klog.V(100).Info("The Config.Raw cannot be empty")

		builder.errorMsg = "'Config.Raw' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Setting Config.Raw: %s", string(config))

	builder.Definition.Spec.Config.Raw = config

	return builder
}

func (builder *MCBuilder) validate() (bool, error) {
	resourceCRD := "MachineConfig"

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
