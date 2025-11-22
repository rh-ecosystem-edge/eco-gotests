package nodesconfig

import (
	"context"
	"fmt"

	configV1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for nodesConfig object from the cluster and a nodesConfig definition.
type Builder struct {
	// nodesConfig definition, used to create the nodesConfig object.
	Definition *configV1.Node
	// Created nodesConfig object.
	Object *configV1.Node
	// api client to interact with the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate clusterOperator definition. errorMsg is processed before the
	// ClusterOperator object is created.
	errorMsg string
}

// Pull retrieves an existing nodesConfig object from the cluster.
func Pull(apiClient *clients.Settings, nodesConfigObjName string) (*Builder, error) {
	klog.V(100).Infof("Pulling nodesConfig object name: %s", nodesConfigObjName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("nodesConfig Config 'apiClient' cannot be empty")
	}

	if nodesConfigObjName == "" {
		klog.V(100).Info("The name of the nodesConfig is empty")

		return nil, fmt.Errorf("nodesConfig 'nodesConfigObjName' cannot be empty")
	}

	builder := Builder{
		apiClient: apiClient.Client,
		Definition: &configV1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodesConfigObjName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("nodesConfig object %s does not exist", nodesConfigObjName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches existing nodesConfig from cluster.
func (builder *Builder) Get() (*configV1.Node, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting existing nodesConfig with name %s from cluster", builder.Definition.Name)

	nodesConfig := &configV1.Node{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, nodesConfig)
	if err != nil {
		klog.V(100).Infof("Failed to get nodesConfig object %s from cluster due to: %v",
			builder.Definition.Name, err)

		return nil, err
	}

	return nodesConfig, nil
}

// Exists checks whether the given nodesConfig exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if nodesConfig %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the nodesConfig in the cluster and stores the created object in struct.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the nodesConfig %s", builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("nodesConfig object %s does not exist", builder.Definition.Name)
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// GetCGroupMode fetches nodesConfig cgroupMode.
func (builder *Builder) GetCGroupMode() (configV1.CgroupMode, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	klog.V(100).Info("Getting nodesConfig cGroupMode configuration")

	if !builder.Exists() {
		return "", fmt.Errorf("nodesConfig object does not exist")
	}

	return builder.Object.Spec.CgroupMode, nil
}

// WithCGroupMode sets the nodesConfig operator's cgroupMode.
func (builder *Builder) WithCGroupMode(expectedCGroupMode configV1.CgroupMode) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting nodesConfig %s with cGroupMode: %v",
		builder.Definition.Name, expectedCGroupMode)

	if expectedCGroupMode == configV1.CgroupModeEmpty {
		klog.V(100).Info("the cGroupMode value can not be empty")

		builder.errorMsg = "the cGroupMode value can not be empty"

		return builder
	}

	builder.Definition.Spec.CgroupMode = expectedCGroupMode

	return builder
}

// GetNodesConfigIoGVR returns nodesConfig's GroupVersionResource which could be used for Clean function.
func GetNodesConfigIoGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: APIKind,
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "Nodes.Config"

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
