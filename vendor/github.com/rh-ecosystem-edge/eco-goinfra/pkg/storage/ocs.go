package storage

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	ocsoperatorv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocs/operatorv1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// StorageClusterBuilder provides struct for StorageCluster object containing connection
// to the cluster and the storageCluster definitions.
type StorageClusterBuilder struct {
	// StorageCluster definition. Used to create a storageCluster object
	Definition *ocsoperatorv1.StorageCluster
	// Created storageCluster object
	Object *ocsoperatorv1.StorageCluster
	// api client to interact with the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate storageCluster definition. errorMsg is processed before the
	// storageCluster object is created.
	errorMsg string
}

// NewStorageClusterBuilder creates a new instance of StorageClusterBuilder.
func NewStorageClusterBuilder(apiClient *clients.Settings, name, nsname string) *StorageClusterBuilder {
	klog.V(100).Infof(
		"Initializing new storageCluster structure with the following params: %s, %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("storageCluster 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(ocsoperatorv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ocs-operator v1 scheme to client schemes")

		return nil
	}

	builder := &StorageClusterBuilder{
		apiClient: apiClient.Client,
		Definition: &ocsoperatorv1.StorageCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the storageCluster is empty")

		builder.errorMsg = "storageCluster 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the storageCluster is empty")

		builder.errorMsg = "storageCluster 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullStorageCluster gets an existing storageCluster object from the cluster.
func PullStorageCluster(apiClient *clients.Settings, name, namespace string) (*StorageClusterBuilder, error) {
	klog.V(100).Infof("Pulling existing storageCluster object %s from namespace %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("storageCluster 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(ocsoperatorv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ocs-operator v1 scheme to client schemes")

		return nil, err
	}

	builder := &StorageClusterBuilder{
		apiClient: apiClient.Client,
		Definition: &ocsoperatorv1.StorageCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the storageCluster is empty")

		return nil, fmt.Errorf("storageCluster 'name' cannot be empty")
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the storageCluster is empty")

		return nil, fmt.Errorf("storageCluster 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing storageCluster from cluster.
func (builder *StorageClusterBuilder) Get() (*ocsoperatorv1.StorageCluster, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting existing storageCluster with name %s from the namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	storageClusterObj := &ocsoperatorv1.StorageCluster{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, storageClusterObj)
	if err != nil {
		klog.V(100).Infof("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return storageClusterObj, nil
}

// Exists checks whether the given storageCluster exists.
func (builder *StorageClusterBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if storageCluster %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a storageCluster in the cluster and stores the created object in struct.
func (builder *StorageClusterBuilder) Create() (*StorageClusterBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the storageCluster %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes storageCluster object from a cluster.
func (builder *StorageClusterBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the storageCluster object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("storageCluster %s in namespace %s cannot be deleted"+
			" because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete storageCluster: %w", err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the storageCluster in the cluster and stores the created object in struct.
func (builder *StorageClusterBuilder) Update() (*StorageClusterBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the storageCluster %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("storageCluster", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// GetManagedResources fetches storageCluster managedResources value.
func (builder *StorageClusterBuilder) GetManagedResources() (*ocsoperatorv1.ManagedResourcesSpec, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting storageCluster %s in namespace %s managedResources configuration",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return &builder.Object.Spec.ManagedResources, nil
}

// GetMonDataDirHostPath fetches storageCluster monDataDirHostPath value.
func (builder *StorageClusterBuilder) GetMonDataDirHostPath() (string, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	klog.V(100).Infof("Getting storageCluster %s in namespace %s monDataDirHostPath configuration",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return "", fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return builder.Object.Spec.MonDataDirHostPath, nil
}

// GetMultiCloudGateway fetches storageCluster multiCloudGateway value.
func (builder *StorageClusterBuilder) GetMultiCloudGateway() (*ocsoperatorv1.MultiCloudGatewaySpec, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting storageCluster %s in namespace %s multiCloudGateway configuration",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return builder.Object.Spec.MultiCloudGateway, nil
}

// GetStorageDeviceSets fetches storageCluster storageDeviceSets value.
func (builder *StorageClusterBuilder) GetStorageDeviceSets() ([]ocsoperatorv1.StorageDeviceSet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting storageCluster %s in namespace %s storageDeviceSets configuration",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("storageCluster object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return builder.Object.Spec.StorageDeviceSets, nil
}

// WithFlexibleScaling sets the storageCluster's flexibleScaling value.
func (builder *StorageClusterBuilder) WithFlexibleScaling(flexibleScaling bool) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with flexibleScaling value: %t",
		builder.Definition.Name, builder.Definition.Namespace, flexibleScaling)

	builder.Definition.Spec.FlexibleScaling = flexibleScaling

	return builder
}

// WithManagedResources sets the storageCluster's managedResources value.
func (builder *StorageClusterBuilder) WithManagedResources(
	expectedManagedResources ocsoperatorv1.ManagedResourcesSpec) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with managedResources value: %v",
		builder.Definition.Name, builder.Definition.Namespace, expectedManagedResources)

	builder.Definition.Spec.ManagedResources = expectedManagedResources

	return builder
}

// WithMonDataDirHostPath sets the storageCluster's monDataDirHostPath value.
func (builder *StorageClusterBuilder) WithMonDataDirHostPath(
	expectedMonDataDirHostPath string) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with monDataDirHostPath value: %s",
		builder.Definition.Name, builder.Definition.Namespace, expectedMonDataDirHostPath)

	if expectedMonDataDirHostPath == "" {
		klog.V(100).Info("the expectedMonDataDirHostPath can not be empty")

		builder.errorMsg = "the expectedMonDataDirHostPath can not be empty"

		return builder
	}

	builder.Definition.Spec.MonDataDirHostPath = expectedMonDataDirHostPath

	return builder
}

// WithMultiCloudGateway sets the storageCluster's multiCloudGateway value.
func (builder *StorageClusterBuilder) WithMultiCloudGateway(
	expectedMultiCloudGateway ocsoperatorv1.MultiCloudGatewaySpec) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with multiCloudGateway value: %v",
		builder.Definition.Name, builder.Definition.Namespace, expectedMultiCloudGateway)

	builder.Definition.Spec.MultiCloudGateway = &expectedMultiCloudGateway

	return builder
}

// WithStorageDeviceSet sets the storageCluster's storageDeviceSets value.
func (builder *StorageClusterBuilder) WithStorageDeviceSet(
	expectedStorageDeviceSet ocsoperatorv1.StorageDeviceSet) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with storageDeviceSets value: %v",
		builder.Definition.Name, builder.Definition.Namespace, expectedStorageDeviceSet)

	builder.Definition.Spec.StorageDeviceSets =
		append(builder.Definition.Spec.StorageDeviceSets, expectedStorageDeviceSet)

	return builder
}

// WithAnnotations sets the storageCluster's annotations value.
func (builder *StorageClusterBuilder) WithAnnotations(
	annotations map[string]string) *StorageClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting storageCluster %s in namespace %s with annotations values: %v",
		builder.Definition.Name, builder.Definition.Namespace, annotations)

	if len(annotations) == 0 {
		klog.V(100).Info("'annotations' argument cannot be empty")

		builder.errorMsg = "'annotations' argument cannot be empty"

		return builder
	}

	builder.Definition.Annotations = annotations

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *StorageClusterBuilder) validate() (bool, error) {
	resourceCRD := "StorageCluster"

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
