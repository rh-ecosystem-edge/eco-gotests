package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupportedModelFormat describes a model format supported by the runtime.
type SupportedModelFormat struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	AutoSelect bool   `json:"autoSelect,omitempty"`
}

// ServingRuntimeSpec defines the desired state of ServingRuntime.
type ServingRuntimeSpec struct {
	SupportedModelFormats []SupportedModelFormat `json:"supportedModelFormats,omitempty"`
	MultiModel            *bool                  `json:"multiModel,omitempty"`
	Containers            []corev1.Container     `json:"containers,omitempty"`
	Volumes               []corev1.Volume        `json:"volumes,omitempty"`
}

//+kubebuilder:object:root=true

// ServingRuntime is the Schema for the servingruntimes API.
type ServingRuntime struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ServingRuntimeSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// ServingRuntimeList contains a list of ServingRuntime.
type ServingRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServingRuntime `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServingRuntime{}, &ServingRuntimeList{})
}
