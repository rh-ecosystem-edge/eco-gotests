package sriovfec

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	sriovfectypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/fec/fectypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterConfigBuilder provides struct for the SriovFecClusterConfig object containing connection to
// the cluster and the SriovFecClusterConfig definitions.
type ClusterConfigBuilder struct {
	// SriovFecClusterConfig definition. Used to create SriovFecClusterConfig object.
	Definition *sriovfectypes.SriovFecClusterConfig
	// Create SriovFecClusterConfig object.
	Object *sriovfectypes.SriovFecClusterConfig
	// apiClient opens a connection to the cluster.
	apiClient runtimeclient.Client
	// Used in functions that define SriovFecClusterConfig definitions. errorMsg is processed before SriovFecClusterConfig
	// object is created.
	errorMsg string
}

// ClusterAdditionalOptions additional options for SriovFecClusterConfig object.
type ClusterAdditionalOptions func(builder *ClusterConfigBuilder) (*ClusterConfigBuilder, error)

// NewClusterConfigBuilder creates a new instance of ClusterConfigBuilder.
func NewClusterConfigBuilder(
	apiClient *clients.Settings,
	name, nsname string) *ClusterConfigBuilder {
	klog.V(100).Infof(
		"Initializing new SriovFecClusterConfig structure with the following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("SriovFecClusterConfig 'apiClient' cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(sriovfectypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriov-fec scheme to client schemes")

		return nil
	}

	builder := &ClusterConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovfectypes.SriovFecClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the SriovFecClusterConfig is empty")

		builder.errorMsg = "SriovFecClusterConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovFecClusterConfig is empty")

		builder.errorMsg = "SriovFecClusterConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullClusterConfig retrieves an existing SriovFecClusterConfig.io object from the cluster.
func PullClusterConfig(apiClient *clients.Settings, name, nsname string) (*ClusterConfigBuilder, error) {
	klog.V(100).Infof(
		"Pulling SriovFecClusterConfig.io object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("SriovFecClusterConfig 'apiClient' cannot be nil")

		return nil, fmt.Errorf("SriovFecClusterConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(sriovfectypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriov-fec scheme to client schemes")

		return nil, err
	}

	builder := &ClusterConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovfectypes.SriovFecClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the SriovFecClusterConfig is empty")

		return nil, fmt.Errorf("SriovFecClusterConfig 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovFecClusterConfig is empty")

		return nil, fmt.Errorf("SriovFecClusterConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("Cannot pull non-existent SriovFecClusterConfig object %s in namespace %s", name, nsname)

		return nil, fmt.Errorf("SriovFecClusterConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given SriovFecClusterConfig exists.
func (builder *ClusterConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if SriovFecClusterConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect SriovFecClusterConfig object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a SriovFecClusterConfig in the cluster and stores the created object in struct.
func (builder *ClusterConfigBuilder) Create() (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the SriovFecClusterConfig %s in namespace %s",
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

// Get returns SriovFecClusterConfig object if found.
func (builder *ClusterConfigBuilder) Get() (*sriovfectypes.SriovFecClusterConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting SriovFecClusterConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	clusterConfig := &sriovfectypes.SriovFecClusterConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, clusterConfig)
	if err != nil {
		klog.V(100).Infof(
			"SriovFecClusterConfig object %s does not exist in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return clusterConfig, nil
}

// Delete removes SriovFecClusterConfig object from a cluster.
func (builder *ClusterConfigBuilder) Delete() (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the SriovFecClusterConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof(
			"SriovFecClusterConfig %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return nil, fmt.Errorf("can not delete SriovFecClusterConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing SriovFecClusterConfig object with the SriovFecClusterConfig definition in builder.
func (builder *ClusterConfigBuilder) Update(force bool) (*ClusterConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof(
		"Updating the SriovFecClusterConfig object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"SriovFecClusterConfig %s in namespace %s does not exist", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent SriovFecClusterConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("SriovFecClusterConfig", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("SriovFecClusterConfig", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithOptions creates SriovFecClusterConfig with generic mutation options.
func (builder *ClusterConfigBuilder) WithOptions(options ...ClusterAdditionalOptions) *ClusterConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting SriovFecClusterConfig additional options")

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

// GetSriovFecClusterConfigIoGVR returns SriovFecClusterConfig's GroupVersionResource.
func GetSriovFecClusterConfigIoGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: ClusterConfigsResource,
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterConfigBuilder) validate() (bool, error) {
	resourceCRD := "SriovFecClusterConfig"

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
