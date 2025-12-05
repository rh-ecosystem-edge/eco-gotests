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
