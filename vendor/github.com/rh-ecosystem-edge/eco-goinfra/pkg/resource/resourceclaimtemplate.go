package resource

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	resourcev1 "k8s.io/api/resource/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errRCTNameEmpty      = "ResourceClaimTemplate 'name' cannot be empty"
	errRCTNamespaceEmpty = "ResourceClaimTemplate 'namespace' cannot be empty"
)

// ResourceClaimTemplateBuilder provides struct for the ResourceClaimTemplate object
// containing connection to the cluster and the ResourceClaimTemplate definitions.
type ResourceClaimTemplateBuilder struct {
	Definition *resourcev1.ResourceClaimTemplate
	Object     *resourcev1.ResourceClaimTemplate
	apiClient  goclient.Client
	errorMsg   string
}

// NewResourceClaimTemplateBuilder creates a new instance of ResourceClaimTemplateBuilder.
func NewResourceClaimTemplateBuilder(
	apiClient *clients.Settings, name, namespace string) *ResourceClaimTemplateBuilder {
	klog.V(100).Infof(
		"Initializing new ResourceClaimTemplateBuilder with name: %s, namespace: %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(resourcev1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add resource v1 scheme to client schemes")

		return nil
	}

	builder := &ResourceClaimTemplateBuilder{
		apiClient: apiClient.Client,
		Definition: &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the ResourceClaimTemplate is empty")

		builder.errorMsg = errRCTNameEmpty

		return builder
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the ResourceClaimTemplate is empty")

		builder.errorMsg = errRCTNamespaceEmpty

		return builder
	}

	return builder
}

// WithDeviceRequest adds a device request to the ResourceClaimTemplate.
func (builder *ResourceClaimTemplateBuilder) WithDeviceRequest(
	requestName, deviceClassName string, count int64) *ResourceClaimTemplateBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if requestName == "" {
		builder.errorMsg = "device request 'name' cannot be empty"

		return builder
	}

	if deviceClassName == "" {
		builder.errorMsg = "device request 'deviceClassName' cannot be empty"

		return builder
	}

	if count < 1 {
		builder.errorMsg = "device request 'count' must be at least 1"

		return builder
	}

	klog.V(100).Infof("Adding device request %s (class: %s, count: %d) to ResourceClaimTemplate %s",
		requestName, deviceClassName, count, builder.Definition.Name)

	request := resourcev1.DeviceRequest{
		Name: requestName,
		FirstAvailable: []resourcev1.DeviceSubRequest{
			{
				Name:            "default",
				DeviceClassName: deviceClassName,
				Count:           count,
			},
		},
	}

	builder.Definition.Spec.Spec.Devices.Requests = append(
		builder.Definition.Spec.Spec.Devices.Requests, request)

	return builder
}

// PullResourceClaimTemplate pulls an existing ResourceClaimTemplate from the cluster.
func PullResourceClaimTemplate(
	apiClient *clients.Settings, name, namespace string) (*ResourceClaimTemplateBuilder, error) {
	klog.V(100).Infof("Pulling existing ResourceClaimTemplate %s from namespace %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("resourceClaimTemplate 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(resourcev1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add resource v1 scheme to client schemes")

		return nil, err
	}

	builder := &ResourceClaimTemplateBuilder{
		apiClient: apiClient.Client,
		Definition: &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		return nil, fmt.Errorf("resourceClaimTemplate 'name' cannot be empty")
	}

	if namespace == "" {
		return nil, fmt.Errorf("resourceClaimTemplate 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"resourceClaimTemplate object %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds the ResourceClaimTemplate in the cluster.
func (builder *ResourceClaimTemplateBuilder) Create() (*ResourceClaimTemplateBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating ResourceClaimTemplate %s in namespace %s",
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

// Delete removes the ResourceClaimTemplate from the cluster.
func (builder *ResourceClaimTemplateBuilder) Delete() (*ResourceClaimTemplateBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting ResourceClaimTemplate %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Info("ResourceClaimTemplate cannot be deleted because it does not exist")

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given ResourceClaimTemplate exists.
func (builder *ResourceClaimTemplateBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if ResourceClaimTemplate %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get fetches the defined ResourceClaimTemplate from the cluster.
func (builder *ResourceClaimTemplateBuilder) Get() (*resourcev1.ResourceClaimTemplate, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting ResourceClaimTemplate %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	rct := &resourcev1.ResourceClaimTemplate{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, rct)
	if err != nil {
		return nil, err
	}

	return rct, nil
}

// validate checks that the builder is properly configured.
func (builder *ResourceClaimTemplateBuilder) validate() (bool, error) {
	resourceCRD := "ResourceClaimTemplate"

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
