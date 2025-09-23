package ocm

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// KlusterletName is the name of the klusterlet created by importing a ManagedCluster.
const KlusterletName = "klusterlet"

// KlusterletBuilder provides a struct to interface with Klusterlet resources on a specific cluster.
type KlusterletBuilder struct {
	// Definition of the Klusterlet used to create the resource.
	Definition *operatorv1.Klusterlet
	// Object of the Klusterlet as it is on the cluster.
	Object *operatorv1.Klusterlet
	// apiClient used to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg is used to store the error message from functions that do not return an error.
	errorMsg string
}

// NewKlusterletBuilder creates a new instance of a Klusterlet builder.
func NewKlusterletBuilder(apiClient *clients.Settings, name string) *KlusterletBuilder {
	glog.V(100).Infof("Initializing new Klusterlet structure with the following params: name: %s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient of the Klusterlet is nil")

		return nil
	}

	err := apiClient.AttachScheme(operatorv1.Install)
	if err != nil {
		glog.V(100).Infof("Failed to add ocm operator v1 scheme to client schemes: %v", err)

		return nil
	}

	builder := &KlusterletBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorv1.Klusterlet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the Klusterlet is empty")

		builder.errorMsg = "klusterlet 'name' cannot be empty"

		return builder
	}

	return builder
}

// PullKlusterlet pulls an existing Klusterlet into a Builder struct.
func PullKlusterlet(apiClient *clients.Settings, name string) (*KlusterletBuilder, error) {
	glog.V(100).Infof("Pulling existing Klusterlet %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient of the Klusterlet is nil")

		return nil, fmt.Errorf("klusterlet 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(operatorv1.Install)
	if err != nil {
		glog.V(100).Infof("Failed to add ocm operator v1 scheme to client schemes: %v", err)

		return nil, err
	}

	builder := &KlusterletBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorv1.Klusterlet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the Klusterlet is empty")

		return nil, fmt.Errorf("klusterlet 'name' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Infof("The Klusterlet %s does not exist", name)

		return nil, fmt.Errorf("klusterlet object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the Klusterlet object if found.
func (builder *KlusterletBuilder) Get() (*operatorv1.Klusterlet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting Klusterlet object %s", builder.Definition.Name)

	klusterlet := &operatorv1.Klusterlet{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, klusterlet)
	if err != nil {
		glog.V(100).Infof("Failed to get Klusterlet object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return klusterlet, nil
}

// Exists checks whether this Klusterlet exists on the cluster.
func (builder *KlusterletBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if Klusterlet %s", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a Klusterlet on the cluster if it does not already exist.
func (builder *KlusterletBuilder) Create() (*KlusterletBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Creating Klusterlet %s", builder.Definition.Name)

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

// Update changes the existing Klusterlet resource on the cluster to match the builder Definition. Deleting a Klusterlet
// is nontrivial and affects the connection to the hub cluster, so there is no force option to allow deleting and
// recreating the Klusterlet if the update fails.
func (builder *KlusterletBuilder) Update() (*KlusterletBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Updating Klusterlet %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("Klusterlet %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("cannot update non-existent klusterlet")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a Klusterlet from the cluster if it exists.
func (builder *KlusterletBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting Klusterlet %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("Klusterlet %s does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *KlusterletBuilder) validate() (bool, error) {
	resourceCRD := "klusterlet"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
