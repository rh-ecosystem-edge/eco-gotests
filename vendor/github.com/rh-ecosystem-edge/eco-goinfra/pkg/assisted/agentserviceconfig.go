package assisted

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	agentServiceConfigName       = "agent"
	defaultDatabaseStorageSize   = "10Gi"
	defaultFilesystemStorageSize = "20Gi"
	defaultImageStoreStorageSize = "10Gi"
)

var (
	validIPXEOptions = []string{"enabled", "disabled"}
)

// AgentServiceConfigBuilder provides struct for the agentserviceconfig object containing connection to
// the cluster and the agentserviceconfig definition.
type AgentServiceConfigBuilder struct {
	Definition *agentInstallV1Beta1.AgentServiceConfig
	Object     *agentInstallV1Beta1.AgentServiceConfig
	errorMsg   string
	apiClient  goclient.Client
}

// AgentServiceConfigAdditionalOptions additional options for AgentServiceConfig object.
type AgentServiceConfigAdditionalOptions func(builder *AgentServiceConfigBuilder) (*AgentServiceConfigBuilder, error)

// NewAgentServiceConfigBuilder creates a new instance of AgentServiceConfigBuilder.
func NewAgentServiceConfigBuilder(
	apiClient *clients.Settings,
	databaseStorageSpec,
	filesystemStorageSpec corev1.PersistentVolumeClaimSpec) *AgentServiceConfigBuilder {
	glog.V(100).Infof(
		"Initializing new agentserviceconfig structure with the following params: "+
			"databaseStorageSpec: %v, filesystemStorageSpec: %v",
		databaseStorageSpec, filesystemStorageSpec)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	builder := &AgentServiceConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.AgentServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: agentServiceConfigName,
			},
			Spec: agentInstallV1Beta1.AgentServiceConfigSpec{
				DatabaseStorage:   databaseStorageSpec,
				FileSystemStorage: filesystemStorageSpec,
			},
		},
	}

	return builder
}

// NewDefaultAgentServiceConfigBuilder creates a new instance of AgentServiceConfigBuilder
// with default storage specs already set.
func NewDefaultAgentServiceConfigBuilder(apiClient *clients.Settings) *AgentServiceConfigBuilder {
	glog.V(100).Infof(
		"Initializing new agentserviceconfig structure")

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	builder := &AgentServiceConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.AgentServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: agentServiceConfigName,
			},
			Spec: agentInstallV1Beta1.AgentServiceConfigSpec{},
		},
	}

	imageStorageSpec, err := GetDefaultStorageSpec(defaultImageStoreStorageSize)
	if err != nil {
		glog.V(100).Infof("The ImageStorage size is in wrong format")

		builder.errorMsg = fmt.Sprintf("error retrieving the storage size: %v", err)

		return builder
	}

	builder.Definition.Spec.ImageStorage = &imageStorageSpec

	databaseStorageSpec, err := GetDefaultStorageSpec(defaultDatabaseStorageSize)
	if err != nil {
		glog.V(100).Infof("The DatabaseStorage size is in wrong format")

		builder.errorMsg = fmt.Sprintf("error retrieving the storage size: %v", err)

		return builder
	}

	builder.Definition.Spec.DatabaseStorage = databaseStorageSpec

	fileSystemStorageSpec, err := GetDefaultStorageSpec(defaultFilesystemStorageSize)
	if err != nil {
		glog.V(100).Infof("The FileSystemStorage size is in wrong format")

		builder.errorMsg = fmt.Sprintf("error retrieving the storage size: %v", err)

		return builder
	}

	builder.Definition.Spec.FileSystemStorage = fileSystemStorageSpec

	return builder
}

// WithImageStorage sets the imageStorageSpec used by the agentserviceconfig.
func (builder *AgentServiceConfigBuilder) WithImageStorage(
	imageStorageSpec corev1.PersistentVolumeClaimSpec) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting imageStorage %v in agentserviceconfig", imageStorageSpec)

	builder.Definition.Spec.ImageStorage = &imageStorageSpec

	return builder
}

// WithMirrorRegistryRef adds a configmap ref to the agentserviceconfig containing mirroring information.
func (builder *AgentServiceConfigBuilder) WithMirrorRegistryRef(configMapName string) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding mirrorRegistryRef %s to agentserviceconfig %s", configMapName, builder.Definition.Name)

	if configMapName == "" {
		glog.V(100).Infof("The configMapName is empty")

		builder.errorMsg = "cannot add agentserviceconfig mirrorRegistryRef with empty configmap name"

		return builder
	}

	builder.Definition.Spec.MirrorRegistryRef = &corev1.LocalObjectReference{
		Name: configMapName,
	}

	return builder
}

// WithOSImage appends an OSImage to the OSImages list used by the agentserviceconfig.
func (builder *AgentServiceConfigBuilder) WithOSImage(osImage agentInstallV1Beta1.OSImage) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding OSImage %v to agentserviceconfig %s", osImage, builder.Definition.Name)

	builder.Definition.Spec.OSImages = append(builder.Definition.Spec.OSImages, osImage)

	return builder
}

// WithUnauthenticatedRegistry appends an unauthenticated registry to the agentserviceconfig.
func (builder *AgentServiceConfigBuilder) WithUnauthenticatedRegistry(registry string) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding unauthenticatedRegistry %s to agentserviceconfig %s", registry, builder.Definition.Name)

	if registry == "" {
		glog.V(100).Infof("AgentServiceConfig UnauthenticatedRegistry supplied empty registry")

		builder.errorMsg = "agentserviceconfig cannot have empty unauthenticated registry"

		return builder
	}

	builder.Definition.Spec.UnauthenticatedRegistries = append(builder.Definition.Spec.UnauthenticatedRegistries, registry)

	return builder
}

// WithIPXEHTTPRoute sets the IPXEHTTPRoute type to be used by the agentserviceconfig.
func (builder *AgentServiceConfigBuilder) WithIPXEHTTPRoute(route string) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding IPXEHTTPRout %s to agentserviceconfig %s", route, builder.Definition.Name)

	if !slices.Contains(validIPXEOptions, route) {
		glog.V(100).Infof("Receieved incorrect IPXEHTTPRoute option: %s, valid options: %v", route, validIPXEOptions)

		builder.errorMsg =
			fmt.Sprintf("agentserviceconfig passed invalid ipxeroute: %s, valid options: %v", route, validIPXEOptions)

		return builder
	}

	builder.Definition.Spec.IPXEHTTPRoute = route

	return builder
}

// WithOptions creates AgentServiceConfig with generic mutation options.
func (builder *AgentServiceConfigBuilder) WithOptions(
	options ...AgentServiceConfigAdditionalOptions) *AgentServiceConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting AgentServiceConfig additional options")

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

// WaitUntilDeployed waits the specified timeout for the agentserviceconfig to deploy.
func (builder *AgentServiceConfigBuilder) WaitUntilDeployed(timeout time.Duration) (*AgentServiceConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Waiting for agetserviceconfig %s to be deployed", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("The agentserviceconfig does not exist on the cluster")

		return builder, fmt.Errorf("cannot wait for non-existent agentserviceconfig to be deployed")
	}

	// Polls every retryInterval to determine if agentserviceconfig is in desired state.
	conditionIndex := -1

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			if conditionIndex < 0 {
				for index, condition := range builder.Object.Status.Conditions {
					if condition.Type == agentInstallV1Beta1.ConditionDeploymentsHealthy {
						conditionIndex = index
					}
				}
			}

			if conditionIndex < 0 {
				return false, nil
			}

			return builder.Object.Status.Conditions[conditionIndex].Status == "True", nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// PullAgentServiceConfig loads the existing agentserviceconfig into AgentServiceConfigBuilder struct.
func PullAgentServiceConfig(apiClient *clients.Settings) (*AgentServiceConfigBuilder, error) {
	glog.V(100).Infof("Pulling existing agentserviceconfig name: %s", agentServiceConfigName)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	builder := AgentServiceConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.AgentServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: agentServiceConfigName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("agentserviceconfig object %s does not exist", agentServiceConfigName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined agentserviceconfig from the cluster.
func (builder *AgentServiceConfigBuilder) Get() (*agentInstallV1Beta1.AgentServiceConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting agentserviceconfig %s",
		builder.Definition.Name)

	agentServiceConfig := &agentInstallV1Beta1.AgentServiceConfig{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, agentServiceConfig)
	if err != nil {
		return nil, err
	}

	return agentServiceConfig, nil
}

// Create generates an agentserviceconfig on the cluster.
func (builder *AgentServiceConfigBuilder) Create() (*AgentServiceConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the agentserviceconfig %s",
		builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing agentserviceconfig on the cluster.
func (builder *AgentServiceConfigBuilder) Update(force bool) (*AgentServiceConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating agentserviceconfig %s",
		builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("agentserviceconfig %s does not exist",
			builder.Definition.Name)

		return builder, fmt.Errorf("cannot update non-existent agentserviceconfig")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("agentserviceconfig", builder.Definition.Name))

			err = builder.DeleteAndWait(time.Second * 5)
			builder.Definition.ResourceVersion = ""
			builder.Definition.CreationTimestamp = metav1.Time{}

			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("agentserviceconfig", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes an agentserviceconfig from the cluster.
func (builder *AgentServiceConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the agentserviceconfig %s",
		builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("agentserviceconfig %s does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete agentserviceconfig: %w", err)
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""

	return nil
}

// DeleteAndWait deletes an agentserviceconfig and waits until it is removed from the cluster.
func (builder *AgentServiceConfigBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(`Deleting agentserviceconfig %s and 
	waiting for the defined period until it is removed`,
		builder.Definition.Name)

	if err := builder.Delete(); err != nil {
		return err
	}

	// Polls the agentserviceconfig every second until it is removed.
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})
}

// Exists checks if the defined agentserviceconfig has already been created.
func (builder *AgentServiceConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if agentserviceconfig %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// GetDefaultStorageSpec returns a default PVC spec for the respective
// agentserviceconfig component's storage and a possible error.
func GetDefaultStorageSpec(defaultStorageSize string) (corev1.PersistentVolumeClaimSpec, error) {
	checkedDefaultStorageSize, err := resource.ParseQuantity(defaultStorageSize)
	if err != nil {
		return corev1.PersistentVolumeClaimSpec{}, fmt.Errorf("the storage size is in wrong format")
	}

	defaultSpec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			"ReadWriteOnce",
		},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: checkedDefaultStorageSize,
			},
		},
	}

	glog.V(100).Infof("Getting default PVC spec: %v", defaultSpec)

	return defaultSpec, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *AgentServiceConfigBuilder) validate() (bool, error) {
	resourceCRD := "AgentServiceConfig"

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
