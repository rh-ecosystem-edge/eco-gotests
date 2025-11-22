package olm

import (
	"fmt"

	operatorsv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OperatorGroupBuilder provides a struct for OperatorGroup object containing connection to the
// cluster and the OperatorGroup definition.
type OperatorGroupBuilder struct {
	// OperatorGroup definition. Used to create OperatorGroup object with minimum set of required elements.
	Definition *operatorsv1.OperatorGroup
	// Created OperatorGroup object on the cluster.
	Object *operatorsv1.OperatorGroup
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	// errorMsg is processed before OperatorGroup object is created.
	errorMsg string
}

// NewOperatorGroupBuilder returns an OperatorGroupBuilder struct.
func NewOperatorGroupBuilder(apiClient *clients.Settings, groupName, nsName string) *OperatorGroupBuilder {
	klog.V(100).Infof(
		"Initializing new OperatorGroupBuilder structure with the following params: %s, %s", groupName, nsName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(operatorsv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add operatorsv1 scheme to client schemes")

		return nil
	}

	builder := &OperatorGroupBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:         groupName,
				Namespace:    nsName,
				GenerateName: fmt.Sprintf("%v-", groupName),
			},
			Spec: operatorsv1.OperatorGroupSpec{
				TargetNamespaces: []string{nsName},
			},
		},
	}

	if groupName == "" {
		klog.V(100).Info("The Name of the OperatorGroup is empty")

		builder.errorMsg = "operatorGroup 'groupName' cannot be empty"

		return builder
	}

	if nsName == "" {
		klog.V(100).Info("The Namespace of the OperatorGroup is empty")

		builder.errorMsg = "operatorGroup 'Namespace' cannot be empty"

		return builder
	}

	return builder
}

// Get returns OperatorGroup object if found.
func (builder *OperatorGroupBuilder) Get() (*operatorsv1.OperatorGroup, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting operatorGroup object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	operatorGroup := &operatorsv1.OperatorGroup{}

	err := builder.apiClient.Get(logging.DiscardContext(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		operatorGroup)
	if err != nil {
		klog.V(100).Infof(
			"OperatorGroup object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return operatorGroup, nil
}

// Create makes an OperatorGroup in cluster and stores the created object in struct.
func (builder *OperatorGroupBuilder) Create() (*OperatorGroupBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the OperatorGroup %s", builder.Definition.Name)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Exists checks whether the given OperatorGroup exists.
func (builder *OperatorGroupBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if OperatorGroup %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes an OperatorGroup.
func (builder *OperatorGroupBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting OperatorGroup %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("OperatorGroup %s namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Update modifies the existing OperatorGroup with the OperatorGroup definition in OperatorGroupBuilder.
func (builder *OperatorGroupBuilder) Update() (*OperatorGroupBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating OperatorGroup %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot update non-existent operatorgroup")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// PullOperatorGroup loads existing OperatorGroup from cluster into the OperatorGroupBuilder struct.
func PullOperatorGroup(apiClient *clients.Settings, groupName, nsName string) (*OperatorGroupBuilder, error) {
	klog.V(100).Infof("Pulling existing OperatorGroup %s from cluster in namespace %s",
		groupName, nsName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("operatorGroup 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(operatorsv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add operatorsv1 scheme to client schemes")

		return nil, err
	}

	builder := &OperatorGroupBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:         groupName,
				Namespace:    nsName,
				GenerateName: fmt.Sprintf("%v-", groupName),
			},
		},
	}

	if groupName == "" {
		klog.V(100).Info("The name of the OperatorGroup is empty")

		return nil, fmt.Errorf("operatorGroup 'Name' cannot be empty")
	}

	if nsName == "" {
		klog.V(100).Info("The namespace of the OperatorGroup is empty")

		return nil, fmt.Errorf("operatorGroup 'Namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("operatorGroup object named %s does not exist", nsName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *OperatorGroupBuilder) validate() (bool, error) {
	resourceCRD := "OperatorGroup"

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
