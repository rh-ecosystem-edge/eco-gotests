/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-kni/oran-o2ims/api/common"
)

// ConditionTypes define the different types of conditions that will be set
var ConditionTypes = struct {
	Registration ConditionType
}{
	Registration: "Registration",
}

// ConditionReasons define the different reasons that conditions will be set for
var ConditionReasons = struct {
	Completed  ConditionReason
	Failed     ConditionReason
	InProgress ConditionReason
}{
	Completed:  "Completed",
	Failed:     "Failed",
	InProgress: "InProgress",
}

// HardwarePluginSpec defines the desired state of HardwarePlugin
type HardwarePluginSpec struct {
	// ApiRoot is the root URL for the Hardware Plugin.
	// +kubebuilder:validation:MinLength=1
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Plugin API root",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ApiRoot string `json:"apiRoot"`

	// CaBundleName references a config map that contains a set of custom CA certificates to be used when communicating
	// with any outside HardwarePlugin server that has its TLS certificate signed by a non-public CA certificate.
	// The config map is expected to contain a single file called 'ca-bundle.crt' containing all trusted CA certificates
	// in PEM format.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Custom CA Certificates",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CaBundleName *string `json:"caBundleName,omitempty"`

	// AuthClientConfig defines the configurable client attributes required to access the OAuth2 authorization server
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="SMO OAuth Configuration"
	AuthClientConfig *common.AuthClientConfig `json:"authClientConfig,omitempty"`
}

// HardwarePluginStatus defines the observed state of HardwarePlugin
type HardwarePluginStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the state of the UpdateService resource.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// HardwarePlugin is the Schema for the hardwareplugins API
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=hardwareplugins,shortName=hwplugin
// +operator-sdk:csv:customresourcedefinitions:displayName="Hardware Plugin",resources={{Namespace, v1}}
type HardwarePlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HardwarePluginSpec   `json:"spec,omitempty"`
	Status HardwarePluginStatus `json:"status,omitempty"`
}

// HardwarePluginList contains a list of HardwarePlugin
//
// +kubebuilder:object:root=true
type HardwarePluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwarePlugin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HardwarePlugin{}, &HardwarePluginList{})
}
