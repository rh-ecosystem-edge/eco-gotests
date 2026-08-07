package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	Host string `json:"host"`
	Port int32  `json:"port"`
}

// ObjectReference is a reference to another object with apiVersion, kind, and name.
type ObjectReference struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// ClusterSpec defines the desired state of Cluster.
type ClusterSpec struct {
	Paused                bool             `json:"paused,omitempty"`
	ControlPlaneEndpoint  APIEndpoint      `json:"controlPlaneEndpoint,omitempty"`
	ControlPlaneRef       *ObjectReference `json:"controlPlaneRef,omitempty"`
	InfrastructureRef     *ObjectReference `json:"infrastructureRef,omitempty"`
}

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Cluster is the Schema for the clusters API.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}
