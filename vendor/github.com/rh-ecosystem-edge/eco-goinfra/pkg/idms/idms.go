package idms

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides struct for configmap object containing connection to the cluster and the configmap definitions.
type Builder struct {
	// ConfigMap definition. Used to create configmap object.
	Definition *configv1.ImageDigestMirrorSet
	// Created configmap object.
	Object *configv1.ImageDigestMirrorSet
	// Used in functions that defines or mutates configmap definition. errorMsg is processed before the configmap
	// object is created.
	errorMsg  string
	apiClient runtimeClient.Client
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name string, mirror configv1.ImageDigestMirrors) *Builder {
	klog.V(100).Infof(
		"Initializing new imagedigestmirrorset structure with the following params: "+
			"name: %s, mirror: %v", name, mirror)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	if err := apiClient.AttachScheme(configv1.AddToScheme); err != nil {
		klog.V(100).Infof(
			"Failed to add configv1 scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.ImageDigestMirrorSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: configv1.ImageDigestMirrorSetSpec{
				ImageDigestMirrors: []configv1.ImageDigestMirrors{
					mirror,
				},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the imagedigestmirrorset is empty")

		builder.errorMsg = "imagedigestmirrorset 'name' cannot be empty"

		return builder
	}

	return builder
}

// Pull retrieves an existing imagedigestmirrorset from the cluster.
func Pull(apiClient *clients.Settings, name string) (*Builder, error) {
	klog.V(100).Infof(
		"Pulling existing imagedigestmirrorset with name %s", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	if err := apiClient.AttachScheme(configv1.AddToScheme); err != nil {
		klog.V(100).Infof(
			"Failed to add configv1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add configv1 to client schemes")
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.ImageDigestMirrorSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the imagedigestmirrorset is empty")

		return nil, fmt.Errorf("imagedigestmirrorset 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("imagedigestmirrorset object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithMirror adds an imagedigestmirror for mirroring images.
func (builder *Builder) WithMirror(mirror configv1.ImageDigestMirrors) *Builder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	klog.V(100).Infof("Adding imagedigestmirror to imagedigestmirrorset %s: %v",
		builder.Definition.Name, mirror)

	builder.Definition.Spec.ImageDigestMirrors = append(builder.Definition.Spec.ImageDigestMirrors, mirror)

	return builder
}

// Get fetches the defined imagedigestmirrorset from the cluster.
func (builder *Builder) Get() (*configv1.ImageDigestMirrorSet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting imagedigestmirrorset %s",
		builder.Definition.Name)

	imageDigestMirrorSet := &configv1.ImageDigestMirrorSet{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeClient.ObjectKey{
		Name: builder.Definition.Name,
	}, imageDigestMirrorSet)
	if err != nil {
		return nil, err
	}

	return imageDigestMirrorSet, err
}

// Create generates an imagedigestmirrorset on the cluster.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the imagedigestmirrorset %s",
		builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing imagedigestmirrorset on the cluster.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating imagedigestmirrorset %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("imagedigestmirrorset %s does not exist",
			builder.Definition.Name)

		return builder, fmt.Errorf("cannot update non-existent imagedigestmirrorset")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("imagedigestmirrorset", builder.Definition.Name))

			err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("imagedigestmirrorset", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes an imagedigestmirrorset from the cluster.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the imagedigestmirrorset %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("imagedigestmirrorset %s does not exist",
			builder.Definition.Name)

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete imagedigestmirrorset: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks if the defined imagedigestmirrorset has already been created.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if imagedigestmirrorset %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ImageDigestMirrorSet"

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
