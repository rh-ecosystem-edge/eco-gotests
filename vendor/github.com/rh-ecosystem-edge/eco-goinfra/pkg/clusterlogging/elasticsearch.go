package clusterlogging

import (
	"fmt"

	eskv1 "github.com/openshift/elasticsearch-operator/apis/logging/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ElasticsearchBuilder provides struct for the elasticsearch object.
type ElasticsearchBuilder struct {
	// Elasticsearch definition. Used to create elasticsearch object with minimum set of required elements.
	Definition *eskv1.Elasticsearch
	// Created elasticsearch object on the cluster.
	Object *eskv1.Elasticsearch
	// api client to interact with the cluster.
	apiClient goclient.Client
	// errorMsg is processed before elasticsearch object is created.
	errorMsg string
}

// NewElasticsearchBuilder method creates new instance of builder.
func NewElasticsearchBuilder(
	apiClient *clients.Settings, name, nsname string) *ElasticsearchBuilder {
	klog.V(100).Infof("Initializing new elasticsearch structure with the following params: name: %s, namespace: %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("elasticsearch 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(eskv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add eskv1 scheme to client schemes")

		return nil
	}

	builder := &ElasticsearchBuilder{
		apiClient: apiClient.Client,
		Definition: &eskv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the elasticsearch is empty")

		builder.errorMsg = "elasticsearch 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the elasticsearch is empty")

		builder.errorMsg = "elasticsearch 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullElasticsearch retrieves an existing elasticsearch object from the cluster.
func PullElasticsearch(apiClient *clients.Settings, name, nsname string) (*ElasticsearchBuilder, error) {
	klog.V(100).Infof(
		"Pulling elasticsearch object name:%s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("elasticsearch 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(eskv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add eskv1 scheme to client schemes")

		return nil, err
	}

	builder := &ElasticsearchBuilder{
		apiClient: apiClient.Client,
		Definition: &eskv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the elasticsearch is empty")

		return nil, fmt.Errorf("elasticsearch 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the elasticsearch is empty")

		return nil, fmt.Errorf("elasticsearch 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("elasticsearch object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns elasticsearch object if found.
func (builder *ElasticsearchBuilder) Get() (*eskv1.Elasticsearch, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting elasticsearch %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	elasticsearchObj := &eskv1.Elasticsearch{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, elasticsearchObj)
	if err != nil {
		return nil, err
	}

	return elasticsearchObj, err
}

// Create makes a elasticsearch in the cluster and stores the created object in struct.
func (builder *ElasticsearchBuilder) Create() (*ElasticsearchBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the elasticsearch %s in namespace %s",
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

// Delete removes elasticsearch from a cluster.
func (builder *ElasticsearchBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the elasticsearch %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return fmt.Errorf("elasticsearch cannot be deleted because it does not exist")
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete elasticsearch: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given elasticsearch exists.
func (builder *ElasticsearchBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if elasticsearch %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing elasticsearch object with elasticsearch definition in builder.
func (builder *ElasticsearchBuilder) Update() (*ElasticsearchBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating elasticsearch %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("elasticsearch", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// WithManagementState sets the elasticsearch operator's management state.
func (builder *ElasticsearchBuilder) WithManagementState(
	expectedManagementState eskv1.ManagementState) *ElasticsearchBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting elasticsearch %s in namespace %s with the ManagementState: %v",
		builder.Definition.Name, builder.Definition.Namespace, expectedManagementState)

	builder.Definition.Spec.ManagementState = expectedManagementState

	return builder
}

// GetManagementState fetches elasticsearch ManagementState.
func (builder *ElasticsearchBuilder) GetManagementState() (*eskv1.ManagementState, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Info("Getting elasticsearch ManagementState configuration")

	if !builder.Exists() {
		return nil, fmt.Errorf("elasticsearch object does not exist")
	}

	return &builder.Object.Spec.ManagementState, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ElasticsearchBuilder) validate() (bool, error) {
	resourceCRD := "Elasticsearch"

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
