package network

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

const (
	clusterNetworkName = "cluster"
)

// ConfigBuilder provides a struct for network object from the cluster and a network definition.
type ConfigBuilder struct {
	// network definition, used to create the network object.
	Definition *configv1.Network
	// Created network object.
	Object *configv1.Network
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
}

// PullConfig loads an existing network into ConfigBuilder struct.
func PullConfig(apiClient *clients.Settings) (*ConfigBuilder, error) {
	glog.V(100).Infof("Pulling existing network.config name: %s", clusterNetworkName)

	if apiClient == nil {
		glog.V(100).Info("The network.configapiClient is nil")

		return nil, fmt.Errorf("network.config 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add config v1 scheme to client schemes")

		return nil, err
	}

	builder := &ConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &configv1.Network{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterNetworkName,
			},
		},
	}

	if !builder.Exists() {
		glog.V(100).Infof("network.config %s does not exist", clusterNetworkName)

		return nil, fmt.Errorf("network.config object %s does not exist", clusterNetworkName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the network object from the cluster if it exists.
func (builder *ConfigBuilder) Get() (*configv1.Network, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting network.config object %s", builder.Definition.Name)

	network := &configv1.Network{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, network)
	if err != nil {
		glog.V(100).Infof("Failed to get network.config object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return network, err
}

// Exists checks whether the given network exists.
func (builder *ConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if network.config %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ConfigBuilder) validate() (bool, error) {
	resourceCRD := "network.config"

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

	return true, nil
}
