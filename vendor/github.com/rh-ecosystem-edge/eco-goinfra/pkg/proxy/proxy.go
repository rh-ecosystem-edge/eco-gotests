package proxy

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterProxyName = "cluster"
)

// Builder provides a struct for proxy object from the cluster and a proxy definition.
type Builder struct {
	// proxy definition, used to create the proxy object.
	Definition *configv1.Proxy
	// Created proxy object.
	Object *configv1.Proxy
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// Pull loads an existing proxy into Builder struct.
func Pull(apiClient *clients.Settings) (*Builder, error) {
	klog.V(100).Infof("Pulling existing proxy name: %s", clusterProxyName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the Proxy is nil")

		return nil, fmt.Errorf("proxy 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add config v1 scheme to client schemes")

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.Proxy{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterProxyName,
			},
		},
	}

	if !builder.Exists() {
		klog.V(100).Infof("The Proxy %s does not exist", clusterProxyName)

		return nil, fmt.Errorf("proxy object %s does not exist", clusterProxyName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the proxy object from the cluster if found.
func (builder *Builder) Get() (*configv1.Proxy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting proxy object %s", builder.Definition.Name)

	proxy := &configv1.Proxy{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{Name: builder.Definition.Name}, proxy)
	if err != nil {
		klog.V(100).Infof("Proxy object %s does not exist: %v", builder.Definition.Name, err)

		return nil, err
	}

	return proxy, nil
}

// Exists checks whether the given proxy exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if proxy %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "proxy"

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
