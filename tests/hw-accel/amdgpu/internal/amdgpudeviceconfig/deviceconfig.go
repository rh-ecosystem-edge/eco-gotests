package amdgpudeviceconfig

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/amdgpu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	amdgpuv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/amd/gpu-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// AMDGPUDeviceIDs contains the supported AMD GPU device IDs.
var AMDGPUDeviceIDs = []string{
	"75a3",
	"75a0",
	"74a5",
	"74a0",
	"74a1",
	"74a9",
	"74bd",
	"740f",
	"7408",
	"740c",
	"738c",
	"738e",
}

// AMDVGPUDeviceIDs contains the supported AMD vGPU device IDs.
var AMDVGPUDeviceIDs = []string{
	"75b3",
	"75b0",
	"74b9",
	"74b5",
	"7410",
}

// CreateDeviceConfig creates the DeviceConfig custom resource to trigger AMD GPU driver installation.
// It includes retry logic to wait for the CRD to become available after operator installation.
func CreateDeviceConfig(apiClient *clients.Settings, deviceConfigName, driverVersion string) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Creating DeviceConfig: %s", deviceConfigName)

	// Wait for CRD to be available with retry logic
	var (
		deviceConfigBuilder *amdgpu.Builder
		lastErr             error
	)

	crdWaitTimeout := 5 * time.Minute
	retryInterval := 10 * time.Second

	klog.V(amdgpuparams.AMDGPULogLevel).Infof(
		"Waiting for AMD GPU CRD to be available (timeout: %s)", crdWaitTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), crdWaitTimeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(
		ctx,
		retryInterval,
		crdWaitTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var builderErr error

			deviceConfigBuilder, builderErr = createDeviceConfigBuilder(apiClient, deviceConfigName, driverVersion)
			if builderErr != nil {
				// Check if it's a transient API error (connection issues, CRD not ready)
				if isTransientAPIError(builderErr) {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"CRD not ready yet, retrying: %v", builderErr)
					lastErr = builderErr

					return false, nil // Continue polling
				}

				// Non-transient error, stop polling
				return false, builderErr
			}

			if deviceConfigBuilder == nil {
				lastErr = fmt.Errorf("deviceConfigBuilder is nil")

				return false, nil // Continue polling
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("AMD GPU CRD is available")

			return true, nil
		})
	if err != nil {
		if lastErr != nil {
			return fmt.Errorf("failed to create DeviceConfig builder after retries: %w (last error: %w)", err, lastErr)
		}

		return fmt.Errorf("failed to create DeviceConfig builder: %w", err)
	}

	if deviceConfigBuilder.Exists() {
		klog.V(amdgpuparams.AMDGPULogLevel).Info("DeviceConfig already exists")

		return nil
	}

	_, err = deviceConfigBuilder.Create()
	if err != nil {
		return handleDeviceConfigCreationError(err, deviceConfigName)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Successfully created DeviceConfig")
	klog.V(amdgpuparams.AMDGPULogLevel).Info("This will trigger AMD GPU driver installation via KMM")

	return nil
}

// isTransientAPIError checks if the error is a transient API error that should be retried.
func isTransientAPIError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Connection errors
	if strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "i/o timeout") {
		return true
	}

	// CRD not ready errors
	if strings.Contains(errStr, "unable to retrieve the complete list of server APIs") ||
		strings.Contains(errStr, "the server could not find the requested resource") ||
		strings.Contains(errStr, "no matches for kind") {
		return true
	}

	return false
}

// createDeviceConfigBuilder creates a DeviceConfig builder with proper definition.
func createDeviceConfigBuilder(
	apiClient *clients.Settings,
	deviceConfigName,
	driverVersion string) (*amdgpu.Builder, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(amdgpuv1.AddToScheme)
	if err != nil {
		return nil, fmt.Errorf("failed to attach amdgpu scheme: %w", err)
	}

	builder, err := amdgpu.Pull(apiClient, deviceConfigName, amdgpuparams.AMDGPUNamespace)
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig %s does not exist, will create new one", deviceConfigName)
	} else if builder != nil {
		return builder, nil
	}

	// Create DeviceConfig with:
	// - Driver enabled with specified version
	// - Device Plugin with Node Labeller explicitly enabled
	// - Selector for AMD GPU nodes detected by NFD
	almExampleJSON := fmt.Sprintf(`[{
		"apiVersion": "amd.com/v1alpha1",
		"kind": "DeviceConfig",
		"metadata": {
			"name": "%s",
			"namespace": "%s"
		},
		"spec": {
			"driver": {
				"enable": true,
				"version": "%s"
			},
			"devicePlugin": {
				"enableNodeLabeller": true
			},
			"selector": {
				"feature.node.kubernetes.io/amd-gpu": "true"
			}
		}
	}]`, deviceConfigName, amdgpuparams.AMDGPUNamespace, driverVersion)

	builder = amdgpu.NewBuilderFromObjectString(apiClient, almExampleJSON)
	if builder == nil {
		return nil, fmt.Errorf("failed to create DeviceConfig builder from JSON")
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Created DeviceConfig builder with definition")

	return builder, nil
}

// handleDeviceConfigCreationError handles errors during DeviceConfig creation.
func handleDeviceConfigCreationError(err error, deviceConfigName string) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Error creating DeviceConfig %s: %v", deviceConfigName, err)

	if amdgpucommon.IsCRDNotAvailable(err) {
		klog.V(amdgpuparams.AMDGPULogLevel).Info("DeviceConfig CRD not available - manual creation required")
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig creation failed - CRD may not be installed")

		return fmt.Errorf("DeviceConfig CRD not available, manual creation required")
	}

	return err
}

// DeleteDeviceConfig deletes the DeviceConfig custom resource.
func DeleteDeviceConfig(apiClient *clients.Settings, deviceConfigName string) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Deleting DeviceConfig: %s", deviceConfigName)

	err := apiClient.AttachScheme(amdgpuv1.AddToScheme)
	if err != nil {
		return fmt.Errorf("failed to attach amdgpu scheme: %w", err)
	}

	deviceConfigBuilder, err := amdgpu.Pull(apiClient, deviceConfigName, amdgpuparams.AMDGPUNamespace)
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig %s not found, nothing to delete", deviceConfigName)

		return nil
	}

	if deviceConfigBuilder == nil || !deviceConfigBuilder.Exists() {
		klog.V(amdgpuparams.AMDGPULogLevel).Info("DeviceConfig does not exist")

		return nil
	}

	_, err = deviceConfigBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete DeviceConfig %s: %w", deviceConfigName, err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("DeviceConfig deleted successfully")

	return nil
}
