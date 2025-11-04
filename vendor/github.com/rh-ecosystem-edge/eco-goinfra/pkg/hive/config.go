package hive

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigBuilder provides struct for the HiveConfig object containing connection to
// the cluster and the HiveConfig definitions.
type ConfigBuilder struct {
	Definition *hiveV1.HiveConfig
	Object     *hiveV1.HiveConfig
	errorMsg   string
	apiClient  runtimeClient.Client
}

// ConfigAdditionalOptions additional options for HiveConfig object.
type ConfigAdditionalOptions func(builder *ConfigBuilder) (*ConfigBuilder, error)

// NewConfigBuilder creates a new instance of ConfigBuilder.
func NewConfigBuilder(apiClient *clients.Settings, name string) *ConfigBuilder {
	glog.V(100).Infof(
		`Initializing new HiveConfig structure with the following params: name: %s`, name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add hive v1 scheme to client schemes")

		return nil
	}

	builder := &ConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.HiveConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: hiveV1.HiveConfigSpec{},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the HiveConfig is empty")

		builder.errorMsg = "hiveconfig 'name' cannot be empty"

		return builder
	}

	return builder
}

// PullConfig loads an existing HiveConfig into ConfigBuilder struct.
func PullConfig(apiClient *clients.Settings, name string) (*ConfigBuilder, error) {
	glog.V(100).Infof("Pulling existing HiveConfig name: %s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("hiveconfig 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add hive v1 scheme to client schemes")

		return nil, err
	}

	builder := &ConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.HiveConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the hiveconfig is empty")

		return nil, fmt.Errorf("hiveconfig 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("hiveconfig object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches the defined HiveConfig from the cluster.
func (builder *ConfigBuilder) Get() (*hiveV1.HiveConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting HiveConfig %s", builder.Definition.Name)

	HiveConfig := &hiveV1.HiveConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeClient.ObjectKey{
		Name: builder.Definition.Name,
	}, HiveConfig)
	if err != nil {
		return nil, err
	}

	return HiveConfig, err
}

// Update modifies an existing HiveConfig on the cluster.
func (builder *ConfigBuilder) Update() (*ConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating HiveConfig %s", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	builder.Object = builder.Definition

	return builder, err
}

// Delete removes a HiveConfig from the cluster.
func (builder *ConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the HiveConfig %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("hiveconfig cannot be deleted because it does not exist")

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete hiveconfig: %w", err)
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""
	builder.Definition.CreationTimestamp = metav1.Time{}

	return nil
}

// Exists checks if the defined HiveConfig has already been created.
func (builder *ConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if hiveconfig %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithOptions creates ClusterDeployment with generic mutation options.
func (builder *ConfigBuilder) WithOptions(options ...ConfigAdditionalOptions) *ConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting HiveConfig additional options")

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ConfigBuilder) validate() (bool, error) {
	resourceCRD := "HiveConfig"

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
