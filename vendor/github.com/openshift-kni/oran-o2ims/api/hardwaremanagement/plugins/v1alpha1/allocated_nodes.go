/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Interface describes an interface of a hardware server
type Interface struct {
	Name  string `json:"name"`  // The name of the network interface (e.g., eth0, ens33)
	Label string `json:"label"` // The label of the interface
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}[:]){5}([0-9A-Fa-f]{2})$`
	MACAddress string `json:"macAddress"` // The MAC address of the interface
}

// AllocatedNodeSpec describes a node presents a hardware server
type AllocatedNodeSpec struct {
	// NodeAllocationRequest
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Allocation Request",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	NodeAllocationRequest string `json:"nodeAllocationRequest"`

	// GroupName
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Group Name",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	GroupName string `json:"groupName"`

	// HwProfile
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Profile",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HwProfile string `json:"hwProfile"`

	// HardwarePluginRef is the identifier for the HardwarePlugin instance.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Plugin Reference",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HardwarePluginRef string `json:"hardwarePluginRef,omitempty"`

	// HwMgrNodeId is the node identifier from the hardware manager.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Manager Node ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HwMgrNodeId string `json:"hwMgrNodeId,omitempty"`

	// HwMgrNodeNs is the node namespace from the hardware manager.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Manager Node Namespace",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HwMgrNodeNs string `json:"hwMgrNodeNs,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Extensions map[string]string `json:"extensions,omitempty"`
}

// BMC describes BMC details of a hardware server
type BMC struct {
	// The Address contains the URL for accessing the BMC over the network.
	Address string `json:"address,omitempty"`

	// CredentialsName is a reference to a secret containing the credentials. That secret
	// should contain the keys `username` and `password`.
	CredentialsName string `json:"credentialsName,omitempty"`
}

// AllocatedNodeStatus describes the observed state of a request to allocate and prepare
// a node that will eventually be part of a deployment manager.
type AllocatedNodeStatus struct {
	//+operator-sdk:csv:customresourcedefinitions:type=status
	BMC *BMC `json:"bmc,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=status
	Interfaces []*Interface `json:"interfaces,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=status
	Hostname string `json:"hostname,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=status
	HwProfile string `json:"hwProfile,omitempty"`

	// Conditions represent the observations of the AllocatedNodeStatus's current state.
	// Possible values of the condition type are `Provisioned`, `Unprovisioned`, `Updating` and `Failed`.
	//+operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AllocatedNode is the schema for an allocated node
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=allocatednodes,shortName=allocatednode
// +kubebuilder:printcolumn:name="Plugin",type="string",JSONPath=".spec.hardwarePluginRef"
// +kubebuilder:printcolumn:name="NodeAllocationRequest",type="string",JSONPath=".spec.nodeAllocationRequest"
// +kubebuilder:printcolumn:name="HwMgr Node ID",type="string",JSONPath=".spec.hwMgrNodeId"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[-1:].reason"
// +operator-sdk:csv:customresourcedefinitions:displayName="Allocated Node",resources={{Namespace, v1}}
type AllocatedNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AllocatedNodeSpec   `json:"spec,omitempty"`
	Status AllocatedNodeStatus `json:"status,omitempty"`
}

// AllocatedNodeList contains a list of provisioned node.
//
// +kubebuilder:object:root=true
type AllocatedNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AllocatedNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&AllocatedNode{},
		&AllocatedNodeList{},
	)
}
