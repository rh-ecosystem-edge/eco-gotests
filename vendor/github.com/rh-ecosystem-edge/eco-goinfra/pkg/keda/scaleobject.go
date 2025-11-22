package keda

import (
	"context"
	"fmt"

	kedav2v1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ScaledObjectBuilder provides a struct for ScaledObject object from the cluster
// and a ScaledObject definition.
type ScaledObjectBuilder struct {
	// ScaledObject definition, used to create the ScaledObject object.
	Definition *kedav2v1alpha1.ScaledObject
	// Created ScaledObject object.
	Object *kedav2v1alpha1.ScaledObject
	// Used to store latest error message upon defining or mutating ScaledObject definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewScaledObjectBuilder creates a new instance of ScaledObjectBuilder.
func NewScaledObjectBuilder(
	apiClient *clients.Settings, name, nsname string) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Initializing new scaledObject structure with the following params: "+
			"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("scaledObject 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(kedav2v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add kedav2v1alpha1 scheme to client schemes")

		return nil
	}

	builder := &ScaledObjectBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav2v1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the scaledObject is empty")

		builder.errorMsg = "scaledObject 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the scaledObject is empty")

		builder.errorMsg = "scaledObject 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullScaledObject pulls existing scaledObject from cluster.
func PullScaledObject(apiClient *clients.Settings, name, nsname string) (*ScaledObjectBuilder, error) {
	klog.V(100).Infof("Pulling existing scaledObject name %s in namespace %s from cluster",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("scaledObject 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(kedav2v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add kedav2v1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := ScaledObjectBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav2v1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the scaledObject is empty")

		return nil, fmt.Errorf("scaledObject 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the scaledObject is empty")

		return nil, fmt.Errorf("scaledObject 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("scaledObject object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined scaledObject from the cluster.
func (builder *ScaledObjectBuilder) Get() (*kedav2v1alpha1.ScaledObject, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting scaledObject %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	scaleObjectObj := &kedav2v1alpha1.ScaledObject{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, scaleObjectObj)
	if err != nil {
		return nil, err
	}

	return scaleObjectObj, nil
}

// Create makes a scaledObject in the cluster and stores the created object in struct.
func (builder *ScaledObjectBuilder) Create() (*ScaledObjectBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the scaledObject %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes scaledObject from a cluster.
func (builder *ScaledObjectBuilder) Delete() (*ScaledObjectBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the scaledObject %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("scaledObject %s in namespace %s cannot be deleted"+
			" because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete scaledObject: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given scaledObject exists.
func (builder *ScaledObjectBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if scaledObject %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing scaledObject object with scaledObject definition in builder.
func (builder *ScaledObjectBuilder) Update() (*ScaledObjectBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating scaledObject %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("scaledObject", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithTriggers sets the scaledObject operator's maxReplicaCount.
func (builder *ScaledObjectBuilder) WithTriggers(
	triggers []kedav2v1alpha1.ScaleTriggers) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding triggers to scaledObject %s in namespace %s; triggers %v",
		builder.Definition.Name, builder.Definition.Namespace, triggers)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(triggers) == 0 {
		klog.V(100).Info("'triggers' argument cannot be empty")

		builder.errorMsg = "'triggers' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Triggers = triggers

	return builder
}

// WithMaxReplicaCount sets the scaledObject operator's maxReplicaCount.
func (builder *ScaledObjectBuilder) WithMaxReplicaCount(
	maxReplicaCount int32) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding maxReplicaCount to scaledObject %s in namespace %s; maxReplicaCount %v",
		builder.Definition.Name, builder.Definition.Namespace, maxReplicaCount)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.MaxReplicaCount = &maxReplicaCount

	return builder
}

// WithMinReplicaCount sets the scaledObject operator's minReplicaCount.
func (builder *ScaledObjectBuilder) WithMinReplicaCount(
	minReplicaCount int32) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding minReplicaCount to scaledObject %s in namespace %s; minReplicaCount %v",
		builder.Definition.Name, builder.Definition.Namespace, minReplicaCount)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.MinReplicaCount = &minReplicaCount

	return builder
}

// WithCooldownPeriod sets the scaledObject operator's cooldownPeriod.
func (builder *ScaledObjectBuilder) WithCooldownPeriod(
	cooldownPeriod int32) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding cooldownPeriod to scaledObject %s in namespace %s; cooldownPeriod %v",
		builder.Definition.Name, builder.Definition.Namespace, cooldownPeriod)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.CooldownPeriod = &cooldownPeriod

	return builder
}

// WithPollingInterval sets the scaledObject operator's pollingInterval.
func (builder *ScaledObjectBuilder) WithPollingInterval(
	pollingInterval int32) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding pollingInterval to scaledObject %s in namespace %s; pollingInterval %v",
		builder.Definition.Name, builder.Definition.Namespace, pollingInterval)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.PollingInterval = &pollingInterval

	return builder
}

// WithScaleTargetRef sets the scaledObject operator's scaleTargetRef.
func (builder *ScaledObjectBuilder) WithScaleTargetRef(
	scaleTargetRef kedav2v1alpha1.ScaleTarget) *ScaledObjectBuilder {
	klog.V(100).Infof(
		"Adding scaleTargetRef to scaledObject %s in namespace %s; scaleTargetRef %v",
		builder.Definition.Name, builder.Definition.Namespace, scaleTargetRef)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.ScaleTargetRef = &scaleTargetRef

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ScaledObjectBuilder) validate() (bool, error) {
	resourceCRD := "ScaledObject"

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
