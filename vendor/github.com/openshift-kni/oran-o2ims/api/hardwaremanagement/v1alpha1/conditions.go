/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

type ConditionType string

// TODO: Refactor condition types, reasons, and messages to clearly indicate which
// hardware related CR (NodeAllocationRequest, AllocatedNode, etc.) each belongs to.

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
	// InProgress indicates that the hardware provisioning or configuration update is in progress.
	InProgress ConditionReason = "InProgress"
	// Completed indicates that the hardware provisioning has been completed successfully.
	Completed     ConditionReason = "Completed"
	Unprovisioned ConditionReason = "Unprovisioned" // TODO: Remove this condition reason as it is not used.
	// Failed indicates that the hardware provisioning or configuration update has failed.
	Failed         ConditionReason = "Failed"
	NotInitialized ConditionReason = "NotInitialized" // TODO: Remove this condition reason as it is not used.
	// TimedOut indicates that the hardware provisioning or configuration update has timed out.
	TimedOut ConditionReason = "TimedOut"
	// ConfigUpdatePending indicates that a new hardware profile is requested for the AllocatedNode and the node is waiting to be processed by the hardware manager.
	ConfigUpdatePending ConditionReason = "ConfigurationUpdatePending"
	// ConfigUpdate indicates that the hardware configuration changes have been requested by the hardware manager for the AllocatedNode.
	ConfigUpdate ConditionReason = "ConfigurationUpdateRequested"
	// ConfigApplied indicates that the hardware configuration update has been applied successfully.
	ConfigApplied ConditionReason = "ConfigurationApplied"
	// InvalidInput indicates that the requested hardware profile contains invalid input.
	InvalidInput ConditionReason = "InvalidUserInput"
)

// ConditionMessage provides detailed messages associated with condition status updates.
type ConditionMessage string

// NAR condition messages
const (
	AwaitConfig      ConditionMessage = "Spec updated; awaiting configuration application by the hardware manager"
	ConfigSuccess    ConditionMessage = "Configuration has been applied successfully"
	ConfigInProgress ConditionMessage = "Configuration update in progress"
	ConfigFailed     ConditionMessage = "Configuration update failed"
)

// AllocatedNode condition messages for day2 update stages
const (
	NodeUpdatePending       ConditionMessage = "Pending for updates evaluation"
	NodeDraining            ConditionMessage = "Draining node"
	NodeUpdateRequested     ConditionMessage = "Update requested"
	NodeWaitingBMHServicing ConditionMessage = "Waiting for BMH to enter Servicing"
	NodeWaitingBMHComplete  ConditionMessage = "Waiting for BMH completion"
	NodeWaitingReady        ConditionMessage = "BMH update complete, waiting for node to become Ready"
)
