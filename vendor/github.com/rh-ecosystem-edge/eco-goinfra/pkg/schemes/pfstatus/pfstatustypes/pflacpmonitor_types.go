/*
Copyright 2024.

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

package pfstatustypes

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PFLACPMonitorSpec defines the desired state of PFLACPMonitor
type PFLACPMonitorSpec struct {
	// +kubebuilder:validation:MinItems=1

	// List of interfaces to monitor
	Interfaces []string `json:"interfaces"`

	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:default:=1000

	// Polling interval in milliseconds
	// +optional
	PollingInterval int `json:"pollingInterval,omitempty"`

	// Selector to filter nodes
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// PFLACPMonitorStatus defines the observed state of PFLACPMonitor
type PFLACPMonitorStatus struct {
	// Degraded indicates whether the monitor is in a degraded state
	// +optional
	Degraded bool `json:"degraded,omitempty"`

	// Error message
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PFLACPMonitor is the Schema for the pflacpmonitors API
type PFLACPMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PFLACPMonitorSpec   `json:"spec,omitempty"`
	Status PFLACPMonitorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PFLACPMonitorList contains a list of PFLACPMonitor
type PFLACPMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PFLACPMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PFLACPMonitor{}, &PFLACPMonitorList{})
}
