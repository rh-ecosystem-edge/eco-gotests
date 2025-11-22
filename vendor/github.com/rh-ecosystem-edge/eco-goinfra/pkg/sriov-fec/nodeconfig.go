package sriovfec

import (
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	sriovfectypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/fec/fectypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeConfigBuilder provides struct for the SriovFecNodeConfig object containing connection to
// the cluster and the SriovFecNodeConfig definitions.
type NodeConfigBuilder struct {
	// SriovFecNodeConfig definition. Used to create SriovFecNodeConfig object.
	Definition *sriovfectypes.SriovFecNodeConfig
	// Create SriovFecNodeConfig object.
	Object *sriovfectypes.SriovFecNodeConfig
	// apiClient opens a connection to the cluster.
	apiClient runtimeclient.Client
	// Used in functions that define SriovFecNodeConfig definitions. errorMsg is processed before SriovFecNodeConfig
	// object is created.
	errorMsg string
}

// AdditionalOptions additional options for sriovfecnodeconfig object.
type AdditionalOptions func(builder *NodeConfigBuilder) (*NodeConfigBuilder, error)

// NewNodeConfigBuilder creates a new instance of NodeConfigBuilder.
func NewNodeConfigBuilder(
	apiClient *clients.Settings,
	name, nsname string,
	label map[string]string) *NodeConfigBuilder {
	klog.V(100).Infof(
		"Initializing new SriovFecNodeConfig structure with the following params: %s, %s, %v",
		name, nsname, label)

	if apiClient == nil {
		klog.V(100).Info("SriovFecNodeConfig 'apiClient' cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(sriovfectypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriov-fec scheme to client schemes")

		return nil
	}

	builder := &NodeConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovfectypes.SriovFecNodeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the SriovFecNodeConfig is empty")

		builder.errorMsg = "sriovFecNodeConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovFecNodeConfig is empty")

		builder.errorMsg = "sriovFecNodeConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Pull retrieves an existing SriovFecNodeConfig.io object from the cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*NodeConfigBuilder, error) {
	klog.V(100).Infof(
		"Pulling SriovFecNodeConfig.io object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("SriovFecNodeConfig 'apiClient' cannot be nil")

		return nil, fmt.Errorf("sriovFecNodeConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(sriovfectypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriov-fec scheme to client schemes")

		return nil, err
	}

	builder := &NodeConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovfectypes.SriovFecNodeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the SriovFecNodeConfig is empty")

		return nil, fmt.Errorf("sriovFecNodeConfig 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovFecNodeConfig is empty")

		return nil, fmt.Errorf("sriovFecNodeConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("Cannot pull non-existent SriovFecNodeConfig object %s in namespace %s", name, nsname)

		return nil, fmt.Errorf("sriovFecNodeConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given SriovFecNodeConfig exists.
func (builder *NodeConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if SriovFecNodeConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect SriovFecNodeConfig object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a SriovFecNodeConfig in the cluster and stores the created object in struct.
func (builder *NodeConfigBuilder) Create() (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the sriovfecnodeconfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Get returns SriovFecNodeConfig object if found.
func (builder *NodeConfigBuilder) Get() (*sriovfectypes.SriovFecNodeConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting SriovFecNodeConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nodeConfig := &sriovfectypes.SriovFecNodeConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nodeConfig)
	if err != nil {
		klog.V(100).Infof(
			"SriovFecNodeConfig object %s does not exist in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return nodeConfig, nil
}

// Delete removes SriovFecNodeConfig object from a cluster.
func (builder *NodeConfigBuilder) Delete() (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the SriovFecNodeConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof(
			"SriovFecNodeConfig %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return nil, fmt.Errorf("can not delete SriovFecNodeConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing SriovFecNodeConfig object with the SriovFecNodeConfig definition in builder.
func (builder *NodeConfigBuilder) Update(force bool) (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof(
		"Updating the SriovFecNodeConfig object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"SriovFecNodeConfig %s in namespace %s does not exist", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent SriovFecNodeConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("SriovFecNodeConfig", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("SriovFecNodeConfig", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithOptions creates SriovFecNodeConfig with generic mutation options.
func (builder *NodeConfigBuilder) WithOptions(options ...AdditionalOptions) *NodeConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting SriovFecNodeConfig additional options")

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

// GetSriovFecNodeConfigIoGVR returns SriovFecNodeConfig's GroupVersionResource which could be used for Clean function.
func GetSriovFecNodeConfigIoGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: NodeConfigsResource,
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NodeConfigBuilder) validate() (bool, error) {
	resourceCRD := "sriovFecNodeConfig"

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
