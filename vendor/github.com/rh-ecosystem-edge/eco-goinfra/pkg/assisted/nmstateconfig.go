package assisted

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	assistedv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NmStateConfigBuilder provides struct for the NMStateConfig object containing connection to
// the cluster and the NMStateConfig definitions.
type NmStateConfigBuilder struct {
	// NMStateConfig definition. Used to create NMStateConfig object with minimum set of required elements.
	Definition *assistedv1beta1.NMStateConfig
	// Created NMStateConfig object on the cluster.
	Object *assistedv1beta1.NMStateConfig
	// API client to interact with the cluster.
	apiClient goclient.Client
	// errorMsg is processed before NMStateConfig object is created.
	errorMsg string
}

// NewNmStateConfigBuilder creates a new instance of NMStateConfig Builder.
func NewNmStateConfigBuilder(apiClient *clients.Settings, name, namespace string) *NmStateConfigBuilder {
	klog.V(100).Infof("Initializing new nmstateconfig structure with the name: %s in namespace: %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	builder := &NmStateConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &assistedv1beta1.NMStateConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the nmstateconfig is empty")

		builder.errorMsg = "nmstateconfig 'name' cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the nmstateconfig is empty")

		builder.errorMsg = "nmstateconfig namespace's name is empty"

		return builder
	}

	return builder
}

// Exists checks whether the given NMStateConfig exists.
func (builder *NmStateConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if nmstateconfig %s exists in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns NMStateConfig object if found.
func (builder *NmStateConfigBuilder) Get() (*assistedv1beta1.NMStateConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting nmstateconfig object %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nmStateConfig := &assistedv1beta1.NMStateConfig{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nmStateConfig)
	if err != nil {
		klog.V(100).Infof("nmstateconfig object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return nmStateConfig, nil
}

// Create makes a NMStateConfig in the cluster and stores the created object in struct.
func (builder *NmStateConfigBuilder) Create() (*NmStateConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the nmstateconfig %s in namespace: %s",
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

// Delete removes nmstateconfig object from a cluster.
func (builder *NmStateConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the nmstateconfig object %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete nmstateconfig: %w", err)
	}

	builder.Object = nil

	return nil
}

// ListNmStateConfigsInAllNamespaces returns a cluster-wide NMStateConfig list.
func ListNmStateConfigsInAllNamespaces(apiClient *clients.Settings) ([]*NmStateConfigBuilder, error) {
	nmStateConfigList := &assistedv1beta1.NMStateConfigList{}

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.List(context.TODO(), nmStateConfigList, &goclient.ListOptions{})
	if err != nil {
		klog.V(100).Infof("Failed to list nmStateConfigs across all namespaces due to %s", err.Error())

		return nil, err
	}

	var nmstateConfigObjects []*NmStateConfigBuilder

	for _, nmStateConfigObj := range nmStateConfigList.Items {
		nmStateConf := nmStateConfigObj
		nmStateConfBuilder := &NmStateConfigBuilder{
			apiClient:  apiClient.Client,
			Definition: &nmStateConf,
			Object:     &nmStateConf,
		}

		nmstateConfigObjects = append(nmstateConfigObjects, nmStateConfBuilder)
	}

	return nmstateConfigObjects, err
}

// ListNmStateConfigs returns a NMStateConfig list in a given namespace.
func ListNmStateConfigs(apiClient *clients.Settings, namespace string) ([]*NmStateConfigBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	nmStateConfigList := &assistedv1beta1.NMStateConfigList{}

	if namespace == "" {
		return nil, fmt.Errorf("namespace to list nmstateconfigs cannot be empty")
	}

	err := apiClient.List(context.TODO(), nmStateConfigList, &goclient.ListOptions{Namespace: namespace})
	if err != nil {
		klog.V(100).Infof("Failed to list nmStateConfigs in namespace: %s due to %s",
			namespace, err.Error())

		return nil, err
	}

	var nmstateConfigObjects []*NmStateConfigBuilder

	for _, nmStateConfigObj := range nmStateConfigList.Items {
		nmStateConf := nmStateConfigObj
		nmStateConfBuilder := &NmStateConfigBuilder{
			apiClient:  apiClient.Client,
			Definition: &nmStateConf,
			Object:     &nmStateConf,
		}

		nmstateConfigObjects = append(nmstateConfigObjects, nmStateConfBuilder)
	}

	return nmstateConfigObjects, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NmStateConfigBuilder) validate() (bool, error) {
	resourceCRD := "NMStateConfig"

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
