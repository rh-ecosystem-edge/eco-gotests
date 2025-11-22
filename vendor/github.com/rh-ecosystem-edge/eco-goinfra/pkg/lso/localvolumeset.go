package lso

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	lsov1 "github.com/openshift/local-storage-operator/api/v1"
	lsov1alpha1 "github.com/openshift/local-storage-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// LocalVolumeSetBuilder provides a struct for localVolumeSet object from the cluster and a localVolumeSet definition.
type LocalVolumeSetBuilder struct {
	// localVolumeSet definition, used to create the localVolumeSet object.
	Definition *lsov1alpha1.LocalVolumeSet
	// Created localVolumeSet object.
	Object *lsov1alpha1.LocalVolumeSet
	// Used in functions that define or mutate localVolumeSet definition. errorMsg is processed
	// before the localVolumeSet object is created
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewLocalVolumeSetBuilder creates new instance of LocalVolumeSetBuilder.
func NewLocalVolumeSetBuilder(apiClient *clients.Settings, name, nsname string) *LocalVolumeSetBuilder {
	klog.V(100).Infof("Initializing new localVolumeSet %s structure in namespace %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("localVolumeSet 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(lsov1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add lsov1alpha1 scheme to client schemes")

		return nil
	}

	builder := &LocalVolumeSetBuilder{
		apiClient: apiClient.Client,
		Definition: &lsov1alpha1.LocalVolumeSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the localVolumeSet is empty")

		builder.errorMsg = "localVolumeSet 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the localVolumeSet is empty")

		builder.errorMsg = "localVolumeSet 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullLocalVolumeSet retrieves an existing localVolumeSet object from the cluster.
func PullLocalVolumeSet(apiClient *clients.Settings, name, nsname string) (*LocalVolumeSetBuilder, error) {
	klog.V(100).Infof(
		"Pulling localVolumeSet object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("localVolumeSet 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(lsov1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add lsov1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := LocalVolumeSetBuilder{
		apiClient: apiClient.Client,
		Definition: &lsov1alpha1.LocalVolumeSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the localVolumeSet is empty")

		return nil, fmt.Errorf("localVolumeSet 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the localVolumeSet is empty")

		return nil, fmt.Errorf("localVolumeSet 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("localVolumeSet object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get fetches existing localVolumeSet from cluster.
func (builder *LocalVolumeSetBuilder) Get() (*lsov1alpha1.LocalVolumeSet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Pulling existing localVolumeSet with name %s under namespace %s from cluster",
		builder.Definition.Name, builder.Definition.Namespace)

	lvs := &lsov1alpha1.LocalVolumeSet{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, lvs)
	if err != nil {
		return nil, err
	}

	return lvs, nil
}

// Create makes a LocalVolumeSetBuilder in the cluster and stores the created object in struct.
func (builder *LocalVolumeSetBuilder) Create() (*LocalVolumeSetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the LocalVolumeSetBuilder %s in namespace %s",
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

// Delete removes localVolumeSet from a cluster.
func (builder *LocalVolumeSetBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Info("localVolumeSet cannot be deleted because it does not exist")

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete localVolumeSet: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given localVolumeSet exists.
func (builder *LocalVolumeSetBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if localVolumeSet %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates a LocalVolumeSetBuilder in the cluster and stores the created object in struct.
func (builder *LocalVolumeSetBuilder) Update() (*LocalVolumeSetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("localVolumeSet object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	builder.Definition.CreationTimestamp = metav1.Time{}
	builder.Definition.ResourceVersion = ""

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("localVolumeSet", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithTolerations sets the localVolumeSet's tolerations.
func (builder *LocalVolumeSetBuilder) WithTolerations(
	tolerations []corev1.Toleration) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding tolerations %v to localVolumeSet %s in namespace %s",
		tolerations, builder.Definition.Name, builder.Definition.Namespace)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(tolerations) == 0 {
		klog.V(100).Info("The tolerations list is empty")

		builder.errorMsg = "'tolerations' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Tolerations = tolerations

	return builder
}

// WithNodeSelector sets the localVolumeSet's nodeSelector.
func (builder *LocalVolumeSetBuilder) WithNodeSelector(
	nodeSelector corev1.NodeSelector) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding nodeSelector %v to localVolumeSet %s in namespace %s",
		nodeSelector, builder.Definition.Name, builder.Definition.Namespace)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.NodeSelector = &nodeSelector

	return builder
}

// WithStorageClassName sets the localVolumeSet's storageClassName.
func (builder *LocalVolumeSetBuilder) WithStorageClassName(
	storageClassName string) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding storageClassName %s to localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, storageClassName)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if storageClassName == "" {
		klog.V(100).Info("The storageClassName is empty")

		builder.errorMsg = "'storageClassName' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.StorageClassName = storageClassName

	return builder
}

// WithVolumeMode sets the localVolumeSet's volumeMode.
func (builder *LocalVolumeSetBuilder) WithVolumeMode(
	volumeMode lsov1.PersistentVolumeMode) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding volumeMode %v to localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, volumeMode)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if volumeMode == "" {
		klog.V(100).Info("The volumeMode is empty")

		builder.errorMsg = "'volumeMode' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.VolumeMode = volumeMode

	return builder
}

// WithFSType sets the localVolumeSet's fstype.
func (builder *LocalVolumeSetBuilder) WithFSType(
	fstype string) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding fstype %s to localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, fstype)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if fstype == "" {
		klog.V(100).Info("The fstype is empty")

		builder.errorMsg = "'fstype' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.FSType = fstype

	return builder
}

// WithMaxDeviceCount sets the localVolumeSet's maxDeviceCount.
func (builder *LocalVolumeSetBuilder) WithMaxDeviceCount(
	maxDeviceCount int32) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding maxDeviceCount %v to localVolumeSet %s in namespace %s",
		maxDeviceCount, builder.Definition.Name, builder.Definition.Namespace)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if maxDeviceCount == int32(0) {
		klog.V(100).Info("The maxDeviceCount is zero")

		builder.errorMsg = "'maxDeviceCount' argument cannot be equal zero"

		return builder
	}

	builder.Definition.Spec.MaxDeviceCount = &maxDeviceCount

	return builder
}

// WithDeviceInclusionSpec sets the localVolumeSet's deviceInclusionSpec.
func (builder *LocalVolumeSetBuilder) WithDeviceInclusionSpec(
	deviceInclusionSpec lsov1alpha1.DeviceInclusionSpec) *LocalVolumeSetBuilder {
	klog.V(100).Infof(
		"Adding deviceInclusionSpec %v to localVolumeSet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, deviceInclusionSpec)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.DeviceInclusionSpec = &deviceInclusionSpec

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *LocalVolumeSetBuilder) validate() (bool, error) {
	resourceCRD := "LocalVolumeSet"

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
