package dns

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const clusterDNSName = "cluster"

// Builder provides a struct for the cluster DNS object and a DNS definition.
type Builder struct {
	// DNS definition, used to create the DNS object.
	Definition *configv1.DNS
	// Created DNS object.
	Object *configv1.DNS
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// Used in functions that define or mutate DNS definition. errorMsg is processed before the
	// DNS object is created.
	errorMsg string
}

// Pull loads an existing DNS into Builder struct.
func Pull(apiClient *clients.Settings) (*Builder, error) {
	glog.V(100).Infof("Pulling existing DNS name: %s", clusterDNSName)

	if apiClient == nil {
		glog.V(100).Info("The apiClient of the DNS is nil")

		return nil, fmt.Errorf("dns 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add config v1 scheme to client schemes")

		return nil, err
	}

	builder := Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.DNS{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterDNSName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("dns object %s does not exist", clusterDNSName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns the DNS object from the cluster if it exists.
func (builder *Builder) Get() (*configv1.DNS, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting DNS object %s", builder.Definition.Name)

	dnsObject := &configv1.DNS{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, dnsObject)
	if err != nil {
		glog.V(100).Infof("Failed to get DNS %s: %s", builder.Definition.Name, err)

		return nil, err
	}

	return dnsObject, nil
}

// Exists checks whether the given DNS exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if DNS %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing DNS object with the DNS definition in builder.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating DNS %s", builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("dns object %s does not exist", builder.Definition.Name)
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion
	builder.Definition.CreationTimestamp = metav1.Time{}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		glog.V(100).Infof("Failed to update DNS %s: %s", builder.Definition.Name, err)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "dnses.config.openshift.io"

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
