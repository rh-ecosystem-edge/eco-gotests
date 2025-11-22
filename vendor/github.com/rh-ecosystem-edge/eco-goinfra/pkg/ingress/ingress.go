package ingress

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultIngressClassName is the default ingress class name for the ingress. It is added in [NewIngressBuilder] to all
// ingresses.
const DefaultIngressClassName = "openshift-default"

// IngressBuilder provides a struct for an ingress object from the cluster and an ingress definition.
type IngressBuilder struct {
	// ingress definition, used to create the ingress object.
	Definition *networkingv1.Ingress
	// Created ingress object.
	Object *networkingv1.Ingress
	// api client to interact with the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate ingress definition. errorMsg is processed before the ingress
	// object is created.
	errorMsg string
}

// NewIngressBuilder creates a new instance of IngressBuilder.
func NewIngressBuilder(apiClient *clients.Settings, name, nsname string) *IngressBuilder {
	klog.V(100).Infof(
		"Initializing new ingress structure with the following params: name=%s, namespace=%s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Infof("The ingress apiClient is nil")

		return nil
	}

	err := apiClient.AttachScheme(networkingv1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add networkingv1 scheme to client schemes")

		return nil
	}

	builder := &IngressBuilder{
		apiClient: apiClient.Client,
		Definition: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: ptr.To(DefaultIngressClassName),
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The name of the ingress is empty")

		builder.errorMsg = "ingress 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Infof("The namespace of the ingress is empty")

		builder.errorMsg = "ingress 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// PullIngress loads an existing ingress into IngressBuilder struct.
func PullIngress(apiClient *clients.Settings, name, nsname string) (*IngressBuilder, error) {
	klog.V(100).Infof("Pulling existing ingress %s in namespace %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Infof("The ingress apiClient is nil")

		return nil, fmt.Errorf("ingress 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(networkingv1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add networkingv1 scheme to client schemes")

		return nil, err
	}

	builder := &IngressBuilder{
		apiClient: apiClient.Client,
		Definition: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The ingress name is empty")

		return nil, fmt.Errorf("ingress name cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Infof("The ingress namespace is empty")

		return nil, fmt.Errorf("ingress namespace cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("could not find ingress %s in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing ingress from cluster.
func (builder *IngressBuilder) Get() (*networkingv1.Ingress, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Fetching existing ingress with name %s under namespace %s from cluster",
		builder.Definition.Name, builder.Definition.Namespace)

	ingress := &networkingv1.Ingress{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, ingress)
	if err != nil {
		klog.V(100).Infof("Failed to get Ingress %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return ingress, nil
}

// Exists checks whether the given ingress exists. It returns true if and only if the ingress was retrieved
// successfully. In the event of any error, it returns false and logs the error.
func (builder *IngressBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if ingress %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("In Exists, failed to get the ingress %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)
	}

	return err == nil
}

// Create makes an ingress in the cluster and stores the created object in struct.
func (builder *IngressBuilder) Create() (*IngressBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the ingress %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		klog.V(100).Infof("Ingress %s already exists in namespace %s, skipping creation",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = builder.Definition

		return builder, nil
	}

	err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Failed to create Ingress %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, err
}

// Update renovates an ingress in the cluster and stores the created object in struct.
func (builder *IngressBuilder) Update() (*IngressBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the ingress %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("ingress object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	// The object should be updated by Exists method, so by reusing its resource version we are more likely to avoid
	// conflicts.
	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Failed to update Ingress %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, fmt.Errorf("cannot update ingress: %w", err)
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes an ingress.
func (builder *IngressBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the ingress %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("Ingress %s in namespace %s not deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Failed to delete Ingress %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return fmt.Errorf("cannot delete ingress: %w", err)
	}

	builder.Object = nil

	return nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *IngressBuilder) validate() (bool, error) {
	resourceCRD := "ingress"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
