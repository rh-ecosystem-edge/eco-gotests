// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

// +kubebuilder:validation:MinLength=1
type NonEmptyString string

// PolicySetSpec defines the group of policies to be included in the policy set.
type PolicySetSpec struct {
	// Description is the description of this policy set.
	Description string `json:"description,omitempty"`

	// Policies is a list of policy names that are contained within the policy set.
	Policies []NonEmptyString `json:"policies"`
}

// PolicySetStatusPlacement reports how and what managed cluster placement resources are attached to
// the policy set.
type PolicySetStatusPlacement struct {
	// PlacementBinding is the name of the PlacementBinding resource, from the
	// policies.open-cluster-management.io API group, that binds the placement resource to the policy
	// set.
	PlacementBinding string `json:"placementBinding,omitempty"`

	// Placement is the name of the Placement resource, from the cluster.open-cluster-management.io
	// API group, that is bound to the policy.
	Placement string `json:"placement,omitempty"`

	// PlacementRule (deprecated) is the name of the PlacementRule resource, from the
	// apps.open-cluster-management.io API group, that is bound to the policy.
	PlacementRule string `json:"placementRule,omitempty"`
}

// PolicySetStatus reports the observed status of the policy set resulting from its policies.
type PolicySetStatus struct {
	Placement []PolicySetStatusPlacement `json:"placement,omitempty"`

	// Compliant reports the observed status resulting from the compliance of the policies within.
	Compliant policyv1.ComplianceState `json:"compliant,omitempty"`

	// StatusMessge reports the current state while determining the compliance of the policy set.
	StatusMessage string `json:"statusMessage,omitempty"`
}

// PolicySet is the schema for the policysets API. A policy set is a logical grouping of policies
// from the same namespace. The policy set is bound to a placement resource and applies the
// placement to all policies within the set. The status reports the overall compliance of the set.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=policysets,scope=Namespaced
// +kubebuilder:resource:path=policysets,shortName=plcset
// +kubebuilder:printcolumn:name="Compliance state",type="string",JSONPath=".status.compliant"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type PolicySet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicySetSpec   `json:"spec"`
	Status PolicySetStatus `json:"status,omitempty"`
}

// PolicySetList contains a list of policy sets.
//
// +kubebuilder:object:root=true
type PolicySetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicySet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PolicySet{}, &PolicySetList{})
}
