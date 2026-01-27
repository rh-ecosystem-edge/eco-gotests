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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PCIVendorID = "1d0f"
)

// DeviceConfigSpec describes how the AMD GPU operator should enable AMD GPU device for customer's use.
type DeviceConfigSpec struct {
	// if the in-tree driver should be used instead of OOT drivers
	UseInTreeDrivers bool `json:"useInTreeDrivers,omitempty"`

	// defines image that includes drivers
	// +kubebuilder:validation:Required
	DriversImage string `json:"driversImage,omitempty"`

	// defines the Version of the neuron drivers. used for rolling upgrade
	// +optional
	DriverVersion string `json:"driverVersion,omitempty"`

	// device plugin image
	// +kubebuilder:validation:Required
	DevicePluginImage string `json:"devicePluginImage,omitempty"`

	// custom scheduler image
	// +kubebuilder:validation:Required
	CustomSchedulerImage string `json:"customSchedulerImage,omitempty"`

	// scheduler extension image
	// +kubebuilder:validation:Required
	SchedulerExtensionImage string `json:"schedulerExtensionImage,omitempty"`

	// node metrics image
	// +kubebuilder:validation:Required
	NodeMetricsImage string `json:"nodeMetricsImage,omitempty"`

	// pull secrets used for pull/setting images used by operator
	// +optional
	ImageRepoSecret *v1.LocalObjectReference `json:"imageRepoSecret,omitempty"`

	// Selector describes on which nodes the GPU Operator should enable the GPU device.
	// +optional
	Selector map[string]string `json:"selector,omitempty"`
}

// DaemonSetStatus contains the status for a daemonset deployed during
// reconciliation loop
type DeploymentStatus struct {
	// number of nodes that are targeted by the DeviceConfig selector
	NodesMatchingSelectorNumber int32 `json:"nodesMatchingSelectorNumber,omitempty"`
	// number of the pods that should be deployed for daemonset
	DesiredNumber int32 `json:"desiredNumber,omitempty"`
	// number of the actually deployed and running pods
	AvailableNumber int32 `json:"availableNumber,omitempty"`
}

// ModuleStatus defines the observed state of Module.
type DeviceConfigStatus struct {
	// DevicePlugin contains the status of the Device Plugin deployment
	DevicePlugin DeploymentStatus `json:"devicePlugin,omitempty"`
	// Driver contains the status of the Drivers deployment
	Drivers DeploymentStatus `json:"driver"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Namespaced,shortName=awslabsdc
//+kubebuilder:subresource:status

// DeviceConfig describes how to enable awslabs GPU device
// +operator-sdk:csv:customresourcedefinitions:displayName="DeviceConfig"
type DeviceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceConfigSpec   `json:"spec,omitempty"`
	Status DeviceConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeviceConfigList contains a list of DeviceConfigs
type DeviceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceConfig{}, &DeviceConfigList{})
}
