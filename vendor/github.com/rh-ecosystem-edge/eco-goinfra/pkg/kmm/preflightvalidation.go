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

// PreflightValidationBuilder provides struct for the preflightvalidation object
// containing connection to the cluster and the preflightvalidation definitions.
type PreflightValidationBuilder struct {
	// PreflightValidation definition. Used to create a PreflightValidation object.
	Definition *kmmv1beta2.PreflightValidation
	// Created PreflightValidation object.
	Object *kmmv1beta2.PreflightValidation
	// ApiClient to interact with the cluster.
	apiClient *clients.Settings
	// errorMsg is processed before the object is created or updated.
	errorMsg string
}

// PreflightValidationAdditionalOptions additional options for preflightvalidation object.
type PreflightValidationAdditionalOptions func(
	builder *PreflightValidationBuilder) (*PreflightValidationBuilder, error)

// NewPreflightValidationBuilder creates a new instance of PreflightValidationBuilder.
func NewPreflightValidationBuilder(
	apiClient *clients.Settings, name, nsname string) *PreflightValidationBuilder {
	klog.V(100).Infof("Initializing new PreflightValidation structure with following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(kmmv1beta2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add kmm v1beta2 scheme to client schemes")

		return nil
	}

	builder := &PreflightValidationBuilder{
		apiClient: apiClient,
		Definition: &kmmv1beta2.PreflightValidation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the PreflightValidation is empty")

		builder.errorMsg = "PreflightValidation 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the PreflightValidation is empty")

		builder.errorMsg = "PreflightValidation 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithKernelVersion sets the kernel for which the preflightvalidation checks the module.
func (builder *PreflightValidationBuilder) WithKernelVersion(kernelVersion string) *PreflightValidationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if kernelVersion == "" {
		builder.errorMsg = "invalid 'kernelVersion' argument can not be nil"

		return builder
	}

	klog.V(100).Infof("Creating new PreflightValidation with kernelVersion: %s",
		kernelVersion)

	builder.Definition.Spec.KernelVersion = kernelVersion

	return builder
}

// WithPushBuiltImage configures the build to be pushed to the registry.
func (builder *PreflightValidationBuilder) WithPushBuiltImage(push bool) *PreflightValidationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating new PreflightValidation with PushBuiltImage set to: %v", push)

	builder.Definition.Spec.PushBuiltImage = push

	return builder
}

// WithOptions creates PreflightValidation with generic mutation options.
func (builder *PreflightValidationBuilder) WithOptions(
	options ...PreflightValidationAdditionalOptions) *PreflightValidationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting PreflightValidation additional options")

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

// PullPreflightValidation fetches existing PreflightValidation from the cluster.
func PullPreflightValidation(apiClient *clients.Settings,
	name, nsname string) (*PreflightValidationBuilder, error) {
	klog.V(100).Infof("Pulling existing preflightvalidation name %s under namespace %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("preflightvalidation 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(kmmv1beta2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add preflightvalidation v1beta2 scheme to client schemes")

		return nil, err
	}

	builder := &PreflightValidationBuilder{
		apiClient: apiClient,
		Definition: &kmmv1beta2.PreflightValidation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the preflightvalidation is empty")

		return builder, fmt.Errorf("%s", "preflightvalidation 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the preflightvalidation is empty")

		return builder, fmt.Errorf("%s", "preflightvalidation 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("preflightvalidation object %s doesn't exist in namespace %s",
			name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds preflightvalidation in the cluster and stores object in struct.
func (builder *PreflightValidationBuilder) Create() (*PreflightValidationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating preflightvalidation %s in namespace %s",
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

// Update modifies an existing preflightvalidation on the cluster.
func (builder *PreflightValidationBuilder) Update() (*PreflightValidationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating preflightvalidation %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks if the defined preflightvalidation has already been created.
func (builder *PreflightValidationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if preflightvalidation %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a preflightvalidation from the cluster.
func (builder *PreflightValidationBuilder) Delete() (*PreflightValidationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting preflightvalidation %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("preflightvalidation %s namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete preflightvalidation: %w", err)
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""

	return builder, nil
}

// Get fetches the defined preflightvalidation from the cluster.
func (builder *PreflightValidationBuilder) Get() (*kmmv1beta2.PreflightValidation, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting preflightvalidation %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	preflightvalidation := &kmmv1beta2.PreflightValidation{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, preflightvalidation)
	if err != nil {
		return nil, err
	}

	return preflightvalidation, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PreflightValidationBuilder) validate() (bool, error) {
	resourceCRD := "PreflightValidation"

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
