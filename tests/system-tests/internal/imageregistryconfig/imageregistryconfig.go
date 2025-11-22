package imageregistryconfig

import (
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	"k8s.io/klog/v2"

	imageregistryV1 "github.com/openshift/api/imageregistry/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/apiservers"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/imageregistry"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
)

var imageRegistryObjName = "cluster"
var imageRegistryNamespace = "openshift-image-registry"
var imageRegistryCoName = "image-registry"
var imageRegistryDeploymentName = "image-registry"

// SetManagementState returns true when succeeded to change imageRegistry operator management state.
func SetManagementState(apiClient *clients.Settings, expectedManagementState operatorv1.ManagementState) error {
	irConfigObj, err := imageregistry.Pull(apiClient, imageRegistryObjName)
	if err != nil {
		klog.V(100).Infof("Failed to get imageRegistry operator due to %v",
			err.Error())

		return err
	}

	klog.V(100).Infof("Set imageRegistry %s ManagementState to the %v",
		irConfigObj.Definition.Name, expectedManagementState)

	currentManagementState, err := irConfigObj.GetManagementState()
	if err != nil {
		klog.V(100).Infof("Failed to get current imageRegistry operator management state value due to %v",
			err.Error())

		return err
	}

	if *currentManagementState != expectedManagementState {
		klog.V(100).Infof("The current imageRegistry %s ManagementState is %v; it needs to be changed to the %v",
			irConfigObj.Definition.Name, currentManagementState, expectedManagementState)

		irConfig, err := irConfigObj.WithManagementState(expectedManagementState).Update()
		if err != nil {
			klog.V(100).Infof("Failed to make change to the imageRegistry operator managementState due to %v", err)

			return err
		}

		newManagementState, err := irConfig.GetManagementState()
		if err != nil {
			klog.V(100).Infof("Failed to get current imageRegistry operator managementState value due to %v", err)

			return err
		}

		if *newManagementState != expectedManagementState {
			return fmt.Errorf("failed to change imageRegistry operator managementState value;"+
				"expected %v, current value is %v", expectedManagementState, newManagementState)
		}

		return nil
	}

	return err
}

// SetStorageToTheEmptyDir sets the imageRegistry storage to an empty directory.
func SetStorageToTheEmptyDir(apiClient *clients.Settings) error {
	irClusterOperator, err := clusteroperator.Pull(apiClient, imageRegistryCoName)
	if err != nil {
		return err
	}

	if !irClusterOperator.IsDegraded() {
		err = await.WaitUntilDeploymentReady(
			apiClient,
			imageRegistryDeploymentName,
			imageRegistryNamespace,
			time.Second*2)
		if err == nil {
			return nil
		}
	}

	klog.V(100).Infof("Setting up imageRegistry storage to the EmptyDir")

	imageRegistryObj, err := imageregistry.Pull(apiClient, imageRegistryObjName)
	if err != nil {
		return err
	}

	emptyDirStorage := imageregistryV1.ImageRegistryConfigStorage{
		EmptyDir:        &imageregistryV1.ImageRegistryConfigStorageEmptyDir{},
		ManagementState: imageregistryV1.StorageManagementStateManaged,
	}

	irConfig, err := imageRegistryObj.WithStorage(emptyDirStorage).Update()
	if err != nil {
		klog.V(100).Infof("Failed to change an imageRegistryObj config and setup storage to the EmptyDir")

		return err
	}

	newStorageConfig, err := irConfig.GetStorageConfig()
	if err != nil {
		klog.V(100).Infof("Failed to get current imageRegistry Storage configuration due to %v", err)

		return err
	}

	if *newStorageConfig != emptyDirStorage {
		return fmt.Errorf("failed to change imageRegistry Storage configuration;"+
			"expected %v, current value is %v", emptyDirStorage, newStorageConfig)
	}

	err = WaitForAPIServersUpdate(apiClient)
	if err != nil {
		return err
	}

	klog.V(100).Info("Wait for the image-registry deployment succeeded")

	err = await.WaitUntilDeploymentReady(
		apiClient,
		imageRegistryDeploymentName,
		imageRegistryNamespace,
		time.Minute*5)
	if err != nil {
		klog.V(100).Infof("image-registry deployment failure due to %s", err.Error())

		return err
	}

	err = WaitForImageregistryCoIsAvailable(apiClient)
	if err != nil {
		return err
	}

	return nil
}

// WaitForAPIServersUpdate waits for the openshiftapiserver and kubeapiserver update finished.
func WaitForAPIServersUpdate(apiClient *clients.Settings) error {
	klog.V(100).Info("Wait for the openshiftapiserver APIServerDeploymentProgressing ending, " +
		"pods have to be updated to the latest generation")

	oasBuilder, err := apiservers.PullOpenshiftAPIServer(apiClient)
	if err != nil {
		return err
	}

	err = oasBuilder.WaitAllPodsAtTheLatestGeneration(time.Minute * 10)
	if err != nil {
		klog.V(100).Infof("Failed to update openshiftapiserver due to: %v", err)

		return err
	}

	klog.V(100).Info("Wait for the kubeapiserver NodeInstallerProgressing ending, " +
		"nodes have to be updated to the latest revision")

	kasBuilder, err := apiservers.PullKubeAPIServer(apiClient)
	if err != nil {
		return err
	}

	err = kasBuilder.WaitAllNodesAtTheLatestRevision(time.Minute * 15)
	if err != nil {
		return err
	}

	return nil
}

// WaitForImageregistryCoIsAvailable verifies imageregistryconfig co is Available.
func WaitForImageregistryCoIsAvailable(apiClient *clients.Settings) error {
	klog.V(100).Infof("Asserting clusteroperators availability")

	imageRegistryCo, err := clusteroperator.Pull(apiClient, imageRegistryCoName)
	if err != nil {
		return err
	}

	if imageRegistryCo.IsAvailable() {
		return nil
	}

	err = imageRegistryCo.WaitUntilAvailable(time.Minute * 2)
	if err != nil {
		return err
	}

	return nil
}
