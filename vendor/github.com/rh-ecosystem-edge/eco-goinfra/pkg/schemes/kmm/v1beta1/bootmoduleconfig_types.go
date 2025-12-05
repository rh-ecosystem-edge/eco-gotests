/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootModuleConfigSpec describes the desired state of BootModuleConfig.
type BootModuleConfigSpec struct {
	// +kubebuilder:validation:Required
	// MachineConfigName is the name of the target machine config.
	MachineConfigName string `json:"machineConfigName"`

	// +kubebuilder:validation:Required
	// MachineConfigPoolName is the name of the machine config pool.
	MachineConfigPoolName string `json:"machineConfigPoolName"`

	// +kubebuilder:validation:Required
	// KernelModuleImage is the container image with the kernel module.
	KernelModuleImage string `json:"kernelModuleImage"`

	// +kubebuilder:validation:Required
	// KernelModuleName is the name of the kernel module to load.
	KernelModuleName string `json:"kernelModuleName"`

	// +optional
	// InTreeModulesToRemove is a list of in-tree kernel modules to remove.
	InTreeModulesToRemove []string `json:"inTreeModulesToRemove,omitempty"`

	// +optional
	// FirmwareFilesPath is the path of firmware files in the container.
	FirmwareFilesPath string `json:"firmwareFilesPath,omitempty"`

	// +optional
	// WorkerImage is the KMM worker image.
	WorkerImage string `json:"workerImage,omitempty"`
}

// BootModuleConfigStatus defines the observed state of BootModuleConfig.
type BootModuleConfigStatus struct {
	// +optional
	// ConfigStatus represents the configuration status.
	ConfigStatus string `json:"configStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:subresource:status

// BootModuleConfig describes how to configure a kernel module to be loaded at boot time.
// +operator-sdk:csv:customresourcedefinitions:displayName="Boot Module Config"
type BootModuleConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BootModuleConfigSpec   `json:"spec,omitempty"`
	Status BootModuleConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BootModuleConfigList contains a list of BootModuleConfig.
type BootModuleConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootModuleConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BootModuleConfig{}, &BootModuleConfigList{})
}
