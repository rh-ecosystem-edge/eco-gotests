/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Condition types for inventory CRs
const (
	// ConditionTypeReady indicates the CR is valid and synced
	ConditionTypeReady = "Ready"
)

// Condition reasons for inventory CRs
const (
	// ReasonReady indicates the CR is ready
	ReasonReady = "Ready"

	// ReasonParentNotFound indicates the referenced parent CR does not exist
	ReasonParentNotFound = "ParentNotFound"

	// ReasonParentNotReady indicates the parent CR exists but is not ready
	ReasonParentNotReady = "ParentNotReady"
)

// IsResourceReady checks if a CR has the Ready=True condition.
func IsResourceReady(conditions []metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == ConditionTypeReady {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}

// GetReadyReason returns the reason from the Ready condition, or empty string if not found.
func GetReadyReason(conditions []metav1.Condition) string {
	for _, c := range conditions {
		if c.Type == ConditionTypeReady {
			return c.Reason
		}
	}
	return ""
}
