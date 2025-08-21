package amdgpudeviceconfig

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/amdgpu"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	amdgpuv1 "github.com/openshift-kni/eco-goinfra/pkg/schemes/amd/gpu-operator/api/v1alpha1"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
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
func CreateDeviceConfig(apiClient *clients.Settings, deviceConfigName string) error {
	glog.V(amdgpuparams.LogLevel).Infof("Creating DeviceConfig: %s", deviceConfigName)

	deviceConfigBuilder, err := createDeviceConfigBuilder(apiClient, deviceConfigName)
	if err != nil {
		return fmt.Errorf("failed to create DeviceConfig builder: %w", err)
	}

	if deviceConfigBuilder.Exists() {
		glog.V(amdgpuparams.LogLevel).Info("DeviceConfig already exists")

		return nil
	}

	_, err = deviceConfigBuilder.Create()
	if err != nil {
		return handleDeviceConfigCreationError(err, deviceConfigName)
	}

	glog.V(amdgpuparams.LogLevel).Info("Successfully created DeviceConfig")
	glog.V(amdgpuparams.LogLevel).Info("This will trigger AMD GPU driver installation via KMM")

	return nil
}

// createDeviceConfigBuilder creates a DeviceConfig builder with proper definition.
func createDeviceConfigBuilder(apiClient *clients.Settings, deviceConfigName string) (*amdgpu.Builder, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(amdgpuv1.AddToScheme)
	if err != nil {
		return nil, fmt.Errorf("failed to attach amdgpu scheme: %w", err)
	}

	builder, err := amdgpu.Pull(apiClient, deviceConfigName, amdgpuparams.AMDGPUOperatorNamespace)
	if err != nil {
		glog.V(amdgpuparams.LogLevel).Infof("DeviceConfig %s does not exist, will create new one", deviceConfigName)
	} else if builder != nil {
		return builder, nil
	}

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
			"selector": {
				"feature.node.kubernetes.io/amd-gpu": "true"
			}
		}
	}]`, deviceConfigName, amdgpuparams.AMDGPUOperatorNamespace, amdgpuparams.DefaultDriverVersion)

	builder = amdgpu.NewBuilderFromObjectString(apiClient, almExampleJSON)
	if builder == nil {
		return nil, fmt.Errorf("failed to create DeviceConfig builder from JSON")
	}

	glog.V(amdgpuparams.LogLevel).Infof("Created DeviceConfig builder with definition")

	return builder, nil
}

// handleDeviceConfigCreationError handles errors during DeviceConfig creation.
func handleDeviceConfigCreationError(err error, deviceConfigName string) error {
	glog.V(amdgpuparams.LogLevel).Infof("Error creating DeviceConfig %s: %v", deviceConfigName, err)

	if amdgpucommon.IsCRDNotAvailable(err) {
		glog.V(amdgpuparams.LogLevel).Info("DeviceConfig CRD not available - manual creation required")
		glog.V(amdgpuparams.LogLevel).Infof("DeviceConfig creation failed - CRD may not be installed")

		return fmt.Errorf("DeviceConfig CRD not available, manual creation required")
	}

	return err
}
