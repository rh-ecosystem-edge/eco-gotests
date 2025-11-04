package cgu

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PreCachingConfigBuilder provides a struct for the PreCachingConfig object containing a connection to the cluster and
// the PreCachingConfig definition.
type PreCachingConfigBuilder struct {
	// Definition of the PreCachingConfig used to create the object.
	Definition *v1alpha1.PreCachingConfig
	// Object of the PreCachingConfig as it is on the cluster.
	Object *v1alpha1.PreCachingConfig
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining or mutating application definition.
	errorMsg string
}

// NewPreCachingConfigBuilder creates a new instance of PreCachingConfig.
func NewPreCachingConfigBuilder(apiClient *clients.Settings, name, nsname string) *PreCachingConfigBuilder {
	glog.V(100).Infof(
		"Initializing new PreCachingConfig structure with the following params: name: %s, nsname: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Info("The apiClient for the PreCachingConfig is nil")

		return nil
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add cgu v1alpha1 scheme to client schemes")

		return nil
	}

	builder := &PreCachingConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.PreCachingConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		}}

	if name == "" {
		glog.V(100).Infof("The name of the PreCachingConfig is empty")

		builder.errorMsg = "preCachingConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the PreCachingConfig is empty")

		builder.errorMsg = "preCachingConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullPreCachingConfig pulls an existing PreCachingConfig into a PreCachingConfigBuilder struct.
func PullPreCachingConfig(apiClient *clients.Settings, name, nsname string) (*PreCachingConfigBuilder, error) {
	glog.V(100).Infof("Pulling existing PreCachingConfig %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("preCachingConfig 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add cgu v1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := PreCachingConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.PreCachingConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the PreCachingConfig is empty")

		return nil, fmt.Errorf("preCachingConfig 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the PreCachingConfig is empty")

		return nil, fmt.Errorf("preCachingConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("preCachingConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Exists checks whether the given PreCachingConfig exists on the apiClient.
func (builder *PreCachingConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if preCachingConfig %s exists in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get pulls the PreCachingConfig from the apiClient into the PreCachingConfigBuilder.
func (builder *PreCachingConfigBuilder) Get() (*v1alpha1.PreCachingConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting PreCachingConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	preCachingConfig := &v1alpha1.PreCachingConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, preCachingConfig)
	if err != nil {
		return nil, err
	}

	return preCachingConfig, nil
}

// Create makes a PreCachingConfig on the apiClient if it does not already exist.
func (builder *PreCachingConfigBuilder) Create() (*PreCachingConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Creating the PreCachingConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a PreCachingConfig from the apiClient if it exists.
func (builder *PreCachingConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Deleting the PreCachingConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Update changes the existing PreCachingConfig object on the apiClient, falling back to deleting and recreating it if
// force is set.
func (builder *PreCachingConfigBuilder) Update(force bool) (*PreCachingConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Updating the PreCachingConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(msg.FailToUpdateNotification("preCachingConfig", builder.Definition.Name))

			err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(msg.FailToUpdateError("preCachingConfig", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *PreCachingConfigBuilder) validate() (bool, error) {
	resourceCRD := "preCachingConfig"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
