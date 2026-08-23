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

const errEmptyDeviceClassName = "deviceClass 'name' cannot be empty"

// DeviceClassBuilder provides struct for the DeviceClass object containing
// connection to the cluster and the DeviceClass definitions.
type DeviceClassBuilder struct {
	Definition *resourcev1.DeviceClass
	Object     *resourcev1.DeviceClass
	apiClient  goclient.Client
	errorMsg   string
}

// NewDeviceClassBuilder creates a new instance of DeviceClassBuilder.
func NewDeviceClassBuilder(apiClient *clients.Settings, name string) *DeviceClassBuilder {
	klog.V(100).Infof(
		"Initializing new DeviceClassBuilder structure with following params: %s", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(resourcev1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add resource v1 scheme to client schemes")

		return nil
	}

	builder := &DeviceClassBuilder{
		apiClient: apiClient.Client,
		Definition: &resourcev1.DeviceClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the DeviceClass is empty")

		builder.errorMsg = errEmptyDeviceClassName

		return builder
	}

	return builder
}

// WithSelector appends a DeviceSelector to the DeviceClass spec.
func (builder *DeviceClassBuilder) WithSelector(
	selector resourcev1.DeviceSelector) *DeviceClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if selector.CEL == nil {
		builder.errorMsg = "selector must have a CEL expression"

		return builder
	}

	klog.V(100).Infof("Appending selector to DeviceClass %s", builder.Definition.Name)

	builder.Definition.Spec.Selectors = append(builder.Definition.Spec.Selectors, selector)

	return builder
}

// WithCELSelector appends a CEL-based DeviceSelector to the DeviceClass spec.
func (builder *DeviceClassBuilder) WithCELSelector(expression string) *DeviceClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if expression == "" {
		builder.errorMsg = "CEL 'expression' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Appending CEL selector to DeviceClass %s: %s",
		builder.Definition.Name, expression)

	builder.Definition.Spec.Selectors = append(builder.Definition.Spec.Selectors,
		resourcev1.DeviceSelector{
			CEL: &resourcev1.CELDeviceSelector{
				Expression: expression,
			},
		})

	return builder
}

// WithConfig appends a DeviceClassConfiguration to the DeviceClass spec.
func (builder *DeviceClassBuilder) WithConfig(
	config resourcev1.DeviceClassConfiguration) *DeviceClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if config.Opaque == nil {
		builder.errorMsg = "config must have an Opaque device configuration"

		return builder
	}

	if config.Opaque.Driver == "" {
		builder.errorMsg = "config Opaque 'Driver' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Appending config to DeviceClass %s", builder.Definition.Name)

	builder.Definition.Spec.Config = append(builder.Definition.Spec.Config, config)

	return builder
}

// WithLabel adds a label to the DeviceClass.
func (builder *DeviceClassBuilder) WithLabel(key, value string) *DeviceClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if key == "" {
		builder.errorMsg = "label 'key' cannot be empty"

		return builder
	}

	klog.V(100).Infof("Adding label %s=%s to DeviceClass %s", key, value, builder.Definition.Name)

	if builder.Definition.Labels == nil {
		builder.Definition.Labels = make(map[string]string)
	}

	builder.Definition.Labels[key] = value

	return builder
}

// PullDeviceClass pulls an existing DeviceClass from the cluster.
func PullDeviceClass(apiClient *clients.Settings, name string) (*DeviceClassBuilder, error) {
	klog.V(100).Infof("Pulling existing DeviceClass %s from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("deviceClass 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(resourcev1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add resource v1 scheme to client schemes")

		return nil, err
	}

	builder := &DeviceClassBuilder{
		apiClient: apiClient.Client,
		Definition: &resourcev1.DeviceClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		return nil, fmt.Errorf("%s", errEmptyDeviceClassName)
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("deviceClass object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds the DeviceClass in the cluster and stores the object in the struct.
func (builder *DeviceClassBuilder) Create() (*DeviceClassBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating DeviceClass %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies the existing DeviceClass in the cluster.
func (builder *DeviceClassBuilder) Update() (*DeviceClassBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating DeviceClass %s", builder.Definition.Name)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes the DeviceClass.
func (builder *DeviceClassBuilder) Delete() (*DeviceClassBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting DeviceClass %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Info("DeviceClass cannot be deleted because it does not exist")

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

// Exists checks whether the given DeviceClass exists.
func (builder *DeviceClassBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if DeviceClass %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get fetches the defined DeviceClass from the cluster.
func (builder *DeviceClassBuilder) Get() (*resourcev1.DeviceClass, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting DeviceClass %s", builder.Definition.Name)

	deviceClass := &resourcev1.DeviceClass{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, deviceClass)
	if err != nil {
		return nil, err
	}

	return deviceClass, nil
}

// ListDeviceClasses returns a list of DeviceClass builders matching the given options.
func ListDeviceClasses(
	apiClient *clients.Settings,
	options ...goclient.ListOption) ([]*DeviceClassBuilder, error) {
	klog.V(100).Info("Listing DeviceClasses")

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("deviceClass 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(resourcev1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add resource v1 scheme to client schemes")

		return nil, err
	}

	deviceClassList := &resourcev1.DeviceClassList{}

	err = apiClient.List(logging.DiscardContext(), deviceClassList, options...)
	if err != nil {
		return nil, err
	}

	var builders []*DeviceClassBuilder

	for idx := range deviceClassList.Items {
		builders = append(builders, &DeviceClassBuilder{
			apiClient:  apiClient.Client,
			Definition: &deviceClassList.Items[idx],
			Object:     &deviceClassList.Items[idx],
		})
	}

	return builders, nil
}

// validate will check that the builder and builder definition are properly initialized.
func (builder *DeviceClassBuilder) validate() (bool, error) {
	resourceCRD := "DeviceClass"

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
