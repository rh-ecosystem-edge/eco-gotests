/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGroupData provides the necessary information for populating a node allocation request
type NodeGroupData struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=master;worker
	Role string `json:"role"`
	// +kubebuilder:validation:MinLength=1
	HwProfile string `json:"hwProfile"`
	// ResourcePoolId is the identifier for the Resource Pool in the hardware manager instance.
	// +optional
	ResourcePoolId string `json:"resourcePoolId,omitempty"`
	// +optional
	ResourceSelector map[string]string `json:"resourceSelector,omitempty"`
}

// HardwareTemplateSpec defines the desired state of HardwareTemplate
type HardwareTemplateSpec struct {

	// HardwarePluginRef is the name of the HardwarePlugin.
	// +kubebuilder:validation:MinLength=1
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Plugin Reference",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HardwarePluginRef string `json:"hardwarePluginRef"`

	// BootInterfaceLabel is the label of the boot interface.
	// +kubebuilder:validation:MinLength=1
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Boot Interface Label",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	BootInterfaceLabel string `json:"bootInterfaceLabel"`

	// HardwareProvisioningTimeout defines the timeout duration string for the hardware provisioning.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Provisioning Timeout",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HardwareProvisioningTimeout string `json:"hardwareProvisioningTimeout,omitempty"`

	// NodeGroupData defines a collection of NodeGroupData items
	// +kubebuilder:validation:MinItems=1
	//+operator-sdk:csv:customresourcedefinitions:type=spec
	NodeGroupData []NodeGroupData `json:"nodeGroupData"`
}

// HardwareTemplateStatus defines the observed state of HardwareTemplate
type HardwareTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of HardwareTemplate
	// Important: Run "make" to regenerate code after modifying this file

	//+operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=hardwaretemplates,shortName=oranhwtmpl
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[-1:].reason"
//+kubebuilder:printcolumn:name="Details",type="string",JSONPath=".status.conditions[-1:].message"

// HardwareTemplate is the Schema for the hardwaretemplates API
// +kubebuilder:validation:XValidation:message="Spec changes are not allowed for a HardwareTemplate that has passed the validation", rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Validation' && c.status=='False') || oldSelf.spec == self.spec"
// +operator-sdk:csv:customresourcedefinitions:displayName="Hardware Template",resources={{ConfigMap, v1}}
type HardwareTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HardwareTemplateSpec   `json:"spec,omitempty"`
	Status HardwareTemplateStatus `json:"status,omitempty"`
}

// HardwareTemplateList contains a list of HardwareTemplate.
//
// +kubebuilder:object:root=true
type HardwareTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&HardwareTemplate{},
		&HardwareTemplateList{},
	)
}
