package keda

import (
	"fmt"

	kedav2v1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TriggerAuthenticationBuilder provides a struct for TriggerAuthentication object from the cluster
// and a TriggerAuthentication definition.
type TriggerAuthenticationBuilder struct {
	// TriggerAuthentication definition, used to create the TriggerAuthentication object.
	Definition *kedav2v1alpha1.TriggerAuthentication
	// Created TriggerAuthentication object.
	Object *kedav2v1alpha1.TriggerAuthentication
	// Used to store latest error message upon defining or mutating TriggerAuthentication definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewTriggerAuthenticationBuilder creates a new instance of TriggerAuthenticationBuilder.
func NewTriggerAuthenticationBuilder(
	apiClient *clients.Settings, name, nsname string) *TriggerAuthenticationBuilder {
	klog.V(100).Infof(
		"Initializing new triggerAuthentication structure with the following params: "+
			"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("triggerAuthentication 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(kedav2v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add kedav2v1alpha1 scheme to client schemes")

		return nil
	}

	builder := &TriggerAuthenticationBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav2v1alpha1.TriggerAuthentication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the triggerAuthentication is empty")

		builder.errorMsg = "triggerAuthentication 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the triggerAuthentication is empty")

		builder.errorMsg = "triggerAuthentication 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullTriggerAuthentication pulls existing triggerAuthentication from cluster.
func PullTriggerAuthentication(apiClient *clients.Settings,
	name, nsname string) (*TriggerAuthenticationBuilder, error) {
	klog.V(100).Infof("Pulling existing triggerAuthentication name %s in namespace %s from cluster",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("triggerAuthentication 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(kedav2v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add kedav2v1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := TriggerAuthenticationBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav2v1alpha1.TriggerAuthentication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the triggerAuthentication is empty")

		return nil, fmt.Errorf("triggerAuthentication 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the triggerAuthentication is empty")

		return nil, fmt.Errorf("triggerAuthentication 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("triggerAuthentication object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined triggerAuthentication from the cluster.
func (builder *TriggerAuthenticationBuilder) Get() (*kedav2v1alpha1.TriggerAuthentication, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting triggerAuthentication %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	triggerAuthenticationObj := &kedav2v1alpha1.TriggerAuthentication{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, triggerAuthenticationObj)
	if err != nil {
		return nil, err
	}

	return triggerAuthenticationObj, nil
}

// Create makes a triggerAuthentication in the cluster and stores the created object in struct.
func (builder *TriggerAuthenticationBuilder) Create() (*TriggerAuthenticationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the triggerAuthentication %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes triggerAuthentication from a cluster.
func (builder *TriggerAuthenticationBuilder) Delete() (*TriggerAuthenticationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the triggerAuthentication %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("triggerAuthentication %s in namespace %s cannot be deleted"+
			" because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete triggerAuthentication: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given triggerAuthentication exists.
func (builder *TriggerAuthenticationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if triggerAuthentication %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing triggerAuthentication object with triggerAuthentication definition in builder.
func (builder *TriggerAuthenticationBuilder) Update() (*TriggerAuthenticationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating triggerAuthentication %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("triggerAuthentication", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithSecretTargetRef sets the triggerAuthentication operator's secretTargetRef.
func (builder *TriggerAuthenticationBuilder) WithSecretTargetRef(
	secretTargetRef []kedav2v1alpha1.AuthSecretTargetRef) *TriggerAuthenticationBuilder {
	klog.V(100).Infof(
		"Adding secretTargetRef to triggerAuthentication %s in namespace %s; secretTargetRef %v",
		builder.Definition.Name, builder.Definition.Namespace, secretTargetRef)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(secretTargetRef) == 0 {
		klog.V(100).Info("'secretTargetRef' argument cannot be empty")

		builder.errorMsg = "'secretTargetRef' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.SecretTargetRef = secretTargetRef

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *TriggerAuthenticationBuilder) validate() (bool, error) {
	resourceCRD := "TriggerAuthentication"

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
