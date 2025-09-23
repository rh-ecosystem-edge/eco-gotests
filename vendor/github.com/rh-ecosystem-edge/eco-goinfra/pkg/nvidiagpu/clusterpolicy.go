package nvidiagpu

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	nvidiagpuv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/nvidiagpu/nvidiagputypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for ClusterPolicy object
// from the cluster and a ClusterPolicy definition.
type Builder struct {
	// Builder definition. Used to create
	// Builder object with minimum set of required elements.
	Definition *nvidiagpuv1.ClusterPolicy
	// Created Builder object on the cluster.
	Object *nvidiagpuv1.ClusterPolicy
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	// errorMsg is processed before Builder object is created.
	errorMsg string
}

// NewBuilderFromObjectString creates a Builder object from CSV alm-examples.
func NewBuilderFromObjectString(apiClient *clients.Settings, almExample string) *Builder {
	glog.V(100).Infof(
		"Initializing new Builder structure from almExample string")

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(nvidiagpuv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add nvidiagpuv1 scheme to client schemes")

		return nil
	}

	clusterPolicy, err := getClusterPolicyFromAlmExample(almExample)
	if err != nil {
		glog.V(100).Infof(
			"error initializing ClusterPolicy from alm-examples: %s", err.Error())

		return &Builder{
			apiClient: apiClient.Client,
			errorMsg:  fmt.Sprintf("error initializing ClusterPolicy from alm-examples: %s", err.Error()),
		}
	}

	glog.V(100).Infof(
		"Initializing new Builder structure from almExample string with clusterPolicy name: %s",
		clusterPolicy.Name)

	builder := &Builder{
		apiClient:  apiClient.Client,
		Definition: clusterPolicy,
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The ClusterPolicy object definition is nil")

		builder.errorMsg = "ClusterPolicy 'Object.Definition' is nil"

		return builder
	}

	return builder
}

// Pull loads an existing clusterPolicy into Builder struct.
func Pull(apiClient *clients.Settings, name string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing clusterPolicy name: %s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("clusterPolicy 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(nvidiagpuv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add nvidiagpuv1 scheme to client schemes")

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &nvidiagpuv1.ClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("ClusterPolicy name is empty")

		return nil, fmt.Errorf("clusterPolicy 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("ClusterPolicy object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns clusterPolicy object if found.
func (builder *Builder) Get() (*nvidiagpuv1.ClusterPolicy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting ClusterPolicy object %s", builder.Definition.Name)

	clusterPolicy := &nvidiagpuv1.ClusterPolicy{}

	err := builder.apiClient.Get(context.TODO(), runtimeClient.ObjectKey{Name: builder.Definition.Name}, clusterPolicy)
	if err != nil {
		glog.V(100).Infof(
			"ClusterPolicy object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return clusterPolicy, err
}

// Exists checks whether the given ClusterPolicy exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if ClusterPolicy %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		glog.V(100).Infof("Failed to collect ClusterPolicy object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a ClusterPolicy.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting ClusterPolicy %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("clusterpolicy cannot be deleted because it does not exist")

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete clusterpolicy: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Create makes a ClusterPolicy in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the ClusterPolicy %s", builder.Definition.Name)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Update renovates the existing ClusterPolicy object with the definition in builder.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the ClusterPolicy object named:  %s", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(msg.FailToUpdateNotification("clusterpolicy", builder.Definition.Name))

			builder, err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("clusterpolicy", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ClusterPolicy"

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

// getClusterPolicyFromAlmExample extracts the ClusterPolicy from the alm-examples block.
func getClusterPolicyFromAlmExample(almExample string) (*nvidiagpuv1.ClusterPolicy, error) {
	clusterPolicyList := &nvidiagpuv1.ClusterPolicyList{}

	if almExample == "" {
		return nil, fmt.Errorf("almExample is an empty string")
	}

	err := json.Unmarshal([]byte(almExample), &clusterPolicyList.Items)
	if err != nil {
		return nil, err
	}

	if len(clusterPolicyList.Items) == 0 {
		return nil, fmt.Errorf("failed to get alm examples")
	}

	return &clusterPolicyList.Items[0], nil
}
