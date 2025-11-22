package kmm

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	kmmv1beta2 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PreflightValidationOCPBuilder provides struct for the preflightvalidationocp object
// containing connection to the cluster and the preflightvalidationocp definitions.
type PreflightValidationOCPBuilder struct {
	// PreflightValidationOCP definition. Used to create a PreflightValidationOCP object.
	Definition *kmmv1beta2.PreflightValidationOCP
	// Created PreflightValidationOCP object.
	Object *kmmv1beta2.PreflightValidationOCP
	// ApiClient to interact with the cluster.
	apiClient *clients.Settings
	// errorMsg is processed before the object is created or updated.
	errorMsg string
}

// PreflightValidationOCPAdditionalOptions additional options for preflightvalidationocp object.
type PreflightValidationOCPAdditionalOptions func(
	builder *PreflightValidationOCPBuilder) (*PreflightValidationOCPBuilder, error)

// NewPreflightValidationOCPBuilder creates a new instance of PreflightValidationOCPBuilder.
func NewPreflightValidationOCPBuilder(
	apiClient *clients.Settings, name, nsname string) *PreflightValidationOCPBuilder {
	klog.V(100).Infof("Initializing new PreflightValidationOCP structure with following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(kmmv1beta2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add module v1beta2 scheme to client schemes")

		return nil
	}

	builder := &PreflightValidationOCPBuilder{
		apiClient: apiClient,
		Definition: &kmmv1beta2.PreflightValidationOCP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the PreflightValidationOCP is empty")

		builder.errorMsg = "PreflightValidationOCP 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the PreflightValidationOCP is empty")

		builder.errorMsg = "PreflightValidationOCP 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithKernelVersion sets the image for which the preflightvalidationocp checks the module.
func (builder *PreflightValidationOCPBuilder) WithKernelVersion(kernelVersion string) *PreflightValidationOCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if kernelVersion == "" {
		builder.errorMsg = "invalid 'kernelVersion' argument can not be nil"

		return builder
	}

	klog.V(100).Infof("Creating new PreflightValidationOCP with kernelVersion: %s",
		kernelVersion)

	builder.Definition.Spec.KernelVersion = kernelVersion

	return builder
}

// WithDtkImage specifies if the kernel is realtime.
func (builder *PreflightValidationOCPBuilder) WithDtkImage(dtkImage string) *PreflightValidationOCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if dtkImage == "" {
		builder.errorMsg = "invalid 'dtkImage' argument can not be nil"

		return builder
	}

	klog.V(100).Infof("Creating new PreflightValidationOCP with dtkImage set to: %s", dtkImage)

	builder.Definition.Spec.DTKImage = dtkImage

	return builder
}

// WithPushBuiltImage configures the build to be pushed to the registry.
func (builder *PreflightValidationOCPBuilder) WithPushBuiltImage(push bool) *PreflightValidationOCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating new PreflightValidationOCP with PushBuiltImage set to: %v", push)

	builder.Definition.Spec.PushBuiltImage = push

	return builder
}

// WithOptions creates Module with generic mutation options.
func (builder *PreflightValidationOCPBuilder) WithOptions(
	options ...PreflightValidationOCPAdditionalOptions) *PreflightValidationOCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting Module additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullPreflightValidationOCP fetches existing PreflightValidationOCP from the cluster.
func PullPreflightValidationOCP(apiClient *clients.Settings,
	name, nsname string) (*PreflightValidationOCPBuilder, error) {
	klog.V(100).Infof("Pulling existing preflightvalidationocp name %s under namespace %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("preflightvalidationocp 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(kmmv1beta2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add module v1beta2 scheme to client schemes")

		return nil, err
	}

	builder := &PreflightValidationOCPBuilder{
		apiClient: apiClient,
		Definition: &kmmv1beta2.PreflightValidationOCP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the preflightvalidationocp is empty")

		return builder, fmt.Errorf("%s", "preflightvalidationocp 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the preflightvalidationocp is empty")

		return builder, fmt.Errorf("%s", "preflightvalidationocp 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("preflightvalidationocp object %s doesn't exist in namespace %s",
			name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds preflightvalidationocp in the cluster and stores object in struct.
func (builder *PreflightValidationOCPBuilder) Create() (*PreflightValidationOCPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating preflightvalidationocp %s in namespace %s",
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

// Update modifies an existing preflightvalidationocp on the cluster.
func (builder *PreflightValidationOCPBuilder) Update() (*PreflightValidationOCPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating preflightvalidationocp %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks if the defined preflightvalidationocp has already been created.
func (builder *PreflightValidationOCPBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if preflightvalidationocp %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a preflightvalidationocp from the cluster.
func (builder *PreflightValidationOCPBuilder) Delete() (*PreflightValidationOCPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting preflightvalidationocp %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("preflightvalidationocp %s namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete preflightvalidationocp: %w", err)
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""

	return builder, nil
}

// Get fetches the defined preflightvalidationocp from the cluster.
func (builder *PreflightValidationOCPBuilder) Get() (*kmmv1beta2.PreflightValidationOCP, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting preflightvalidationocp %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	preflightvalidationocp := &kmmv1beta2.PreflightValidationOCP{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, preflightvalidationocp)
	if err != nil {
		klog.V(100).Infof("Preflightvalidationocp object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return preflightvalidationocp, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PreflightValidationOCPBuilder) validate() (bool, error) {
	resourceCRD := "PreflightValidationOCP"

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
