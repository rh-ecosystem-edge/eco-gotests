/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

type ConditionType string

// The following constants define the different types of conditions that will be set
const (
	Provisioned ConditionType = "Provisioned"
	Configured  ConditionType = "Configured"
	Validation  ConditionType = "Validation"
	Unknown     ConditionType = "Unknown" // Indicates the condition has not been evaluated
)

// ConditionReason describes the reasons for a condition's status.
type ConditionReason string

const (
	InProgress     ConditionReason = "InProgress"
	Completed      ConditionReason = "Completed"
	Unprovisioned  ConditionReason = "Unprovisioned"
	Failed         ConditionReason = "Failed"
	NotInitialized ConditionReason = "NotInitialized"
	TimedOut       ConditionReason = "TimedOut"
	ConfigUpdate   ConditionReason = "ConfigurationUpdateRequested"
	ConfigApplied  ConditionReason = "ConfigurationApplied"
	InvalidInput   ConditionReason = "InvalidUserInput"
)

// ConditionMessage provides detailed messages associated with condition status updates.
type ConditionMessage string

const (
	AwaitConfig   ConditionMessage = "Spec updated; awaiting configuration application by the hardware plugin"
	ConfigSuccess ConditionMessage = "Configuration has been applied successfully"
)
