package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelFormat describes the model format and optional version.
type ModelFormat struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// ModelSpec describes the model to serve.
type ModelSpec struct {
	ModelFormat ModelFormat                 `json:"modelFormat,omitempty"`
	Runtime     *string                     `json:"runtime,omitempty"`
	StorageURI  *string                     `json:"storageUri,omitempty"`
	Resources   corev1.ResourceRequirements `json:"resources,omitempty"`
	Env         []corev1.EnvVar             `json:"env,omitempty"`
}

// PredictorSpec defines the predictor configuration.
type PredictorSpec struct {
	ServiceAccountName string    `json:"serviceAccountName,omitempty"`
	Model              ModelSpec `json:"model,omitempty"`
}

// InferenceServiceSpec defines the desired state of InferenceService.
type InferenceServiceSpec struct {
	Predictor PredictorSpec `json:"predictor"`
}

// ConditionStatus represents the status of a condition.
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// Condition defines an observation of an InferenceService resource state.
type Condition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	LastTransitionTime *metav1.Time    `json:"lastTransitionTime,omitempty"`
	Reason             string          `json:"reason,omitempty"`
	Message            string          `json:"message,omitempty"`
}

// InferenceServiceStatus defines the observed state of InferenceService.
type InferenceServiceStatus struct {
	Conditions []Condition `json:"conditions,omitempty"`
	URL        string      `json:"url,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// InferenceService is the Schema for the inferenceservices API.
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceServiceSpec   `json:"spec,omitempty"`
	Status InferenceServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InferenceServiceList contains a list of InferenceService.
type InferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceService{}, &InferenceServiceList{})
}
