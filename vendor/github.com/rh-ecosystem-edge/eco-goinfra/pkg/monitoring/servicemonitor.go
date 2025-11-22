package monitoring

import (
	"fmt"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for serviceMonitor object from the cluster
// and a serviceMonitor definition.
type Builder struct {
	// serviceMonitor definition, used to create the serviceMonitor object.
	Definition *monv1.ServiceMonitor
	// Created serviceMonitor object.
	Object *monv1.ServiceMonitor
	// Used to store latest error message upon defining or mutating serviceMonitor definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(
	apiClient *clients.Settings, name, nsname string) *Builder {
	klog.V(100).Infof(
		"Initializing new serviceMonitor structure with the following params: "+
			"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("serviceMonitor 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(monv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add prometheus v1 scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &monv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the serviceMonitor is empty")

		builder.errorMsg = "serviceMonitor 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the serviceMonitor is empty")

		builder.errorMsg = "serviceMonitor 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Pull pulls existing serviceMonitor from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing serviceMonitor name %s in namespace %s from cluster",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("serviceMonitor 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(monv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add prometheus v1 scheme to client schemes")

		return nil, err
	}

	builder := Builder{
		apiClient: apiClient.Client,
		Definition: &monv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the serviceMonitor is empty")

		return nil, fmt.Errorf("serviceMonitor 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the serviceMonitor is empty")

		return nil, fmt.Errorf("serviceMonitor 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("serviceMonitor object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined serviceMonitor from the cluster.
func (builder *Builder) Get() (*monv1.ServiceMonitor, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting serviceMonitor %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	serviceMonitorObj := &monv1.ServiceMonitor{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, serviceMonitorObj)
	if err != nil {
		return nil, err
	}

	return serviceMonitorObj, nil
}

// Create makes a serviceMonitor in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the serviceMonitor %s in namespace %s",
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

// Delete removes serviceMonitor from a cluster.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the serviceMonitor %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("serviceMonitor %s in namespace %s cannot be deleted"+
			" because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete serviceMonitor: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given serviceMonitor exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if serviceMonitor %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing serviceMonitor object with serviceMonitor definition in builder.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating serviceMonitor %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("serviceMonitor", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithEndpoints sets the serviceMonitor operator's endpoints.
func (builder *Builder) WithEndpoints(
	endpoints []monv1.Endpoint) *Builder {
	klog.V(100).Infof(
		"Adding endpoints to serviceMonitor %s in namespace %s; endpoints %v",
		builder.Definition.Name, builder.Definition.Namespace, endpoints)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(endpoints) == 0 {
		klog.V(100).Info("'endpoints' argument cannot be empty")

		builder.errorMsg = "'endpoints' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Endpoints = endpoints

	return builder
}

// WithLabels redefines the serviceMonitor with labels.
func (builder *Builder) WithLabels(labels map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Defining serviceMonitor with labels: %v", labels)

	if len(labels) == 0 {
		klog.V(100).Info("labels can not be empty")

		builder.errorMsg = "labels can not be empty"

		return builder
	}

	for key := range labels {
		if key == "" {
			klog.V(100).Info("The 'labels' key cannot be empty")

			builder.errorMsg = "can not apply a labels with an empty key"

			return builder
		}
	}

	builder.Definition.Labels = labels

	return builder
}

// WithSelector redefines the serviceMonitor with selector.
func (builder *Builder) WithSelector(selector map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Defining serviceMonitor with selector: %v", selector)

	if len(selector) == 0 {
		klog.V(100).Info("selector can not be empty")

		builder.errorMsg = "selector can not be empty"

		return builder
	}

	for key := range selector {
		if key == "" {
			klog.V(100).Info("The 'selector' key cannot be empty")

			builder.errorMsg = "can not apply a selector with an empty key"

			return builder
		}
	}

	builder.Definition.Spec.Selector.MatchLabels = selector

	return builder
}

// WithNamespaceSelector redefines the serviceMonitor with namespaceSelector.
func (builder *Builder) WithNamespaceSelector(namespaceSelector []string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Defining serviceMonitor with namespaceSelector: %v", namespaceSelector)

	if len(namespaceSelector) == 0 {
		klog.V(100).Info("namespaceSelector can not be empty")

		builder.errorMsg = "namespaceSelector can not be empty"

		return builder
	}

	builder.Definition.Spec.NamespaceSelector = monv1.NamespaceSelector{
		MatchNames: namespaceSelector,
	}

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ServiceMonitor"

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
