package storage

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/exp/slices"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

var validPVCModesMap = map[string]string{
	"ReadWriteOnce":    "ReadWriteOnce",
	"ReadOnlyMany":     "ReadOnlyMany",
	"ReadWriteMany":    "ReadWriteMany",
	"ReadWriteOncePod": "ReadWriteOncePod",
}

// PVCBuilder provides struct for persistentvolumeclaim object containing connection
// to the cluster and the persistentvolumeclaim definitions.
type PVCBuilder struct {
	// PersistentVolumeClaim definition. Used to create a persistentvolumeclaim object
	Definition *corev1.PersistentVolumeClaim
	// Created persistentvolumeclaim object
	Object *corev1.PersistentVolumeClaim

	errorMsg  string
	apiClient *clients.Settings
}

// NewPVCBuilder creates a new structure for persistentvolumeclaim.
func NewPVCBuilder(apiClient *clients.Settings, name, nsname string) *PVCBuilder {
	glog.V(100).Infof("Creating PersistentVolumeClaim %s in namespace %s",
		name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("storageSystem 'apiClient' cannot be empty")

		return nil
	}

	builder := &PVCBuilder{
		Definition: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: corev1.PersistentVolumeClaimSpec{},
		},
	}

	builder.apiClient = apiClient

	if name == "" {
		glog.V(100).Infof("PVC name is empty")

		builder.errorMsg = "PVC name is empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("PVC namespace is empty")

		builder.errorMsg = "PVC namespace is empty"

		return builder
	}

	return builder
}

// WithPVCAccessMode configure access mode for the PV.
func (builder *PVCBuilder) WithPVCAccessMode(accessMode string) (*PVCBuilder, error) {
	glog.V(100).Infof("Set PVC accessMode: %s", accessMode)

	if accessMode == "" {
		glog.V(100).Infof("Empty accessMode for PVC %s", builder.Definition.Name)

		return builder, fmt.Errorf("empty accessMode for PVC requested")
	}

	if !validatePVCAccessMode(accessMode) {
		glog.V(100).Infof("Invalid accessMode for PVC %s", accessMode)

		return builder, fmt.Errorf("invalid accessMode for PVC %s", accessMode)
	}

	if builder.Definition.Spec.AccessModes != nil {
		builder.Definition.Spec.AccessModes = append(builder.Definition.Spec.AccessModes,
			corev1.PersistentVolumeAccessMode(accessMode))
	} else {
		builder.Definition.Spec.AccessModes =
			[]corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(accessMode)}
	}

	return builder, nil
}

// validatePVCAccessMode validates if requested mode is valid for PVC.
func validatePVCAccessMode(accessMode string) bool {
	glog.V(100).Info("Validating accessMode %s", accessMode)

	_, ok := validPVCModesMap[accessMode]

	return ok
}

// WithPVCCapacity configures the minimum resources the volume should have.
func (builder *PVCBuilder) WithPVCCapacity(capacity string) (*PVCBuilder, error) {
	if capacity == "" {
		glog.V(100).Infof("Capacity of the PersistentVolumeClaim is empty")

		return builder, fmt.Errorf("capacity of the PersistentVolumeClaim is empty")
	}

	defer func() (*PVCBuilder, error) {
		if r := recover(); r != nil {
			glog.V(100).Infof("Failed to parse %v", capacity)

			return builder, fmt.Errorf("failed to parse: %v", capacity)
		}

		return builder, nil
	}() //nolint:errcheck

	capMap := make(map[corev1.ResourceName]resource.Quantity)
	capMap[corev1.ResourceStorage] = resource.MustParse(capacity)

	builder.Definition.Spec.Resources = corev1.VolumeResourceRequirements{Requests: capMap}

	return builder, nil
}

// WithStorageClass configures storageClass required by the claim.
func (builder *PVCBuilder) WithStorageClass(storageClass string) (*PVCBuilder, error) {
	glog.V(100).Infof("Set storage class %s for the PersistentVolumeClaim", storageClass)

	if storageClass == "" {
		glog.V(100).Infof("Empty storageClass requested for the PersistentVolumeClaim", storageClass)

		return builder, fmt.Errorf("empty storageClass requested for the PersistentVolumeClaim %s",
			builder.Definition.Name)
	}

	builder.Definition.Spec.StorageClassName = &storageClass

	return builder, nil
}

// WithVolumeMode configures what type of volume is required by the claim.
func (builder *PVCBuilder) WithVolumeMode(volumeMode string) (*PVCBuilder, error) {
	glog.V(100).Infof("Set VolumeMode %s for the PersistentVolumeClaim", volumeMode)

	if volumeMode == "" {
		glog.V(100).Infof(fmt.Sprintf("Empty volumeMode requested for the PersistentVolumeClaim %s in %s namespace",
			builder.Definition.Name, builder.Definition.Namespace))

		return builder, fmt.Errorf("empty volumeMode requested for the PersistentVolumeClaim %s in %s namespace",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	if !validateVolumeMode(volumeMode) {
		glog.V(100).Infof(fmt.Sprintf("Unsupported VolumeMode: %s", volumeMode))

		return builder, fmt.Errorf("unsupported VolumeMode %q requested for %s PersistentVolumeClaim in %s namespace",
			volumeMode, builder.Definition.Name, builder.Definition.Name)
	}

	// volumeMode is string while Spec.VolumeMode requires pointer to corev1.PersistentVolumeMode,
	// therefore temporary variable strVolMode is created to be used within assignment.
	strVolMode := corev1.PersistentVolumeMode(volumeMode)

	builder.Definition.Spec.VolumeMode = &strVolMode

	return builder, nil
}

// Create generates a PVC in cluster and stores the created object in struct.
func (builder *PVCBuilder) Create() (*PVCBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating persistentVolumeClaim %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.PersistentVolumeClaims(builder.Definition.Namespace).Create(
			context.TODO(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes PVC from cluster.
func (builder *PVCBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		glog.V(100).Infof("PersistentVolumeClaim %s in %s namespace is invalid: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return err
	}

	glog.V(100).Infof("Delete PersistentVolumeClaim %s from %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("PersistentVolumeClaim %s not found in %s namespace",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.PersistentVolumeClaims(builder.Definition.Namespace).Delete(
		context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		glog.V(100).Infof("Failed to delete PersistentVolumeClaim %s from %s namespace",
			builder.Definition.Name, builder.Definition.Namespace)
		glog.V(100).Infof("PersistenteVolumeClaim deletion error: %v", err)

		return err
	}

	glog.V(100).Infof("Deleted PersistentVolumeClaim %s from %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes PersistentVolumeClaim and waits until it is removed from the cluster.
func (builder *PVCBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		glog.V(100).Infof("PersistentVolumeClaim %s in %s namespace is invalid: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return err
	}

	glog.V(100).Infof("Deleting PersistenVolumeClaim %s from %s namespace and waiting for the removal to complete",
		builder.Definition.Name, builder.Definition.Namespace)

	if err := builder.Delete(); err != nil {
		glog.V(100).Infof("Failed to delete PersistentVolumeClaim %s from %s namespace",
			builder.Definition.Name, builder.Definition.Namespace)
		glog.V(100).Infof("PersistenteVolumeClaim deletion error: %v", err)

		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.apiClient.PersistentVolumeClaims(builder.Definition.Namespace).Get(
				context.TODO(), builder.Definition.Name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})
}

// PullPersistentVolumeClaim gets an existing PersistentVolumeClaim
// from the cluster.
func PullPersistentVolumeClaim(
	apiClient *clients.Settings, name string, nsname string) (
	*PVCBuilder, error) {
	glog.V(100).Infof("Pulling existing PersistentVolumeClaim object: %s from namespace %s",
		name, nsname)

	if apiClient == nil {
		glog.V(100).Info("The PersistentVolumeClaim apiClient is nil")

		return nil, fmt.Errorf("persistentVolumeClaim 'apiClient' cannot be empty")
	}

	builder := &PVCBuilder{
		apiClient: apiClient,
		Definition: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the PersistentVolumeClaim is empty")

		return nil, fmt.Errorf("persistentVolumeClaim 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Info("The namespace of the PersistentVolumeClaim is empty")

		return nil, fmt.Errorf("persistentVolumeClaim 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("persistentVolumeClaim object %s does not exist in namespace %s",
			name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given PersistentVolumeClaim exists.
func (builder *PVCBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if PersistentVolumeClaim %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.PersistentVolumeClaims(builder.Definition.Namespace).Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PVCBuilder) validate() (bool, error) {
	resourceCRD := "PersistentVolumeClaim"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}

// GetPersistentVolumeClaimGVR returns the GroupVersionResource for the PersistentVolumeClaim.
func GetPersistentVolumeClaimGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "persistentvolumeclaims",
	}
}

// validateVolumeMode validates if requested volume mode is valid for PVC.
func validateVolumeMode(volumeMode string) bool {
	glog.V(100).Info("Validating volumeMode %s", volumeMode)

	var validVolumeModes = []string{
		"Block",
		"Filesystem",
	}

	return slices.Contains(validVolumeModes, volumeMode)
}
