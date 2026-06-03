/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocationSpec is the geographical location of the requested node.
type LocationSpec struct {
	// Location
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Location",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Location string `json:"location,omitempty"`
	// Site
	// +kubebuilder:validation:Required
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Site",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Site string `json:"site"`
}

// NodeAllocationRequestSpec describes a group of nodes to allocate
type NodeAllocationRequestSpec struct {
	// ClusterID is the identifier of the O-Cloud that generated this request. The hardware
	// manager may want to use this to tag the nodes in its database, and to generate
	// statistics.
	//
	// +kubebuilder:validation:Required
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Cluster ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ClusterId string `json:"clusterId"`

	// LocationSpec is the geographical location of the requested node.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Location Spec",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	LocationSpec `json:",inline"`

	//+operator-sdk:csv:customresourcedefinitions:type=spec
	NodeGroup []NodeGroup `json:"nodeGroup"`

	//+operator-sdk:csv:customresourcedefinitions:type=spec
	Extensions map[string]string `json:"extensions,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=spec
	ConfigTransactionId int64 `json:"configTransactionId"`

	// HardwareProvisioningTimeout defines the timeout duration string for the hardware provisioning.
	// If not specified, the default timeout value will be applied.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Hardware Provisioning Timeout",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	HardwareProvisioningTimeout string `json:"hardwareProvisioningTimeout,omitempty"`

	// ClusterProvisioned indicates that the cluster using the allocated nodes has been
	// fully provisioned and is operational. The hardware manager uses this signal to
	// perform any post-provisioning steps, such as enabling BMO management of
	// IBI-provisioned nodes.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Cluster Provisioned",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	ClusterProvisioned bool `json:"clusterProvisioned,omitempty"`

	// SkipCleanup indicates that the hardware manager should skip cleanup operations
	// (disk wipe, power off) when the NodeAllocationRequest is deleted. This is used
	// when the spoke cluster needs to remain running, such as during seed image
	// generation for IBI/IBU. The hardware manager propagates this to the BMH as the
	// clcm.openshift.io/skip-cleanup annotation.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Skip Cleanup",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	SkipCleanup bool `json:"skipCleanup,omitempty"`
}

type NodeGroup struct {
	NodeGroupData NodeGroupData `json:"nodeGroupData"` // Explicitly include as a named field
	Size          int           `json:"size" yaml:"size"`
}

type Properties struct {
	NodeNames []string `json:"nodeNames,omitempty"`
}

// NodeAllocationRequestStatus describes the observed state of a request to allocate and prepare
// a node that will eventually be part of a deployment manager.
type NodeAllocationRequestStatus struct {
	// Properties represent the node properties in the pool
	//+operator-sdk:csv:customresourcedefinitions:type=status
	Properties Properties `json:"properties,omitempty"`

	// Conditions represent the latest available observations of an NodeAllocationRequest's state.
	// +optional
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:Items=Type=object
	//+operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to detect spec changes that require re-processing.
	//+operator-sdk:csv:customresourcedefinitions:type=status
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+operator-sdk:csv:customresourcedefinitions:type=status
	ObservedConfigTransactionId int64 `json:"observedConfigTransactionId"`

	//+operator-sdk:csv:customresourcedefinitions:type=status
	SelectedGroups map[string]string `json:"selectedGroups,omitempty"`

	// HardwareOperationStartTime tracks when the current hardware operation (provisioning or configuration) actually started.
	// This timestamp is used for timeout calculations. The active operation is determined from the conditions.
	//+operator-sdk:csv:customresourcedefinitions:type=status
	HardwareOperationStartTime *metav1.Time `json:"hardwareOperationStartTime,omitempty"`
}

// NodeAllocationRequest is the schema for an allocation request of nodes
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nodeallocationrequests,shortName=nar
// +kubebuilder:printcolumn:name="Cluster ID",type="string",JSONPath=".spec.clusterId"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[-1:].reason"
// +operator-sdk:csv:customresourcedefinitions:displayName="Node Allocation Request",resources={{Namespace, v1}}
type NodeAllocationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeAllocationRequestSpec   `json:"spec,omitempty"`
	Status NodeAllocationRequestStatus `json:"status,omitempty"`
}

// NodeAllocationRequestList contains a list of node allocation requests.
//
// +kubebuilder:object:root=true
type NodeAllocationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeAllocationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&NodeAllocationRequest{},
		&NodeAllocationRequestList{},
	)
}
