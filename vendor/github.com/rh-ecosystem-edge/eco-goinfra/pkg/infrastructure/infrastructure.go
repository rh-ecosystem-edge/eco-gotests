package infrastructure

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	infrastructureName = "cluster"
)

// Builder provides a struct for infrastructure object from the cluster and a infrastructure definition.
type Builder struct {
	// infrastructure definition, used to create the infrastructure object.
	Definition *configv1.Infrastructure
	// Created infrastructure object.
	Object *configv1.Infrastructure
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// Pull loads an existing infrastructure into Builder struct.
func Pull(apiClient *clients.Settings) (*Builder, error) {
	klog.V(100).Infof("Pulling existing infrastructure name: %s", infrastructureName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the Infrastructure is nil")

		return nil, fmt.Errorf("infrastructure 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add config v1 scheme to client schemes")

		return nil, err
	}

	builder := Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name: infrastructureName,
			},
		},
	}

	if !builder.Exists() {
		klog.V(100).Infof("The Infrastructure %s does not exist", infrastructureName)

		return nil, fmt.Errorf("infrastructure object %s does not exist", infrastructureName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns the Infrastructure object if found.
func (builder *Builder) Get() (*configv1.Infrastructure, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Infrastructure object %s", builder.Definition.Name)

	infrastructure := &configv1.Infrastructure{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{Name: builder.Definition.Name}, infrastructure)
	if err != nil {
		klog.V(100).Infof("Failed to get Infrastructure object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return infrastructure, nil
}

// Exists checks whether the given infrastructure exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if infrastructure %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "Infrastructure"

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
