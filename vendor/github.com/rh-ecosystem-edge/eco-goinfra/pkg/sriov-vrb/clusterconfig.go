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

// ClusterConfigBuilder provides struct for the SriovVrbClusterConfig object containing connection to
// the cluster and the SriovVrbClusterConfig definitions.
type ClusterConfigBuilder struct {
	// SriovVrbClusterConfig definition. Used to create SriovVrbClusterConfig object.
	Definition *sriovvrbtypes.SriovVrbClusterConfig
	// Create SriovVrbClusterConfig object.
	Object *sriovvrbtypes.SriovVrbClusterConfig
	// apiClient opens a connection to the cluster.
	apiClient runtimeclient.Client
	// Used in functions that define SriovVrbClusterConfig definitions. errorMsg is processed before SriovVrbClusterConfig
	// object is created.
	errorMsg string
}

// ClusterAdditionalOptions additional options for SriovVrbclusterconfig object.
type ClusterAdditionalOptions func(builder *ClusterConfigBuilder) (*ClusterConfigBuilder, error)

// NewClusterConfigBuilder creates a new instance of ClusterConfigBuilder.
func NewClusterConfigBuilder(
	apiClient *clients.Settings,
	name, nsname string) *ClusterConfigBuilder {
	glog.V(100).Infof(
		"Initializing new SriovVrbClusterConfig structure with the following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("SriovVrbClusterConfig 'apiClient' cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(sriovvrbtypes.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriov-vrb scheme to client schemes")

		return nil
	}

	builder := &ClusterConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovvrbtypes.SriovVrbClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the SriovVrbClusterConfig is empty")

		builder.errorMsg = "SriovVrbClusterConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the SriovVrbClusterConfig is empty")

		builder.errorMsg = "SriovVrbClusterConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullClusterConfig retrieves an existing SriovVrbClusterConfig.io object from the cluster.
func PullClusterConfig(apiClient *clients.Settings, name, nsname string) (*ClusterConfigBuilder, error) {
	glog.V(100).Infof(
		"Pulling SriovVrbClusterConfig.io object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("SriovVrbClusterConfig 'apiClient' cannot be nil")

		return nil, fmt.Errorf("SriovVrbClusterConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(sriovvrbtypes.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriov-vrb scheme to client schemes")

		return nil, err
	}

	builder := &ClusterConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovvrbtypes.SriovVrbClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the SriovVrbClusterConfig is empty")

		return nil, fmt.Errorf("SriovVrbClusterConfig 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the SriovVrbClusterConfig is empty")

		return nil, fmt.Errorf("SriovVrbClusterConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Infof("Cannot pull non-existent SriovVrbClusterConfig object %s in namespace %s", name, nsname)

		return nil, fmt.Errorf("SriovVrbClusterConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given SriovVrbClusterConfig exists.
func (builder *ClusterConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if SriovVrbClusterConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		glog.V(100).Infof("Failed to collect SriovVrbClusterConfig object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a SriovVrbClusterConfig in the cluster and stores the created object in struct.
func (builder *ClusterConfigBuilder) Create() (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the SriovVrbclusterconfig %s in namespace %s",
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

// Get returns SriovVrbClusterConfig object if found.
func (builder *ClusterConfigBuilder) Get() (*sriovvrbtypes.SriovVrbClusterConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting SriovVrbClusterConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nodeConfig := &sriovvrbtypes.SriovVrbClusterConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nodeConfig)
	if err != nil {
		glog.V(100).Infof(
			"SriovVrbClusterConfig object %s does not exist in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return nodeConfig, nil
}

// Delete removes SriovVrbClusterConfig object from a cluster.
func (builder *ClusterConfigBuilder) Delete() (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the SriovVrbClusterConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		glog.V(100).Infof(
			"SriovVrbClusterConfig %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
	if err != nil {
		return nil, fmt.Errorf("can not delete SriovVrbClusterConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing SriovVrbClusterConfig object with the SriovVrbClusterConfig definition in builder.
func (builder *ClusterConfigBuilder) Update(force bool) (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(
		"Updating the SriovVrbClusterConfig object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof(
			"SriovVrbClusterConfig %s in namespace %s does not exist", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent SriovVrbClusterConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("SriovVrbClusterConfig", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("SriovVrbClusterConfig", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithOptions creates SriovVrbClusterConfig with generic mutation options.
func (builder *ClusterConfigBuilder) WithOptions(options ...ClusterAdditionalOptions) *ClusterConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting SriovVrbClusterConfig additional options")

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

// GetSriovVrbClusterConfigIoGVR returns SriovVrbClusterConfig's GroupVersionResource.
func GetSriovVrbClusterConfigIoGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: ClusterConfigsResource,
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterConfigBuilder) validate() (bool, error) {
	resourceCRD := "SriovVrbClusterConfig"

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
