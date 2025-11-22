package nrop

import (
	"context"
	"fmt"

	nropv1 "github.com/openshift-kni/numaresources-operator/api/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SchedulerBuilder provides a struct for NUMAResourcesScheduler object from the cluster
// and a NUMAResourcesScheduler definition.
type SchedulerBuilder struct {
	// NUMAResourcesScheduler definition, used to create the NUMAResourcesScheduler object.
	Definition *nropv1.NUMAResourcesScheduler
	// Created NUMAResourcesScheduler object.
	Object *nropv1.NUMAResourcesScheduler
	// Used to store latest error message upon defining or mutating NUMAResourcesScheduler definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewSchedulerBuilder creates a new instance of NUMAResourcesScheduler.
func NewSchedulerBuilder(
	apiClient *clients.Settings, name, nsname string) *SchedulerBuilder {
	klog.V(100).Infof(
		"Initializing new NUMAResourcesScheduler structure with the following name: %s in namespace %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("NUMAResourcesScheduler 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(nropv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nrop v1 scheme to client schemes")

		return nil
	}

	builder := &SchedulerBuilder{
		apiClient: apiClient.Client,
		Definition: &nropv1.NUMAResourcesScheduler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the NUMAResourcesScheduler is empty")

		builder.errorMsg = "NUMAResourcesScheduler 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the NUMAResourcesScheduler is empty")

		builder.errorMsg = "NUMAResourcesScheduler 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullScheduler pulls existing NUMAResourcesScheduler from cluster.
func PullScheduler(apiClient *clients.Settings, name, nsname string) (*SchedulerBuilder, error) {
	klog.V(100).Infof("Pulling existing NUMAResourcesScheduler %s in namespace %s from the cluster",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("NUMAResourcesScheduler 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(nropv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nrop v1 scheme to client schemes")

		return nil, err
	}

	builder := SchedulerBuilder{
		apiClient: apiClient.Client,
		Definition: &nropv1.NUMAResourcesScheduler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the NUMAResourcesScheduler is empty")

		return nil, fmt.Errorf("NUMAResourcesScheduler 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the NUMAResourcesScheduler is empty")

		return nil, fmt.Errorf("NUMAResourcesScheduler 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("NUMAResourcesScheduler object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined NUMAResourcesScheduler from the cluster.
func (builder *SchedulerBuilder) Get() (*nropv1.NUMAResourcesScheduler, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting NUMAResourcesScheduler %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nrosObj := &nropv1.NUMAResourcesScheduler{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nrosObj)
	if err != nil {
		klog.V(100).Infof("NUMAResourcesScheduler object %s not found in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return nrosObj, nil
}

// Create makes a NUMAResourcesScheduler in the cluster and stores the created object in struct.
func (builder *SchedulerBuilder) Create() (*SchedulerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the NUMAResourcesScheduler %s in namespace %s",
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

// Delete removes NUMAResourcesScheduler from a cluster.
func (builder *SchedulerBuilder) Delete() (*SchedulerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the NUMAResourcesScheduler %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("NUMAResourcesScheduler %s cannot be deleted because "+
			"it does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete NUMAResourcesScheduler %s from namespace %s due to %w",
			builder.Definition.Name, builder.Definition.Namespace, err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given NUMAResourcesScheduler exists.
func (builder *SchedulerBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if NUMAResourcesScheduler %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing NUMAResourcesScheduler object with NUMAResourcesScheduler definition in builder.
func (builder *SchedulerBuilder) Update() (*SchedulerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating NUMAResourcesScheduler %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("NUMAResourcesScheduler", builder.Definition.Name))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithImageSpec sets the NUMAResourcesScheduler operator's imageSpec.
func (builder *SchedulerBuilder) WithImageSpec(imageSpec string) *SchedulerBuilder {
	klog.V(100).Infof("Adding imageSpec to the NUMAResourcesScheduler %s in namespace %s; imageSpec: %s",
		builder.Definition.Name, builder.Definition.Namespace, imageSpec)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if imageSpec == "" {
		klog.V(100).Info("The 'NUMAResourcesScheduler' imageSpec cannot be empty")

		builder.errorMsg = "can not apply a NUMAResourcesScheduler with an empty imageSpec"

		return builder
	}

	builder.Definition.Spec.SchedulerImage = imageSpec

	return builder
}

// WithSchedulerName sets the NUMAResourcesScheduler operator's schedulerName.
func (builder *SchedulerBuilder) WithSchedulerName(schedulerName string) *SchedulerBuilder {
	klog.V(100).Infof(
		"Adding schedulerName to the NUMAResourcesScheduler %s in namespace %s; schedulerName: %s",
		builder.Definition.Name, builder.Definition.Namespace, schedulerName)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if schedulerName == "" {
		klog.V(100).Info("The 'NUMAResourcesScheduler' schedulerName cannot be empty")

		builder.errorMsg = "can not apply a NUMAResourcesScheduler with an empty schedulerName"

		return builder
	}

	builder.Definition.Spec.SchedulerName = schedulerName

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *SchedulerBuilder) validate() (bool, error) {
	resourceCRD := "NUMAResourcesScheduler"

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
