package olm

import (
	"context"
	"fmt"

	oplmV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CatalogSourceBuilder provides a struct for catalogsource object
// from the cluster and a catalogsource definition.
type CatalogSourceBuilder struct {
	// CatalogSource definition. Used to create
	// CatalogSource object with minimum set of required elements.
	Definition *oplmV1alpha1.CatalogSource
	// Created CatalogSource object on the cluster.
	Object *oplmV1alpha1.CatalogSource
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	// errorMsg is processed before CatalogSourceBuilder object is created.
	errorMsg string
}

// NewCatalogSourceBuilder creates new instance of CatalogSourceBuilder.
func NewCatalogSourceBuilder(apiClient *clients.Settings, name, nsname string) *CatalogSourceBuilder {
	glog.V(100).Infof("Initializing new %s catalogsource structure", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil
	}

	builder := &CatalogSourceBuilder{
		apiClient: apiClient.Client,
		Definition: &oplmV1alpha1.CatalogSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the catalogsource is empty")

		builder.errorMsg = "catalogsource 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The nsname of the catalogsource is empty")

		builder.errorMsg = "catalogsource 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullCatalogSource loads an existing catalogsource into Builder struct.
func PullCatalogSource(apiClient *clients.Settings, name, nsname string) (*CatalogSourceBuilder,
	error) {
	glog.V(100).Infof("Pulling existing catalogsource name %s in namespace %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("catalogsource 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &CatalogSourceBuilder{
		apiClient: apiClient.Client,
		Definition: &oplmV1alpha1.CatalogSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the catalogsource is empty")

		return nil, fmt.Errorf("catalogsource 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the catalogsource is empty")

		return nil, fmt.Errorf("catalogsource 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("catalogsource object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes an CatalogSourceBuilder in cluster and stores the created object in struct.
func (builder *CatalogSourceBuilder) Create() (*CatalogSourceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the catalogsource %s in namespace %s",
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

// Get returns CatalogSource object if found.
func (builder *CatalogSourceBuilder) Get() (*oplmV1alpha1.CatalogSource, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting CatalogSource object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	catalogSource := &oplmV1alpha1.CatalogSource{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		catalogSource)
	if err != nil {
		glog.V(100).Infof(
			"CatalogSource object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return catalogSource, nil
}

// Update renovates the existing CatalogSource object with the CatalogSource definition in builder.
func (builder *CatalogSourceBuilder) Update(force bool) (*CatalogSourceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the CatalogSource object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update CatalogSource, object does not exist on cluster")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("CatalogSource", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("CatalogSource", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, nil
}

// Exists checks whether the given catalogsource exists.
func (builder *CatalogSourceBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if catalogSource %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a catalogsource.
func (builder *CatalogSourceBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting catalogsource %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("catalogsource cannot be deleted because it does not exist")

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *CatalogSourceBuilder) validate() (bool, error) {
	resourceCRD := "catalogsource"

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
