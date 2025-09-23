package keda

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	kedav1alpha1 "github.com/kedacore/keda-olm-operator/api/keda/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ControllerBuilder provides a struct for KedaController object from the cluster and a KedaController definition.
type ControllerBuilder struct {
	// KedaController definition, used to create the KedaController object.
	Definition *kedav1alpha1.KedaController
	// Created KedaController object.
	Object *kedav1alpha1.KedaController
	// Used to store latest error message upon defining or mutating KedaController definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewControllerBuilder creates a new instance of ControllerBuilder.
func NewControllerBuilder(
	apiClient *clients.Settings, name, nsname string) *ControllerBuilder {
	glog.V(100).Infof(
		"Initializing new kedaController structure with the following params: "+
			"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("kedaController 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(kedav1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add kedav1alpha1 scheme to client schemes")

		return nil
	}

	builder := &ControllerBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav1alpha1.KedaController{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the KedaController is empty")

		builder.errorMsg = "kedaController 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The nsname of the KedaController is empty")

		builder.errorMsg = "kedaController 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullController pulls existing kedaController from cluster.
func PullController(apiClient *clients.Settings, name, nsname string) (*ControllerBuilder, error) {
	glog.V(100).Infof("Pulling existing kedaController name %s in namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("kedaController 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(kedav1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add kedav1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := ControllerBuilder{
		apiClient: apiClient.Client,
		Definition: &kedav1alpha1.KedaController{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the kedaController is empty")

		return nil, fmt.Errorf("kedaController 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the kedaController is empty")

		return nil, fmt.Errorf("kedaController 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("kedaController object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches the defined kedaController from the cluster.
func (builder *ControllerBuilder) Get() (*kedav1alpha1.KedaController, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting kedaController %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	kedaObj := &kedav1alpha1.KedaController{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, kedaObj)
	if err != nil {
		return nil, err
	}

	return kedaObj, nil
}

// Create makes a kedaController in the cluster and stores the created object in struct.
func (builder *ControllerBuilder) Create() (*ControllerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the kedaController %s in namespace %s",
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

// Delete removes kedaController from a cluster.
func (builder *ControllerBuilder) Delete() (*ControllerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the kedaController %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("kedaController %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete kedaController: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given kedaController exists.
func (builder *ControllerBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if kedaController %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing kedaController object with kedaController definition in builder.
func (builder *ControllerBuilder) Update() (*ControllerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating kedaController %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		glog.V(100).Infof(
			msg.FailToUpdateError("kedaController", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithAdmissionWebhooks sets the kedaController operator's profile.
func (builder *ControllerBuilder) WithAdmissionWebhooks(
	admissionWebhooks kedav1alpha1.KedaAdmissionWebhooksSpec) *ControllerBuilder {
	glog.V(100).Infof(
		"Adding admissionWebhooks to kedaController %s in namespace %s; admissionWebhooks %v",
		builder.Definition.Name, builder.Definition.Namespace, admissionWebhooks)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.AdmissionWebhooks = admissionWebhooks

	return builder
}

// WithOperator sets the kedaController operator's profile.
func (builder *ControllerBuilder) WithOperator(
	operator kedav1alpha1.KedaOperatorSpec) *ControllerBuilder {
	glog.V(100).Infof(
		"Adding operator to kedaController %s in namespace %s; operator %v",
		builder.Definition.Name, builder.Definition.Namespace, operator)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.Operator = operator

	return builder
}

// WithMetricsServer sets the kedaController operator's metricsServer.
func (builder *ControllerBuilder) WithMetricsServer(
	metricsServer kedav1alpha1.KedaMetricsServerSpec) *ControllerBuilder {
	glog.V(100).Infof(
		"Adding metricsServer to kedaController %s in namespace %s; metricsServer %v",
		builder.Definition.Name, builder.Definition.Namespace, metricsServer)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.MetricsServer = metricsServer

	return builder
}

// WithWatchNamespace sets the kedaController operator's watchNamespace.
func (builder *ControllerBuilder) WithWatchNamespace(
	watchNamespace string) *ControllerBuilder {
	glog.V(100).Infof(
		"Adding watchNamespace to kedaController %s in namespace %s; watchNamespace %v",
		builder.Definition.Name, builder.Definition.Namespace, watchNamespace)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if watchNamespace == "" {
		glog.V(100).Infof("The watchNamespace is empty")

		builder.errorMsg = "'watchNamespace' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.WatchNamespace = watchNamespace

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ControllerBuilder) validate() (bool, error) {
	resourceCRD := "KedaController"

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
