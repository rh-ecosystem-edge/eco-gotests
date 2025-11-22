package console

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for console object from the cluster and a console definition.
type Builder struct {
	// Console definition, used to create the pod object.
	Definition *configv1.Console
	// Created console object.
	Object *configv1.Console
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg is processed before console object is created.
	errorMsg string
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name string) *Builder {
	klog.V(100).Infof("Initializing new console %s structure", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the Console is nil")

		return nil
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		klog.V(100).Infof("Failed to add config v1 scheme to client schemes: %v", err)

		return nil
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &configv1.Console{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the Console is empty")

		builder.errorMsg = "console 'name' cannot be empty"

		return builder
	}

	return builder
}

// Pull loads an existing console into the Builder struct.
func Pull(apiClient *clients.Settings, name string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing Console %s from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the Console is nil")

		return nil, fmt.Errorf("console 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		klog.V(100).Infof("Failed to add config v1 scheme to client schemes: %v", err)

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &configv1.Console{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the Console is empty")

		return builder, fmt.Errorf("console 'name' cannot be empty")
	}

	klog.V(100).Infof("Pulling cluster console %s", name)

	if !builder.Exists() {
		klog.V(100).Infof("The Console %s does not exist", name)

		return nil, fmt.Errorf("console object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the Console object if found.
func (builder *Builder) Get() (*configv1.Console, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Console object %s", builder.Definition.Name)

	console := &configv1.Console{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name: builder.Definition.Name,
	}, console)
	if err != nil {
		klog.V(100).Infof("Failed to get Console object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return console, nil
}

// Create makes a console in the cluster if it does not already exist.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Creating Console %s", builder.Definition.Name)

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

// Exists checks whether the given console exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if console %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a console object from a cluster.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the console object %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("Console %s does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
	if err != nil {
		return fmt.Errorf("cannot delete console: %w", err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing cluster console object with cluster console definition in builder.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Updating cluster console %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("Console %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("cannot update non-existent console")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "console"

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
		klog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
