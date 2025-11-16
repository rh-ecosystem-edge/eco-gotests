package sriovvrb

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	sriovvrbtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/fec/vrbtypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeConfigBuilder provides struct for the SriovVrbNodeConfig object containing connection to
// the cluster and the SriovVrbNodeConfig definitions.
type NodeConfigBuilder struct {
	// SriovVrbNodeConfig definition. Used to create SriovVrbNodeConfig object.
	Definition *sriovvrbtypes.SriovVrbNodeConfig
	// Create SriovVrbNodeConfig object.
	Object *sriovvrbtypes.SriovVrbNodeConfig
	// apiClient opens a connection to the cluster.
	apiClient runtimeclient.Client
	// Used in functions that define SriovVrbNodeConfig definitions. errorMsg is processed before SriovVrbNodeConfig
	// object is created.
	errorMsg string
}

// NodeAdditionalOptions additional options for SriovVrbnodeconfig object.
type NodeAdditionalOptions func(builder *NodeConfigBuilder) (*NodeConfigBuilder, error)

// NewNodeConfigBuilder creates a new instance of NodeConfigBuilder.
func NewNodeConfigBuilder(
	apiClient *clients.Settings,
	name, nsname string) *NodeConfigBuilder {
	glog.V(100).Infof(
		"Initializing new sriovVrbNodeConfig structure with the following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("sriovVrbNodeConfig 'apiClient' cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(sriovvrbtypes.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriov-vrb scheme to client schemes")

		return nil
	}

	builder := &NodeConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovvrbtypes.SriovVrbNodeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the sriovVrbNodeConfig is empty")

		builder.errorMsg = "sriovVrbNodeConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the sriovVrbNodeConfig is empty")

		builder.errorMsg = "sriovVrbNodeConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullNodeConfig retrieves an existing SriovVrbNodeConfig.io object from the cluster.
func PullNodeConfig(apiClient *clients.Settings, name, nsname string) (*NodeConfigBuilder, error) {
	glog.V(100).Infof(
		"Pulling SriovVrbNodeConfig.io object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("sriovVrbNodeConfig 'apiClient' cannot be nil")

		return nil, fmt.Errorf("sriovVrbNodeConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(sriovvrbtypes.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriov-vrb scheme to client schemes")

		return nil, err
	}

	builder := &NodeConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovvrbtypes.SriovVrbNodeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the sriovVrbNodeConfig is empty")

		return nil, fmt.Errorf("sriovVrbNodeConfig 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the SriovVrbNodeConfig is empty")

		return nil, fmt.Errorf("sriovVrbNodeConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Infof("Cannot pull non-existent sriovVrbNodeConfig object %s in namespace %s", name, nsname)

		return nil, fmt.Errorf("sriovVrbNodeConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given SriovVrbNodeConfig exists.
func (builder *NodeConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if sriovVrbNodeConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		glog.V(100).Infof("Failed to collect sriovVrbNodeConfig object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a SriovVrbNodeConfig in the cluster and stores the created object in struct.
func (builder *NodeConfigBuilder) Create() (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the SriovVrbnodeconfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Get returns SriovVrbNodeConfig object if found.
func (builder *NodeConfigBuilder) Get() (*sriovvrbtypes.SriovVrbNodeConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting SriovVrbNodeConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nodeConfig := &sriovvrbtypes.SriovVrbNodeConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nodeConfig)
	if err != nil {
		glog.V(100).Infof(
			"SriovVrbNodeConfig object %s does not exist in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return nodeConfig, nil
}

// Delete removes SriovVrbNodeConfig object from a cluster.
func (builder *NodeConfigBuilder) Delete() (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the SriovVrbNodeConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		glog.V(100).Infof(
			"SriovVrbNodeConfig %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
	if err != nil {
		return nil, fmt.Errorf("can not delete SriovVrbNodeConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing SriovVrbNodeConfig object with the SriovVrbNodeConfig definition in builder.
func (builder *NodeConfigBuilder) Update(force bool) (*NodeConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(
		"Updating the SriovVrbNodeConfig object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof(
			"SriovVrbNodeConfig %s in namespace %s does not exist", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent SriovVrbNodeConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("SriovVrbNodeConfig", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("SriovVrbNodeConfig", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithOptions creates SriovVrbNodeConfig with generic mutation options.
func (builder *NodeConfigBuilder) WithOptions(options ...NodeAdditionalOptions) *NodeConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting SriovVrbNodeConfig additional options")

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

// GetSriovVrbNodeConfigIoGVR returns SriovVrbNodeConfig's GroupVersionResource which could be used for Clean function.
func GetSriovVrbNodeConfigIoGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: NodeConfigsResource,
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NodeConfigBuilder) validate() (bool, error) {
	resourceCRD := "SriovVrbNodeConfig"

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
