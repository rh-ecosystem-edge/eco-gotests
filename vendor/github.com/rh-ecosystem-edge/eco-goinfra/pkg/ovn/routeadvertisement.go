package ovn

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	ovnv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/routeadvertisement/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/types"
)

// RouteAdvertisementBuilder provides a wrapper around RouteAdvertisements objects for the Kubernetes API.
type RouteAdvertisementBuilder struct {
	// RouteAdvertisement definition, used to create the RouteAdvertisement object.
	Definition *ovnv1.RouteAdvertisements
	// Created RouteAdvertisement object.
	Object *ovnv1.RouteAdvertisements
	// api client to interact with the kubernetes cluster.
	apiClient client.Client
	// Used to store latest error message upon defining or mutating RouteAdvertisement definition.
	errorMsg string
}

// NewRouteAdvertisementBuilder creates a new instance of RouteAdvertisementBuilder.
func NewRouteAdvertisementBuilder(
	apiClient client.Client,
	name string,
	advertisements []ovnv1.AdvertisementType,
	nodeSelector metav1.LabelSelector,
	frrConfigurationSelector metav1.LabelSelector,
	networkSelectors types.NetworkSelectors) *RouteAdvertisementBuilder {
	glog.V(100).Infof(
		"Initializing new RouteAdvertisement structure with the following params: "+
			"name: %s, advertisements: %v, nodeSelector: %v, frrConfigurationSelector: %v, networkSelectors: %v",
		name, advertisements, nodeSelector, frrConfigurationSelector, networkSelectors)

	if apiClient == nil {
		glog.V(100).Infof("RouteAdvertisement 'apiClient' cannot be nil")

		return nil
	}

	builder := &RouteAdvertisementBuilder{
		apiClient: apiClient,
		Definition: &ovnv1.RouteAdvertisements{
			TypeMeta: metav1.TypeMeta{
				Kind:       "RouteAdvertisements",
				APIVersion: fmt.Sprintf("%s/%s", ovnv1.GroupName, ovnv1.SchemeGroupVersion.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: ovnv1.RouteAdvertisementsSpec{
				Advertisements:           advertisements,
				NodeSelector:             nodeSelector,
				FRRConfigurationSelector: frrConfigurationSelector,
				NetworkSelectors:         networkSelectors,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the RouteAdvertisement is empty")

		builder.errorMsg = "RouteAdvertisement 'name' cannot be empty"

		return builder
	}

	if len(advertisements) == 0 {
		glog.V(100).Infof("RouteAdvertisement 'advertisements' cannot be empty")

		builder.errorMsg = "RouteAdvertisement 'advertisements' cannot be empty"

		return builder
	}

	return builder
}

// PullRouteAdvertisement pulls existing RouteAdvertisement from cluster.
func PullRouteAdvertisement(apiClient client.Client, name string) (*RouteAdvertisementBuilder, error) {
	glog.V(100).Infof("Pulling existing RouteAdvertisement name %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("RouteAdvertisement 'apiClient' cannot be nil")
	}

	builder := RouteAdvertisementBuilder{
		apiClient: apiClient,
		Definition: &ovnv1.RouteAdvertisements{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the RouteAdvertisement is empty")

		return nil, fmt.Errorf("RouteAdvertisement 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("RouteAdvertisement object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns RouteAdvertisement object if found.
func (builder *RouteAdvertisementBuilder) Get() (*ovnv1.RouteAdvertisements, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting RouteAdvertisement %s", builder.Definition.Name)

	routeAdvertisement := &ovnv1.RouteAdvertisements{}
	err := builder.apiClient.Get(context.TODO(), client.ObjectKey{
		Name: builder.Definition.Name,
	}, routeAdvertisement)

	if err != nil {
		glog.V(100).Infof("RouteAdvertisement object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return routeAdvertisement, err
}

// Create makes a RouteAdvertisement in the cluster and stores the created object in struct.
func (builder *RouteAdvertisementBuilder) Create() (*RouteAdvertisementBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the RouteAdvertisement %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes RouteAdvertisement object from a cluster.
func (builder *RouteAdvertisementBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the RouteAdvertisement %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("RouteAdvertisement %s cannot be deleted because it does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)

	if err != nil {
		return fmt.Errorf("can not delete RouteAdvertisement: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given RouteAdvertisement exists.
func (builder *RouteAdvertisementBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if RouteAdvertisement %s exists", builder.Definition.Name)

	var err error
	builder.Object, err = builder.Get()

	return err == nil || !errors.IsNotFound(err)
}

// Update renovates the existing RouteAdvertisement object with RouteAdvertisement definition in builder.
func (builder *RouteAdvertisementBuilder) Update() (*RouteAdvertisementBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the RouteAdvertisement object %s", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)

	if err != nil {
		glog.V(100).Infof(
			msg.FailToUpdateNotification("RouteAdvertisement", builder.Definition.Name))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, err
}

// WithTargetVRF sets the targetVRF field in the RouteAdvertisement definition.
func (builder *RouteAdvertisementBuilder) WithTargetVRF(targetVRF string) *RouteAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RouteAdvertisement %s targetVRF to %s", builder.Definition.Name, targetVRF)

	builder.Definition.Spec.TargetVRF = targetVRF

	return builder
}

// WithAdvertisements sets the advertisements field in the RouteAdvertisement definition.
func (builder *RouteAdvertisementBuilder) WithAdvertisements(advertisements []ovnv1.AdvertisementType) *RouteAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RouteAdvertisement %s advertisements to %v", builder.Definition.Name, advertisements)

	if len(advertisements) == 0 {
		glog.V(100).Infof("RouteAdvertisement 'advertisements' cannot be empty")

		builder.errorMsg = "RouteAdvertisement 'advertisements' cannot be empty"

		return builder
	}

	builder.Definition.Spec.Advertisements = advertisements

	return builder
}

// WithNodeSelector sets the nodeSelector field in the RouteAdvertisement definition.
func (builder *RouteAdvertisementBuilder) WithNodeSelector(nodeSelector metav1.LabelSelector) *RouteAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RouteAdvertisement %s nodeSelector to %v", builder.Definition.Name, nodeSelector)

	builder.Definition.Spec.NodeSelector = nodeSelector

	return builder
}

// WithFRRConfigurationSelector sets the frrConfigurationSelector field in the RouteAdvertisement definition.
func (builder *RouteAdvertisementBuilder) WithFRRConfigurationSelector(frrConfigurationSelector metav1.LabelSelector) *RouteAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RouteAdvertisement %s frrConfigurationSelector to %v", builder.Definition.Name, frrConfigurationSelector)

	builder.Definition.Spec.FRRConfigurationSelector = frrConfigurationSelector

	return builder
}

// WithNetworkSelectors sets the networkSelectors field in the RouteAdvertisement definition.
func (builder *RouteAdvertisementBuilder) WithNetworkSelectors(networkSelectors types.NetworkSelectors) *RouteAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RouteAdvertisement %s networkSelectors to %v", builder.Definition.Name, networkSelectors)

	builder.Definition.Spec.NetworkSelectors = networkSelectors

	return builder
}

// GetRouteAdvertisementGVR returns RouteAdvertisement's GroupVersionResource which could be used for Clean function.
func GetRouteAdvertisementGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: ovnv1.GroupName, Version: ovnv1.SchemeGroupVersion.Version, Resource: "routeadvertisements",
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *RouteAdvertisementBuilder) validate() (bool, error) {
	resourceCRD := "RouteAdvertisement"

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

	if builder.Definition.Name == "" {
		glog.V(100).Infof("The %s name is empty", resourceCRD)

		return false, fmt.Errorf("%s 'name' cannot be empty", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
