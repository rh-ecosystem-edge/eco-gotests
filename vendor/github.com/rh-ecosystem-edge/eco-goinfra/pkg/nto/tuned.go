package nto //nolint:misspell

import (
	"fmt"

	tunedv1 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/tuned/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TunedBuilder provides a struct for Tuned object from the cluster and a Tuned definition.
type TunedBuilder struct {
	// Tuned definition, used to create the Tuned object.
	Definition *tunedv1.Tuned
	// Created Tuned object.
	Object *tunedv1.Tuned
	// Used to store latest error message upon defining or mutating Tuned definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewTunedBuilder creates a new instance of TunedBuilder.
func NewTunedBuilder(
	apiClient *clients.Settings, name, nsname string) *TunedBuilder {
	klog.V(100).Infof(
		"Initializing new Tuned structure with the following params: "+
			"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(tunedv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add tuned v1 scheme to client schemes")

		return nil
	}

	builder := &TunedBuilder{
		apiClient: apiClient.Client,
		Definition: &tunedv1.Tuned{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the Tuned is empty")

		builder.errorMsg = "tuned 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the tuned is empty")

		builder.errorMsg = "tuned 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullTuned pulls existing Tuned from cluster.
func PullTuned(apiClient *clients.Settings, name, nsname string) (*TunedBuilder, error) {
	klog.V(100).Infof("Pulling existing Tuned name %s in namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("tuned 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(tunedv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add tuned v1 scheme to client schemes")

		return nil, err
	}

	builder := TunedBuilder{
		apiClient: apiClient.Client,
		Definition: &tunedv1.Tuned{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the tuned is empty")

		return nil, fmt.Errorf("tuned 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the tuned is empty")

		return nil, fmt.Errorf("tuned 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("tuned object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined Tuned from the cluster.
func (builder *TunedBuilder) Get() (*tunedv1.Tuned, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Tuned %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	tunedObj := &tunedv1.Tuned{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, tunedObj)
	if err != nil {
		return nil, err
	}

	return tunedObj, nil
}

// Create makes a tuned in the cluster and stores the created object in struct.
func (builder *TunedBuilder) Create() (*TunedBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the tuned %s in namespace %s",
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

// Delete removes tuned from a cluster.
func (builder *TunedBuilder) Delete() (*TunedBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the tuned %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("tuned %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete tuned: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given tuned exists.
func (builder *TunedBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if tuned %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing tuned object with tuned definition in builder.
func (builder *TunedBuilder) Update() (*TunedBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating tuned %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("tuned", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithProfile sets the tuned operator's profile.
func (builder *TunedBuilder) WithProfile(
	profile tunedv1.TunedProfile) *TunedBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting tuned %s in namespace %s with the Profile: %v",
		builder.Definition.Name, builder.Definition.Namespace, profile)

	builder.Definition.Spec.Profile = append(builder.Definition.Spec.Profile, profile)

	return builder
}

// WithRecommend sets the tuned operator's recommend.
func (builder *TunedBuilder) WithRecommend(
	recommend tunedv1.TunedRecommend) *TunedBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting tuned %s in namespace %s with the Recommend: %v",
		builder.Definition.Name, builder.Definition.Namespace, recommend)

	if builder.Definition.Spec.Recommend == nil {
		builder.Definition.Spec.Recommend = []tunedv1.TunedRecommend{}
	}

	builder.Definition.Spec.Recommend = append(builder.Definition.Spec.Recommend, recommend)

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *TunedBuilder) validate() (bool, error) {
	resourceCRD := "Tuned"

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

	return true, nil
}
