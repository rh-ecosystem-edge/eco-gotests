package olm

import (
	"context"
	"fmt"

	operatorsV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// InstallPlanBuilder provides a struct for installplan object from the cluster and an installplan definition.
type InstallPlanBuilder struct {
	// Installplan definition, used to create the installplan object.
	Definition *operatorsV1alpha1.InstallPlan
	// Created installplan object.
	Object *operatorsV1alpha1.InstallPlan
	// Used in functions that define or mutate installplan definition. errorMsg is processed
	// before the installplan object is created
	errorMsg string
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
}

// NewInstallPlanBuilder creates new instance of InstallPlanBuilder.
func NewInstallPlanBuilder(apiClient *clients.Settings, name, nsname string) *InstallPlanBuilder {
	klog.V(100).Infof("Initializing new %s installplan structure", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(operatorsV1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add operatorsV1alpha1 scheme to client schemes")

		return nil
	}

	builder := &InstallPlanBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorsV1alpha1.InstallPlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the installplan is empty")

		builder.errorMsg = "installplan 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the installplan is empty")

		builder.errorMsg = "installplan 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullInstallPlan loads existing InstallPlan from cluster into the InstallPlanBuilder struct.
func PullInstallPlan(apiClient *clients.Settings, name, nsName string) (*InstallPlanBuilder, error) {
	klog.V(100).Infof("Pulling existing InstallPlan %s from cluster in namespace %s", name, nsName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("installPlan 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(operatorsV1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add operatorsV1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &InstallPlanBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorsV1alpha1.InstallPlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsName,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the InstallPlan is empty")

		return nil, fmt.Errorf("installPlan 'name' cannot be empty")
	}

	if nsName == "" {
		klog.V(100).Info("The namespace of the InstallPlan is empty")

		return nil, fmt.Errorf("installPlan 'nsName' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"installPlan object named %s does not exist in namespace %s", name, nsName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns InstallPlan object if found.
func (builder *InstallPlanBuilder) Get() (*operatorsV1alpha1.InstallPlan, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting InstallPlan object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	installPlan := &operatorsV1alpha1.InstallPlan{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		installPlan)
	if err != nil {
		klog.V(100).Infof(
			"InstallPlan object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return installPlan, nil
}

// Create makes an InstallPlanBuilder in cluster and stores the created object in struct.
func (builder *InstallPlanBuilder) Create() (*InstallPlanBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the InstallPlan %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

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

// Exists checks whether the given installplan exists.
func (builder *InstallPlanBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if installplan %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes an installplan.
func (builder *InstallPlanBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting installplan %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("InstallPlan object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

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

// Update modifies the existing InstallPlanBuilder with the InstallPlan definition in InstallPlanBuilder.
func (builder *InstallPlanBuilder) Update() (*InstallPlanBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating installPlan %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("installPlan named %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *InstallPlanBuilder) validate() (bool, error) {
	resourceCRD := "installplan"

	if builder == nil {
		klog.V(100).Infof("The builder %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The builder %s apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The builder %s has error message: %v", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
