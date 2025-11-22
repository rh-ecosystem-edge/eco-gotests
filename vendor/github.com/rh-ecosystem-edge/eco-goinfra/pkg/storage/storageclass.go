package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// ClassBuilder provides struct for storageclass object containing
// connection to the cluster and the storageclass definitions.
type ClassBuilder struct {
	// Storageclass definition. Used to create the storageclass object.
	Definition *storageV1.StorageClass
	// Created storageclass object
	Object *storageV1.StorageClass
	// Used in functions that define or mutate storageclass definition. errorMsg is processed before the storageclass
	// object is created.
	errorMsg  string
	apiClient *clients.Settings
}

// AdditionalOptions additional options for storageclass object.
type AdditionalOptions func(builder *ClassBuilder) (*ClassBuilder, error)

// NewClassBuilder creates a new instance of ClassBuilder.
func NewClassBuilder(apiClient *clients.Settings, name, provisioner string) *ClassBuilder {
	klog.V(100).Infof(
		"Initializing new storageclass structure with the following params: "+
			"name: %s, provisioner: %s", name, provisioner)

	if apiClient == nil {
		klog.V(100).Info("StorageClass apiClient cannot be nil")

		return nil
	}

	builder := &ClassBuilder{
		apiClient: apiClient,
		Definition: &storageV1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Provisioner: provisioner,
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the storageclass is empty")

		builder.errorMsg = "storageclass 'name' cannot be empty"

		return builder
	}

	if provisioner == "" {
		klog.V(100).Info("The provisioner of the storageclass is empty")

		builder.errorMsg = "storageclass 'provisioner' cannot be empty"

		return builder
	}

	return builder
}

// WithReclaimPolicy adds a reclaimPolicy to the storageclass definition.
func (builder *ClassBuilder) WithReclaimPolicy(
	reclaimPolicy corev1.PersistentVolumeReclaimPolicy) *ClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if reclaimPolicy == "" {
		klog.V(100).Info("The reclaimPolicy of the storageclass is empty")

		builder.errorMsg = "storageclass 'reclaimPolicy' cannot be empty"

		return builder
	}

	builder.Definition.ReclaimPolicy = &reclaimPolicy

	return builder
}

// WithVolumeBindingMode adds a volumeBindingMode to the storage class definition.
func (builder *ClassBuilder) WithVolumeBindingMode(
	bindingMode storageV1.VolumeBindingMode) *ClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if bindingMode == "" {
		klog.V(100).Info("The bindingMode of the storageclass is empty")

		builder.errorMsg = "storageclass 'bindingMode' cannot be empty"

		return builder
	}

	builder.Definition.VolumeBindingMode = &bindingMode

	return builder
}

// WithParameter adds a parameter to the storage class definition.
func (builder *ClassBuilder) WithParameter(parameterKey, parameterValue string) *ClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if parameterKey == "" {
		klog.V(100).Info("The parameter key of the storageclass is empty")

		builder.errorMsg = "storageclass parameter key cannot be empty"

		return builder
	}

	if parameterValue == "" {
		klog.V(100).Info("The parameter value of the storageclass is empty")

		builder.errorMsg = "storageclass parameter value cannot be empty"

		return builder
	}

	if builder.Definition.Parameters == nil {
		builder.Definition.Parameters = make(map[string]string)
	}

	builder.Definition.Parameters[parameterKey] = parameterValue

	return builder
}

// WithOptions creates a storageclass with generic mutation options.
func (builder *ClassBuilder) WithOptions(options ...AdditionalOptions) *ClassBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting storageclass additional options")

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

// PullClass pulls an existing storage class into a ClassBuilder struct.
func PullClass(apiClient *clients.Settings, name string) (*ClassBuilder, error) {
	klog.V(100).Infof("Pulling existing storageclass %s from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The storageclass apiClient is nil")

		return nil, fmt.Errorf("storageclass 'apiClient' cannot be empty")
	}

	builder := &ClassBuilder{
		apiClient: apiClient,
		Definition: &storageV1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the storageclass is empty")

		return nil, fmt.Errorf("storageclass 'name' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("storageclass object %s does not exist", name)

		return nil, fmt.Errorf("storageclass object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given storageclass exists.
func (builder *ClassBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if storageclass %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.apiClient.StorageClasses().Get(
		logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create generates a storageclass in cluster and stores the created object in struct.
func (builder *ClassBuilder) Create() (*ClassBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating storageclass %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.StorageClasses().Create(
			logging.DiscardContext(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a storageclass.
func (builder *ClassBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting storageclass %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("storageclass %s does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.StorageClasses().Delete(
		logging.DiscardContext(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes the StorageClass and waits up to timeout until it has been removed.
func (builder *ClassBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Deleting StorageClass %s and waiting up to %s until it is removed", builder.Definition.Name, timeout)

	err := builder.Delete()
	if err != nil {
		return err
	}

	return builder.WaitUntilDeleted(timeout)
}

// WaitUntilDeleted waits for the duration of timeout or until the StorageClass has been deleted.
func (builder *ClassBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Waiting up to %s until StorageClass %s is deleted", timeout, builder.Definition.Name)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.apiClient.StorageClasses().Get(logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})
			if err == nil {
				klog.V(100).Infof("StorageClass %s still present", builder.Definition.Name)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				klog.V(100).Infof("StorageClass %s is gone", builder.Definition.Name)

				return true, nil
			}

			klog.V(100).Infof("failed to get StorageClass %s", builder.Definition.Name)

			return false, err
		})
}

// Update renovates the existing storageclass object with the storageclass definition in builder.
func (builder *ClassBuilder) Update(force bool) (*ClassBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating storageclass %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("storageclass %s does not exist",
			builder.Definition.Name)

		return builder, fmt.Errorf("cannot update non-existent storageclass")
	}

	var err error

	builder.Object, err = builder.apiClient.StorageClasses().
		Update(logging.DiscardContext(), builder.Definition, metav1.UpdateOptions{})
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("storageclass", builder.Definition.Name))

			err = builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof(
					"Failed to update the storageclass object %s, "+
						"due to error in delete function",
					builder.Definition.Name,
				)

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClassBuilder) validate() (bool, error) {
	resourceCRD := "storageClass"

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
