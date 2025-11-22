package velero

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BackupStorageLocationBuilder provides a struct for backupstoragelocation
// object from the cluster and a backupstoragelocation definition.
type BackupStorageLocationBuilder struct {
	// BackupStorageLocation definition, used to create the backupstoragelocation object.
	Definition *velerov1.BackupStorageLocation
	// Created backupstoragelocation object.
	Object *velerov1.BackupStorageLocation
	// Used to store latest error message upon defining or mutating backupstoragelocation definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewBackupStorageLocationBuilder creates a new instance of BackupStorageLocationBuilder.
func NewBackupStorageLocationBuilder(
	apiClient *clients.Settings,
	name string,
	namespace string,
	provider string,
	objectStorage velerov1.ObjectStorageLocation) *BackupStorageLocationBuilder {
	klog.V(100).Infof(
		"Initializing new backupstoragelocation structure with the following params: "+
			"name: %s, namespace: %s, provider: %s, objectStorage: %v", name, namespace, provider, objectStorage)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(velerov1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add velero v1 scheme to client schemes")

		return nil
	}

	builder := &BackupStorageLocationBuilder{
		apiClient: apiClient.Client,
		Definition: &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: velerov1.BackupStorageLocationSpec{
				Provider: provider,
				StorageType: velerov1.StorageType{
					ObjectStorage: &objectStorage,
				},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the backupstoragelocation is empty")

		builder.errorMsg = "backupstoragelocation name cannot be empty"
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the backupstoragelocation is empty")

		builder.errorMsg = "backupstoragelocation namespace cannot be empty"
	}

	if provider == "" {
		klog.V(100).Info("The provider of the backupstoragelocation is empty")

		builder.errorMsg = "backupstoragelocation provider cannot be empty"
	}

	if objectStorage.Bucket == "" {
		klog.V(100).Info("The objectstorage bucket of the backupstoragelocation is empty")

		builder.errorMsg = "backupstoragelocation objectstorage bucket cannot be empty"
	}

	return builder
}

// PullBackupStorageLocationBuilder pulls existing backupstoragelocation from cluster.
func PullBackupStorageLocationBuilder(
	apiClient *clients.Settings, name, namespace string) (*BackupStorageLocationBuilder, error) {
	klog.V(100).Infof("Pulling existing backupstoragelocation name: %s under namespace: %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.AttachScheme(velerov1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add velero v1 scheme to client schemes")

		return nil, err
	}

	builder := BackupStorageLocationBuilder{
		apiClient: apiClient.Client,
		Definition: &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the backupstoragelocation is empty")

		return nil, fmt.Errorf("backupstoragelocation name cannot be empty")
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the backupstoragelocation is empty")

		return nil, fmt.Errorf("backupstoragelocation namespace cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("backupstoragelocation object %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// WithConfig includes the provided configuration to the backupstoragelocation.
func (builder *BackupStorageLocationBuilder) WithConfig(config map[string]string) *BackupStorageLocationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(config) == 0 {
		klog.V(100).Info("The config of the backupstoragelocation is empty")

		builder.errorMsg = "backupstoragelocation cannot have empty config"

		return builder
	}

	builder.Definition.Spec.Config = config

	return builder
}

// WaitUntilAvailable waits the specified timeout for the backupstoragelocation to become available.
func (builder *BackupStorageLocationBuilder) WaitUntilAvailable(
	timeout time.Duration) (*BackupStorageLocationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		return builder, fmt.Errorf("cannot wait for backupstoragelocation that does not exist")
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			klog.V(100).Infof("Waiting for the backupstoragelocation %s in %s to become available",
				builder.Definition.Name, builder.Definition.Namespace)

			builder.Object, err = builder.Get()
			if err != nil {
				return false, err
			}

			return builder.Object.Status.Phase == velerov1.BackupStorageLocationPhaseAvailable, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, fmt.Errorf("error waiting for backupstoragelocation to become available: %w", err)
}

// WaitUntilUnavailable waits the specified timeout for the backupstoragelocation to become unavailable.
func (builder *BackupStorageLocationBuilder) WaitUntilUnavailable(
	timeout time.Duration) (*BackupStorageLocationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		return builder, fmt.Errorf("cannot wait for backupstoragelocation that does not exist")
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			klog.V(100).Infof("Waiting for the backupstoragelocation %s in %s to become unavailable",
				builder.Definition.Name, builder.Definition.Namespace)

			builder.Object, err = builder.Get()
			if err != nil {
				return false, err
			}

			return builder.Object.Status.Phase == velerov1.BackupStorageLocationPhaseUnavailable, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, fmt.Errorf("error waiting for backupstoragelocation to become unavailable: %w", err)
}

// Get returns Backup object if found.
func (builder *BackupStorageLocationBuilder) Get() (*velerov1.BackupStorageLocation, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting BackupStorageLocation object %s", builder.Definition.Name)

	backupStorageLocation := &velerov1.BackupStorageLocation{}

	err := builder.apiClient.Get(
		context.TODO(),
		goclient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace}, backupStorageLocation)
	if err != nil {
		klog.V(100).Infof("BackupStorageLocation object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return backupStorageLocation, err
}

// Exists checks whether the given backupstoragelocation exists.
func (builder *BackupStorageLocationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if backupstoragelocation %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a backupstoragelocation according to the backupstoragelocation
// definition and stores the created object in the backupstoragelocation builder.
func (builder *BackupStorageLocationBuilder) Create() (*BackupStorageLocationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating backupstoragelocation %s in namespace %s",
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

// Update renovates the existing backupstoragelocation object with the backupstoragelocation definition in builder.
func (builder *BackupStorageLocationBuilder) Update() (*BackupStorageLocationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating backupstoragelocation %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return builder, fmt.Errorf("cannot update non-existent backupstoragelocation")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes the backupstoragelocation object and resets the builder object.
func (builder *BackupStorageLocationBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting backupstoragelocation %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("BackupStorageLocation %s in namespace %s cannot be deleted"+
			" because it does not exist", builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete backupstoragelocation: %w", err)
	}

	builder.Object = nil

	return nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BackupStorageLocationBuilder) validate() (bool, error) {
	resourceCRD := "BackupStorageLocation"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		builder.errorMsg = msg.UndefinedCrdObjectErrString(resourceCRD)
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		builder.errorMsg = fmt.Sprintf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
