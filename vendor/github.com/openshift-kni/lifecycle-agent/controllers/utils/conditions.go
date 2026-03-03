package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	ipcv1 "github.com/openshift-kni/lifecycle-agent/api/ipconfig/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionType is a string representing the condition's type
type ConditionType string

// ConditionTypes define the different types of conditions that will be set
var ConditionTypes = struct {
	Idle               ConditionType
	PrepInProgress     ConditionType
	PrepCompleted      ConditionType
	UpgradeInProgress  ConditionType
	UpgradeCompleted   ConditionType
	RollbackInProgress ConditionType
	RollbackCompleted  ConditionType
	SeedGenInProgress  ConditionType
	SeedGenCompleted   ConditionType
	ConfigInProgress   ConditionType
	ConfigCompleted    ConditionType
}{
	Idle:               "Idle",
	PrepInProgress:     "PrepInProgress",
	PrepCompleted:      "PrepCompleted",
	UpgradeInProgress:  "UpgradeInProgress",
	UpgradeCompleted:   "UpgradeCompleted",
	RollbackInProgress: "RollbackInProgress",
	RollbackCompleted:  "RollbackCompleted",
	SeedGenInProgress:  "SeedGenInProgress",
	SeedGenCompleted:   "SeedGenCompleted",
	ConfigInProgress:   "ConfigInProgress",
	ConfigCompleted:    "ConfigCompleted",
}

var SeedGenConditionTypes = struct {
	SeedGenInProgress ConditionType
	SeedGenCompleted  ConditionType
}{
	SeedGenInProgress: "SeedGenInProgress",
	SeedGenCompleted:  "SeedGenCompleted",
}

// FinalConditionTypes defines the valid conditions for transitioning back to idle
var FinalConditionTypes = []ConditionType{ConditionTypes.UpgradeCompleted, ConditionTypes.RollbackCompleted}

// ConditionReason is a string representing the condition's reason
type ConditionReason string

// ConditionReasons define the different reasons that conditions will be set for
var ConditionReasons = struct {
	Idle                    ConditionReason
	ConfigurationInProgress ConditionReason
	Completed               ConditionReason
	Failed                  ConditionReason
	TimedOut                ConditionReason
	InProgress              ConditionReason
	Aborting                ConditionReason
	AbortCompleted          ConditionReason
	AbortFailed             ConditionReason
	Finalizing              ConditionReason
	FinalizeCompleted       ConditionReason
	FinalizeFailed          ConditionReason
	InvalidTransition       ConditionReason
	Blocked                 ConditionReason
}{
	Idle:                    "Idle",
	ConfigurationInProgress: "ConfigurationInProgress",
	Completed:               "Completed",
	Failed:                  "Failed",
	TimedOut:                "TimedOut",
	InProgress:              "InProgress",
	Aborting:                "Aborting",
	AbortCompleted:          "AbortCompleted",
	AbortFailed:             "AbortFailed",
	Finalizing:              "Finalizing",
	FinalizeCompleted:       "FinalizeCompleted",
	FinalizeFailed:          "FinalizeFailed",
	InvalidTransition:       "InvalidTransition",
	// Blocked condition reason is used to specify IPC or IBU is blocked by each other.
	// They are not allowed to run their flows simultaneously due to conflicts.
	Blocked: "Blocked",
}

// Common condition messages
// Note: This is not a complete list and does not include the custom messages
const (
	InProgress               = "In progress"
	InProgressOfBecomingIdle = "In progress of becoming idle"
	ConfigurationInProgress  = "Configuration in progress"
	ConfigurationCompleted   = "Configuration completed"
	Finalizing               = "Finalizing"
	Aborting                 = "Aborting"
	PrepCompleted            = "Prep completed"
	PrepFailed               = "Prep failed"
	UpgradeCompleted         = "Upgrade completed"
	UpgradeFailed            = "Upgrade failed"
	RollbackCompleted        = "Rollback completed"
	RollbackFailed           = "Rollback failed"
	RollbackRequested        = "Rollback requested"
	IBUNotIdle               = "IBU is not idle"
	IPCNotIdle               = "IPC is not idle"
)

var SeedGenConditionReasons = struct {
	Completed  ConditionReason
	Failed     ConditionReason
	InProgress ConditionReason
}{
	Completed:  "Completed",
	Failed:     "Failed",
	InProgress: "InProgress",
}

// SetStatusCondition is a convenience wrapper for meta.SetStatusCondition that takes in the types defined here and converts them to strings
func SetStatusCondition(existingConditions *[]metav1.Condition, conditionType ConditionType, conditionReason ConditionReason, conditionStatus metav1.ConditionStatus, message string, generation int64) {
	conditions := *existingConditions
	condition := meta.FindStatusCondition(*existingConditions, string(conditionType))
	if condition != nil &&
		(condition.Status != conditionStatus || condition.Type == string(ConditionTypes.Idle)) &&
		conditions[len(conditions)-1].Type != string(conditionType) {
		meta.RemoveStatusCondition(existingConditions, string(conditionType))
	}
	meta.SetStatusCondition(
		existingConditions,
		metav1.Condition{
			Type:               string(conditionType),
			Status:             conditionStatus,
			Reason:             string(conditionReason),
			Message:            message,
			ObservedGeneration: generation,
		},
	)
}

// ClearInvalidTransitionStatusConditions clears any invalid transitions if exist
func ClearInvalidTransitionStatusConditions(ibu *ibuv1.ImageBasedUpgrade) {
	for _, condition := range ibu.Status.Conditions {
		if condition.Reason == string(ConditionReasons.InvalidTransition) {
			if condition.Type == string(ConditionTypes.Idle) {
				// revert back to in progress
				SetIdleStatusInProgress(ibu, ConditionReasons.InProgress, InProgress)
			} else if condition.Type == string(ConditionTypes.PrepInProgress) ||
				condition.Type == string(ConditionTypes.UpgradeInProgress) ||
				condition.Type == string(ConditionTypes.RollbackInProgress) {
				meta.RemoveStatusCondition(&ibu.Status.Conditions, condition.Type)
			}
		}
	}
}

// ClearIPInvalidTransitionStatusConditions clears any invalid transitions if exist.
func ClearIPInvalidTransitionStatusConditions(ipc *ipcv1.IPConfig) {
	for _, condition := range ipc.Status.Conditions {
		if condition.Reason == string(ConditionReasons.InvalidTransition) {
			if condition.Type == string(ConditionTypes.Idle) {
				SetIPIdleStatusFalse(ipc, ConditionReasons.ConfigurationInProgress, ConfigurationInProgress)
			} else if condition.Type == string(ConditionTypes.ConfigInProgress) ||
				condition.Type == string(ConditionTypes.RollbackInProgress) {
				meta.RemoveStatusCondition(&ipc.Status.Conditions, condition.Type)
			}
		}
	}
}

// ResetStatusConditions remove all other conditions and sets idle to true
func ResetStatusConditions(existingConditions *[]metav1.Condition, generation int64) {
	for _, condition := range *existingConditions {
		if condition.Type != string(ConditionTypes.Idle) {
			meta.RemoveStatusCondition(existingConditions, condition.Type)
		}
	}
	meta.SetStatusCondition(
		existingConditions,
		metav1.Condition{
			Type:               string(ConditionTypes.Idle),
			Status:             metav1.ConditionTrue,
			Reason:             string(ConditionReasons.Idle),
			Message:            "Idle",
			ObservedGeneration: generation,
		},
	)
}

// IsOnlyIdleConditionTrue returns true only if the conditions slice contains
// exactly one condition of type Idle and it is true. This indicates that
// cleanup has completed and conditions were reset.
func IsIdleConditionTrue(conditions []metav1.Condition) bool {
	if len(conditions) == 0 {
		return false
	}

	idle := meta.FindStatusCondition(conditions, string(ConditionTypes.Idle))
	if idle == nil {
		return false
	}

	return idle.Status == metav1.ConditionTrue
}

// IsStageCompleted checks if the completed condition status for the stage is true
func IsStageCompleted(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) bool {
	condition := GetCompletedCondition(ibu, stage)
	if condition != nil && condition.Status == metav1.ConditionTrue {
		return true
	}
	return false
}

// IsStageFailed checks if the completed condition status for the stage is false
func IsStageFailed(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) bool {
	condition := GetCompletedCondition(ibu, stage)
	if condition != nil && condition.Status == metav1.ConditionFalse {
		return true
	}
	return false
}

// IsStageCompletedOrFailed checks if the completed condition for the stage is present
func IsStageCompletedOrFailed(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) bool {
	condition := GetCompletedCondition(ibu, stage)
	return condition != nil
}

// IsStageInProgress checks if ibu is working on the stage
func IsStageInProgress(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) bool {
	condition := GetInProgressCondition(ibu, stage)
	if stage == ibuv1.Stages.Idle {
		if condition == nil || condition.Status == metav1.ConditionTrue {
			return false
		}

		switch condition.Reason {
		case string(ConditionReasons.Aborting), string(ConditionReasons.AbortFailed), string(ConditionReasons.Finalizing), string(ConditionReasons.FinalizeFailed):
			return true
		}
		// idle reason is in progress
		return false
	}
	// other stages
	if condition != nil && condition.Status == metav1.ConditionTrue {
		return true
	}
	return false
}

// GetInProgressStage returns the stage that is currently in progress
func GetInProgressStage(ibu *ibuv1.ImageBasedUpgrade) ibuv1.ImageBasedUpgradeStage {
	stages := []ibuv1.ImageBasedUpgradeStage{
		ibuv1.Stages.Idle,
		ibuv1.Stages.Prep,
		ibuv1.Stages.Upgrade,
		ibuv1.Stages.Rollback,
	}

	for _, stage := range stages {
		if IsStageInProgress(ibu, stage) {
			return stage
		}
	}
	return ""
}

// GetInProgressCondition returns the in progress condition based on the stage
func GetInProgressCondition(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) *metav1.Condition {
	conditionType := GetInProgressConditionType(stage)
	if conditionType != "" {
		return meta.FindStatusCondition(ibu.Status.Conditions, string(conditionType))
	}
	return nil
}

// GetInProgressConditionType returns the in progress condition type based on the stage
func GetInProgressConditionType(stage ibuv1.ImageBasedUpgradeStage) (conditionType ConditionType) {
	switch stage {
	case ibuv1.Stages.Idle:
		conditionType = ConditionTypes.Idle
	case ibuv1.Stages.Prep:
		conditionType = ConditionTypes.PrepInProgress
	case ibuv1.Stages.Upgrade:
		conditionType = ConditionTypes.UpgradeInProgress
	case ibuv1.Stages.Rollback:
		conditionType = ConditionTypes.RollbackInProgress
	}
	return
}

// GetCompletedCondition returns the completed condition based on the stage
func GetCompletedCondition(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) *metav1.Condition {
	conditionType := GetCompletedConditionType(stage)
	if conditionType != "" {
		return meta.FindStatusCondition(ibu.Status.Conditions, string(conditionType))
	}
	return nil
}

// GetCompletedConditionType returns the completed condition type based on the stage
func GetCompletedConditionType(stage ibuv1.ImageBasedUpgradeStage) (conditionType ConditionType) {
	switch stage {
	case ibuv1.Stages.Idle:
		conditionType = ConditionTypes.Idle
	case ibuv1.Stages.Prep:
		conditionType = ConditionTypes.PrepCompleted
	case ibuv1.Stages.Upgrade:
		conditionType = ConditionTypes.UpgradeCompleted
	case ibuv1.Stages.Rollback:
		conditionType = ConditionTypes.RollbackCompleted
	}
	return
}

// GetIPInProgressConditionType returns the IPConfig in-progress condition type based on the stage
func GetIPInProgressConditionType(stage ipcv1.IPConfigStage) (conditionType ConditionType) {
	switch stage {
	case ipcv1.IPStages.Idle:
		conditionType = ConditionTypes.Idle
	case ipcv1.IPStages.Config:
		conditionType = ConditionTypes.ConfigInProgress
	case ipcv1.IPStages.Rollback:
		conditionType = ConditionTypes.RollbackInProgress
	}
	return
}

// GetIPCompletedConditionType returns the IPConfig completed condition type based on the stage
func GetIPCompletedConditionType(stage ipcv1.IPConfigStage) (conditionType ConditionType) {
	switch stage {
	case ipcv1.IPStages.Idle:
		conditionType = ConditionTypes.Idle
	case ipcv1.IPStages.Config:
		conditionType = ConditionTypes.ConfigCompleted
	case ipcv1.IPStages.Rollback:
		conditionType = ConditionTypes.RollbackCompleted
	}
	return
}

// GetIPInProgressCondition returns the in-progress condition for the given IPConfig stage
func GetIPInProgressCondition(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) *metav1.Condition {
	conditionType := GetIPInProgressConditionType(stage)
	if conditionType != "" {
		return meta.FindStatusCondition(ipc.Status.Conditions, string(conditionType))
	}
	return nil
}

// GetIPCompletedCondition returns the completed condition for the given IPConfig stage
func GetIPCompletedCondition(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) *metav1.Condition {
	conditionType := GetIPCompletedConditionType(stage)
	if conditionType != "" {
		return meta.FindStatusCondition(ipc.Status.Conditions, string(conditionType))
	}
	return nil
}

// SetStatusInvalidTransition updates the given stage status to invalid transition with message
func SetStatusInvalidTransition(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibu.Spec.Stage),
		ConditionReasons.InvalidTransition,
		metav1.ConditionFalse,
		msg,
		ibu.Generation,
	)
}

// SetUpgradeStatusFailed updates the upgrade status to failed with message
func SetUpgradeStatusFailed(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		UpgradeFailed,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ibu.Generation)
}

// SetUpgradeStatusInProgress updates the upgrade status to in progress with message
func SetUpgradeStatusInProgress(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.InProgress,
		metav1.ConditionTrue,
		msg,
		ibu.Generation)
}

// SetUpgradeStatusCompleted updates the upgrade status to completed
func SetUpgradeStatusCompleted(ibu *ibuv1.ImageBasedUpgrade) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Completed,
		metav1.ConditionFalse,
		UpgradeCompleted,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Completed,
		metav1.ConditionTrue,
		UpgradeCompleted,
		ibu.Generation)
}

// SetUpgradeStatusRollbackRequested updates the upgrade status to failed with rollback requested message
func SetUpgradeStatusRollbackRequested(ibu *ibuv1.ImageBasedUpgrade) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		RollbackRequested,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Upgrade),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		RollbackRequested,
		ibu.Generation)
}

// SetPrepStatusInProgress updates the prep status to in progress with message
func SetPrepStatusInProgress(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Prep),
		ConditionReasons.InProgress,
		metav1.ConditionTrue,
		msg,
		ibu.Generation)
}

// SetPrepStatusFailed updates the prep status to failed with message
func SetPrepStatusFailed(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Prep),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		PrepFailed,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Prep),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ibu.Generation)
}

// SetPrepStatusCompleted updates the prep status to completed
func SetPrepStatusCompleted(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Prep),
		ConditionReasons.Completed,
		metav1.ConditionFalse,
		PrepCompleted,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Prep),
		ConditionReasons.Completed,
		metav1.ConditionTrue,
		msg,
		ibu.Generation)
}

// SetRollbackStatusFailed updates the Rollback status to failed with message
func SetRollbackStatusFailed(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Rollback),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		RollbackFailed,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Rollback),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ibu.Generation)
}

// SetRollbackStatusInProgress updates the Rollback status to in progress with message
func SetRollbackStatusInProgress(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Rollback),
		ConditionReasons.InProgress,
		metav1.ConditionTrue,
		msg,
		ibu.Generation)
}

// SetUpgradeStatusCompleted updates the Rollback status to completed
func SetRollbackStatusCompleted(ibu *ibuv1.ImageBasedUpgrade) {
	SetStatusCondition(&ibu.Status.Conditions,
		GetInProgressConditionType(ibuv1.Stages.Rollback),
		ConditionReasons.Completed,
		metav1.ConditionFalse,
		RollbackCompleted,
		ibu.Generation)
	SetStatusCondition(&ibu.Status.Conditions,
		GetCompletedConditionType(ibuv1.Stages.Rollback),
		ConditionReasons.Completed,
		metav1.ConditionTrue,
		RollbackCompleted,
		ibu.Generation)
}

// SetIdleStatusInProgress updates the Idle status to in progress with message
func SetIdleStatusInProgress(ibu *ibuv1.ImageBasedUpgrade, reason ConditionReason, msg string) {
	SetStatusCondition(&ibu.Status.Conditions,
		ConditionTypes.Idle,
		reason,
		metav1.ConditionFalse,
		msg,
		ibu.Generation,
	)
}

// SetIPIdleStatusInProgress updates the IPConfig Idle status to in progress with message
func SetIPIdleStatusFalse(ipc *ipcv1.IPConfig, reason ConditionReason, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		ConditionTypes.Idle,
		reason,
		metav1.ConditionFalse,
		msg,
		ipc.Generation,
	)
}

// SetIBUStatusBlocked updates the given IBU stage in-progress status to Blocked with message.
func SetIBUStatusBlocked(ibu *ibuv1.ImageBasedUpgrade, msg string) {
	ct := GetInProgressConditionType(ibu.Spec.Stage)
	if ct == "" {
		return
	}
	SetStatusCondition(&ibu.Status.Conditions,
		ct,
		ConditionReasons.Blocked,
		metav1.ConditionFalse,
		msg,
		ibu.Generation,
	)
}

// IsIBUStatusBlocked checks if the given IBU stage in-progress status is Blocked.
func IsIBUStatusBlocked(ibu *ibuv1.ImageBasedUpgrade, stage ibuv1.ImageBasedUpgradeStage) bool {
	condition := GetInProgressCondition(ibu, stage)
	return condition != nil && condition.Reason == string(ConditionReasons.Blocked)
}

func UpdateIBUStatus(ctx context.Context, c client.Client, ibu *ibuv1.ImageBasedUpgrade) error {
	if c == nil {
		// In UT code
		return nil
	}

	ibu.Status.ObservedGeneration = ibu.ObjectMeta.Generation

	for i := range ibu.Status.Conditions {
		condition := &ibu.Status.Conditions[i]
		if condition.Type == string(GetCompletedConditionType(ibu.Spec.Stage)) ||
			condition.Type == string(GetInProgressConditionType(ibu.Spec.Stage)) {
			condition.ObservedGeneration = ibu.ObjectMeta.Generation
		}
	}

	if err := c.Status().Update(ctx, ibu); err != nil {
		return fmt.Errorf("failed to update IBU status: %w", err)
	}

	return nil
}

// IsIPStageCompleted checks if the completed condition status for the IPConfig stage is true
func IsIPStageCompleted(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	condition := GetIPCompletedCondition(ipc, stage)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsIPStageFailed checks if the completed condition status for the IPConfig stage is false
func IsIPStageFailed(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	condition := GetIPCompletedCondition(ipc, stage)
	if stage == ipcv1.IPStages.Idle {
		return condition != nil &&
			condition.Status == metav1.ConditionFalse &&
			condition.Reason == string(ConditionReasons.Failed)
	}

	return condition != nil && condition.Status == metav1.ConditionFalse
}

// IsIPStageCompletedOrFailed checks if the completed condition for the IPConfig stage is present
func IsIPStageCompletedOrFailed(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	return IsIPStageCompleted(ipc, stage) ||
		IsIPStageFailed(ipc, stage)
}

// IsIPStageInProgress checks if IPConfig is working on the stage
func IsIPStageInProgress(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	condition := GetIPInProgressCondition(ipc, stage)
	if stage == ipcv1.IPStages.Idle {
		return condition != nil &&
			condition.Status == metav1.ConditionFalse &&
			condition.Reason == string(ConditionReasons.InProgress)
	}

	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsIPStageStatusInvalidTransition checks whether IPConfig indicates an invalid transition request.
func IsIPStageStatusInvalidTransition(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	if ipc == nil {
		return false
	}

	if ct := GetIPInProgressConditionType(stage); ct != "" {
		if c := meta.FindStatusCondition(ipc.Status.Conditions, string(ct)); c != nil {
			return c.Status == metav1.ConditionFalse &&
				c.Reason == string(ConditionReasons.InvalidTransition)
		}
	}

	return false
}

// GetIPInProgressStage returns the IPConfig stage that is currently in progress
func GetIPInProgressStage(ipc *ipcv1.IPConfig) ipcv1.IPConfigStage {
	stages := []ipcv1.IPConfigStage{
		ipcv1.IPStages.Idle,
		ipcv1.IPStages.Config,
		ipcv1.IPStages.Rollback,
	}

	for _, stage := range stages {
		if IsIPStageInProgress(ipc, stage) {
			return stage
		}
	}

	return ""
}

// SetIPStatusInvalidTransition updates the given IP stage status to invalid transition with message
func SetIPStatusInvalidTransition(ipc *ipcv1.IPConfig, msg string) {
	ct := GetIPInProgressConditionType(ipc.Spec.Stage)
	if ct == "" {
		return
	}
	SetStatusCondition(&ipc.Status.Conditions,
		ct,
		ConditionReasons.InvalidTransition,
		metav1.ConditionFalse,
		msg,
		ipc.Generation,
	)
}

// SetIPConfigStatusInProgress updates the IP Config status to in progress with message
func SetIPConfigStatusInProgress(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Config),
		ConditionReasons.InProgress,
		metav1.ConditionTrue,
		msg,
		ipc.Generation)
}

// SetIPConfigStatusFailed updates the IP Config status to failed with message
func SetIPConfigStatusFailed(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPCompletedConditionType(ipcv1.IPStages.Config),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Config),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
}

// SetIPConfigStatusCompleted updates the IP Config status to completed
func SetIPConfigStatusCompleted(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Config),
		ConditionReasons.Completed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPCompletedConditionType(ipcv1.IPStages.Config),
		ConditionReasons.Completed,
		metav1.ConditionTrue,
		msg,
		ipc.Generation)
}

// SetIPRollbackStatusInProgress updates the IP Rollback status to in progress with message
func SetIPRollbackStatusInProgress(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Rollback),
		ConditionReasons.InProgress,
		metav1.ConditionTrue,
		msg,
		ipc.Generation)
}

// SetIPRollbackStatusFailed updates the IP Rollback status to failed with message
func SetIPRollbackStatusFailed(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPCompletedConditionType(ipcv1.IPStages.Rollback),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Rollback),
		ConditionReasons.Failed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
}

// SetIPRollbackStatusCompleted updates the IP Rollback status to completed
func SetIPRollbackStatusCompleted(ipc *ipcv1.IPConfig, msg string) {
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPInProgressConditionType(ipcv1.IPStages.Rollback),
		ConditionReasons.Completed,
		metav1.ConditionFalse,
		msg,
		ipc.Generation)
	SetStatusCondition(&ipc.Status.Conditions,
		GetIPCompletedConditionType(ipcv1.IPStages.Rollback),
		ConditionReasons.Completed,
		metav1.ConditionTrue,
		msg,
		ipc.Generation)
}

// IsIPCStatusBlocked checks if the given IPConfig stage in-progress status is Blocked.
func IsIPCStatusBlocked(ipc *ipcv1.IPConfig, stage ipcv1.IPConfigStage) bool {
	condition := GetIPInProgressCondition(ipc, stage)
	return condition != nil && condition.Reason == string(ConditionReasons.Blocked)
}

// UpdateIPStatus updates IPConfig status and observed generations consistently
func UpdateIPCStatus(ctx context.Context, c client.Client, ipc *ipcv1.IPConfig) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	ipc.Status.ObservedGeneration = ipc.ObjectMeta.Generation

	for i := range ipc.Status.Conditions {
		condition := &ipc.Status.Conditions[i]
		if condition.Type == string(GetIPCompletedConditionType(ipc.Spec.Stage)) ||
			condition.Type == string(GetIPInProgressConditionType(ipc.Spec.Stage)) {
			condition.ObservedGeneration = ipc.ObjectMeta.Generation
		}
	}

	if err := c.Status().Update(ctx, ipc); err != nil {
		return fmt.Errorf("failed to update IPConfig status: %w", err)
	}

	return nil
}

// SetIPStatusBlocked updates the given IPConfig stage in-progress status to Blocked with message.
func SetIPStatusBlocked(ipc *ipcv1.IPConfig, msg string) {
	ct := GetIPInProgressConditionType(ipc.Spec.Stage)
	if ct == "" {
		return
	}
	SetStatusCondition(&ipc.Status.Conditions,
		ct,
		ConditionReasons.Blocked,
		metav1.ConditionFalse,
		msg,
		ipc.Generation,
	)
}
