package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// PVBuilder provides struct for persistentvolume object containing connection
// to the cluster and the persistentvolume definitions.
type PVBuilder struct {
	// PersistentVolume definition. Used to create a persistentvolume object
	Definition *corev1.PersistentVolume
	// Created persistentvolume object
	Object *corev1.PersistentVolume
	// api client to interact with the cluster.
	apiClient *clients.Settings
	// Used in functions that define or mutate storageSystem definition. errorMsg is processed before the
	// storageSystem object is created.
	errorMsg string
}

// PullPersistentVolume gets an existing PersistentVolume from the cluster.
func PullPersistentVolume(apiClient *clients.Settings, name string) (*PVBuilder, error) {
	glog.V(100).Infof("Pulling existing PersistentVolume object: %s", name)

	if apiClient == nil {
		glog.V(100).Info("The PersistentVolume apiClient is nil")

		return nil, fmt.Errorf("persistentVolume 'apiClient' cannot be empty")
	}

	builder := &PVBuilder{
		apiClient: apiClient,
		Definition: &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the PersistentVolume is empty")

		return nil, fmt.Errorf("persistentVolume 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("PersistentVolume object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given PersistentVolume exists.
func (builder *PVBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if PersistentVolume %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.apiClient.PersistentVolumes().Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a PersistentVolume from the apiClient if it exists.
func (builder *PVBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the PersistentVolume %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("PersistentVolume %s cannot be deleted because it does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.PersistentVolumes().Delete(context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes the PersistentVolume and waits up to timeout until it has been removed.
func (builder *PVBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Deleting PersistentVolume %s and waiting up to %s until it is removed", timeout, builder.Definition.Name)

	err := builder.Delete()
	if err != nil {
		return err
	}

	return builder.WaitUntilDeleted(timeout)
}

// WaitUntilDeleted waits for the duration of timeout or until the PersistentVolume has been deleted.
func (builder *PVBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Waiting up to %s until PersistentVolume %s is deleted", timeout, builder.Definition.Name)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.apiClient.PersistentVolumes().Get(context.TODO(), builder.Definition.Name, metav1.GetOptions{})
			if err == nil {
				glog.V(100).Infof("PersistentVolume %s still present", builder.Definition.Name)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				glog.V(100).Infof("PersistentVolume %s is gone", builder.Definition.Name)

				return true, nil
			}

			glog.V(100).Infof("failed to get PersistentVolume %s", builder.Definition.Name)

			return false, err
		})
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PVBuilder) validate() (bool, error) {
	resourceCRD := "PersistentVolume"

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
