/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=seedgenerators,scope=Cluster,shortName=seedgen
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[-1:].type"
//+kubebuilder:printcolumn:name="Details",type="string",JSONPath=".status.conditions[-1:].message"
// +kubebuilder:validation:XValidation:message="seedgen is a singleton, metadata.name must be 'seedimage'", rule="self.metadata.name == 'seedimage'"

// SeedGenerator is the Schema for the seedgenerators API
// +operator-sdk:csv:customresourcedefinitions:displayName="Seed Generator",resources={{Namespace, v1}}
type SeedGenerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	//+kubebuilder:validation:XValidation:rule="self == oldSelf",message="cannot modify spec, cr must be deleted and recreated"
	Spec   SeedGeneratorSpec   `json:"spec,omitempty"`
	Status SeedGeneratorStatus `json:"status,omitempty"`
}

// SeedGeneratorSpec defines the desired state of SeedGenerator
type SeedGeneratorSpec struct {
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Seed Image",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern="^([a-z0-9]+://)?[\\S]+$"
	// SeedImage defines the full pull-spec of the seed container image to be created.
	SeedImage string `json:"seedImage,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern="^([a-z0-9]+://)?[\\S]+$"
	// RecertImage defines the full pull-spec of the recert container image to use.
	RecertImage string `json:"recertImage,omitempty"`
}

// SeedGeneratorStatus defines the observed state of SeedGenerator
type SeedGeneratorStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Status"
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	StartedAt          metav1.Time `json:"startedAt,omitempty"`
	CompletedAt        metav1.Time `json:"completedAt,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Conditions",xDescriptors={"urn:alm:descriptor:io.kubernetes.conditions"}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true

// SeedGeneratorList contains a list of SeedGenerator
type SeedGeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SeedGenerator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SeedGenerator{}, &SeedGeneratorList{})
}
