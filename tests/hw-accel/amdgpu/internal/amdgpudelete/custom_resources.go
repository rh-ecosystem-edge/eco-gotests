package amdgpudelete

import (
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	amdgpuv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/amd/gpu-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AMDGPUCustomResourceCleaner implements CustomResourceCleaner for AMD GPU operators.
type AMDGPUCustomResourceCleaner struct {
	APIClient *clients.Settings
	Namespace string
	LogLevel  glog.Level
}

// NewAMDGPUCustomResourceCleaner creates a new AMD GPU custom resource cleaner.
func NewAMDGPUCustomResourceCleaner(
	apiClient *clients.Settings,
	namespace string,
	logLevel glog.Level) *AMDGPUCustomResourceCleaner {
	return &AMDGPUCustomResourceCleaner{
		APIClient: apiClient,
		Namespace: namespace,
		LogLevel:  logLevel,
	}
}

// CleanupCustomResources implements the CustomResourceCleaner interface for AMD GPU.
func (a *AMDGPUCustomResourceCleaner) CleanupCustomResources() error {
	glog.V(a.LogLevel).Infof("Deleting AMD GPU custom resources in namespace %s", a.Namespace)

	deviceConfigList, err := a.listDeviceConfigs()
	if err != nil {
		return err
	}

	if deviceConfigList == nil || len(deviceConfigList.Items) == 0 {
		glog.V(a.LogLevel).Infof("No AMD GPU custom resources found to delete")

		return nil
	}

	deletedCount := a.deleteDeviceConfigs(deviceConfigList.Items)

	err = a.waitForDeviceConfigCleanup()
	if err != nil {
		glog.V(a.LogLevel).Infof("Timeout waiting for AMD GPU DeviceConfigs removal: %v", err)
	}

	glog.V(a.LogLevel).Infof("Successfully cleaned up %d AMD GPU custom resources", deletedCount)

	return nil
}

// listDeviceConfigs retrieves all DeviceConfig resources in the namespace.
func (a *AMDGPUCustomResourceCleaner) listDeviceConfigs() (*amdgpuv1.DeviceConfigList, error) {
	ctx := context.Background()
	deviceConfigList := &amdgpuv1.DeviceConfigList{}

	glog.V(a.LogLevel).Infof("Looking for AMD GPU DeviceConfigs in namespace: %s", a.Namespace)
	err := a.APIClient.Client.List(ctx, deviceConfigList, client.InNamespace(a.Namespace))

	if err != nil {
		if amdgpucommon.IsCRDNotAvailable(err) {
			glog.V(a.LogLevel).Info("AMD GPU DeviceConfig CRD not available - skipping DeviceConfig cleanup")

			return &amdgpuv1.DeviceConfigList{}, nil
		}

		glog.V(a.LogLevel).Infof("Error listing AMD GPU DeviceConfigs: %v", err)

		return nil, err
	}

	glog.V(a.LogLevel).Infof("Found %d AMD GPU DeviceConfig(s) to delete", len(deviceConfigList.Items))

	return deviceConfigList, nil
}

// deleteDeviceConfigs deletes all DeviceConfig resources and returns the count of deleted resources.
func (a *AMDGPUCustomResourceCleaner) deleteDeviceConfigs(deviceConfigs []amdgpuv1.DeviceConfig) int {
	ctx := context.Background()
	deletedCount := 0

	for _, deviceConfig := range deviceConfigs {
		if a.deleteDeviceConfig(ctx, &deviceConfig) {
			deletedCount++
		}
	}

	return deletedCount
}

// deleteDeviceConfig deletes a single DeviceConfig resource.
func (a *AMDGPUCustomResourceCleaner) deleteDeviceConfig(
	ctx context.Context, deviceConfig *amdgpuv1.DeviceConfig) bool {
	deviceConfigName := deviceConfig.GetName()
	glog.V(a.LogLevel).Infof("Deleting AMD GPU DeviceConfig: %s", deviceConfigName)

	if len(deviceConfig.GetFinalizers()) > 0 {
		a.removeFinalizers(ctx, deviceConfig, deviceConfigName)
	}

	err := a.APIClient.Client.Delete(ctx, deviceConfig)
	if err != nil {
		glog.V(a.LogLevel).Infof("Error deleting AMD GPU DeviceConfig %s: %v", deviceConfigName, err)

		return false
	}

	glog.V(a.LogLevel).Infof("Successfully deleted AMD GPU DeviceConfig: %s", deviceConfigName)

	return true
}

// removeFinalizers removes finalizers from a DeviceConfig to ensure clean deletion.
func (a *AMDGPUCustomResourceCleaner) removeFinalizers(
	ctx context.Context, deviceConfig *amdgpuv1.DeviceConfig, deviceConfigName string) {
	glog.V(a.LogLevel).Infof("Removing finalizers from AMD GPU DeviceConfig: %s", deviceConfigName)
	deviceConfig.SetFinalizers([]string{})
	err := a.APIClient.Client.Update(ctx, deviceConfig)

	if err != nil {
		glog.V(a.LogLevel).Infof("Warning: failed to remove finalizers from DeviceConfig %s: %v", deviceConfigName, err)
	}
}

// waitForDeviceConfigCleanup waits for all DeviceConfig resources to be completely removed.
func (a *AMDGPUCustomResourceCleaner) waitForDeviceConfigCleanup() error {
	ctx := context.Background()

	glog.V(a.LogLevel).Info("Waiting for AMD GPU DeviceConfigs to be fully removed...")

	return wait.PollUntilContextTimeout(
		ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			return a.checkDeviceConfigRemoval(ctx)
		})
}

// checkDeviceConfigRemoval checks if all DeviceConfig resources have been removed.
func (a *AMDGPUCustomResourceCleaner) checkDeviceConfigRemoval(ctx context.Context) (bool, error) {
	currentList := &amdgpuv1.DeviceConfigList{}

	err := a.APIClient.Client.List(ctx, currentList, client.InNamespace(a.Namespace))
	if err != nil {
		return false, nil
	}

	if len(currentList.Items) == 0 {
		glog.V(a.LogLevel).Info("All AMD GPU DeviceConfigs successfully removed")

		return true, nil
	}

	glog.V(a.LogLevel).Infof("Still waiting for %d AMD GPU DeviceConfigs to be removed", len(currentList.Items))

	return false, nil
}

// Interface verification.
var _ deploy.CustomResourceCleaner = (*AMDGPUCustomResourceCleaner)(nil)
