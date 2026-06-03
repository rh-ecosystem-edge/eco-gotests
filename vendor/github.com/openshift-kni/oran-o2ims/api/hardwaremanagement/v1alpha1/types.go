/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

// Node role constants
const (
	NodeRoleMaster = "master"
	NodeRoleWorker = "worker"
)

// NodeGroupData provides the necessary information for populating a node allocation request
type NodeGroupData struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=master;worker
	Role string `json:"role"`
	// HwProfile is the name of the HardwareProfile to use for this node group.
	// +optional
	HwProfile string `json:"hwProfile,omitempty"`
	// ResourcePoolId is the identifier for the Resource Pool in the hardware manager instance.
	// +optional
	ResourcePoolId string `json:"resourcePoolId,omitempty"`
	// +optional
	ResourceSelector map[string]string `json:"resourceSelector,omitempty"`
}
