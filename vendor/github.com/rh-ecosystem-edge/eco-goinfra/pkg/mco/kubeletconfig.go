package mco

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/golang/glog"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeletConfigBuilder provides struct for KubeletConfig Object which contains connection to cluster
// and KubeletConfig definitions.
type KubeletConfigBuilder struct {
	// KubeletConfig definition. Used to create KubeletConfig object with minimum set of required elements.
	Definition *mcv1.KubeletConfig
	// Created KubeletConfig object on the cluster.
	Object *mcv1.KubeletConfig
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg is processed before KubeletConfig object is created.
	errorMsg string
}

// AdditionalOptions for kubeletconfig object.
type AdditionalOptions func(builder *KubeletConfigBuilder) (*KubeletConfigBuilder, error)

// NewKubeletConfigBuilder provides struct for KubeletConfig object which contains connection to cluster
// and KubeletConfig definition.
func NewKubeletConfigBuilder(apiClient *clients.Settings, name string) *KubeletConfigBuilder {
	glog.V(100).Infof("Initializing new KubeletConfigBuilder structure with the name: %s", name)

	if apiClient == nil {
		glog.V(100).Info("The apiClient of the KubeletConfig is nil")

		return nil
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil
	}

	builder := &KubeletConfigBuilder{
		apiClient: apiClient,
		Definition: &mcv1.KubeletConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the KubeletConfig is empty")

		builder.errorMsg = "kubeletconfig 'name' cannot be empty"

		return builder
	}

	return builder
}

// PullKubeletConfig fetches existing kubeletconfig from cluster.
func PullKubeletConfig(apiClient *clients.Settings, name string) (*KubeletConfigBuilder, error) {
	glog.V(100).Infof("Pulling existing kubeletconfig name %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Info("The apiClient of the KubeletConfig is nil")

		return nil, fmt.Errorf("kubeletconfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil, err
	}

	builder := &KubeletConfigBuilder{
		apiClient: apiClient,
		Definition: &mcv1.KubeletConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the kubeletconfig is empty")

		return nil, fmt.Errorf("kubeletconfig 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("kubeletconfig object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the KubeletConfig object if found.
func (builder *KubeletConfigBuilder) Get() (*mcv1.KubeletConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting KubeletConfig object %s", builder.Definition.Name)

	kubeletConfig := &mcv1.KubeletConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, kubeletConfig)
	if err != nil {
		glog.V(100).Infof("KubeletConfig object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return kubeletConfig, nil
}

// Create generates a kubeletconfig in the cluster and stores the created object in struct.
func (builder *KubeletConfigBuilder) Create() (*KubeletConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating KubeletConfig %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes the kubeletconfig.
func (builder *KubeletConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the kubeletconfig object %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("KubeletConfig %s cannot be deleted because it does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
	if err != nil {
		return fmt.Errorf("cannot delete kubeletconfig: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given kubeletconfig exists.
func (builder *KubeletConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if the kubeletconfig object %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithMCPoolSelector redefines kubeletconfig definition with the given machineConfigPoolSelector field.
func (builder *KubeletConfigBuilder) WithMCPoolSelector(key, value string) *KubeletConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Labeling the kubeletconfig %s with %s=%s", builder.Definition.Name, key, value)

	if key == "" {
		glog.V(100).Infof("The key cannot be empty")

		builder.errorMsg = "'key' cannot be empty"

		return builder
	}

	if builder.Definition.Spec.MachineConfigPoolSelector == nil {
		builder.Definition.Spec.MachineConfigPoolSelector = &metav1.LabelSelector{}
	}

	if builder.Definition.Spec.MachineConfigPoolSelector.MatchLabels == nil {
		builder.Definition.Spec.MachineConfigPoolSelector.MatchLabels = map[string]string{}
	}

	builder.Definition.Spec.MachineConfigPoolSelector.MatchLabels[key] = value

	return builder
}

// WithSystemReserved redefines kubeletconfig definition with the given systemreserved fields.
func (builder *KubeletConfigBuilder) WithSystemReserved(cpu, memory string) *KubeletConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting cpu=%s and memory=%s in the %s kubeletconfig definition",
		cpu, memory, builder.Definition.Name)

	if cpu == "" {
		glog.V(100).Infof("The cpu cannot be empty")

		builder.errorMsg = "'cpu' cannot be empty"

		return builder
	}

	if memory == "" {
		glog.V(100).Infof("The memory cannot be empty")

		builder.errorMsg = "'memory' cannot be empty"

		return builder
	}

	if builder.Definition.Spec.KubeletConfig == nil {
		builder.Definition.Spec.KubeletConfig = &runtime.RawExtension{}
	}

	systemReservedKubeletConfiguration := &kubeletconfigv1beta1.KubeletConfiguration{
		SystemReserved: map[string]string{
			"cpu":    cpu,
			"memory": memory,
		},
	}

	builder.Definition.Spec.KubeletConfig.Object = systemReservedKubeletConfiguration

	return builder
}

// WithOptions creates the kubeletconfig with generic mutation options.
func (builder *KubeletConfigBuilder) WithOptions(options ...AdditionalOptions) *KubeletConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting kubeletconfig additional options")

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

func (builder *KubeletConfigBuilder) validate() (bool, error) {
	resourceCRD := "KubeletConfig"

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
