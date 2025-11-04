package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
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
	Idle              ConditionReason
	Completed         ConditionReason
	Failed            ConditionReason
	TimedOut          ConditionReason
	InProgress        ConditionReason
	Aborting          ConditionReason
	AbortCompleted    ConditionReason
	AbortFailed       ConditionReason
	Finalizing        ConditionReason
	FinalizeCompleted ConditionReason
	FinalizeFailed    ConditionReason
	InvalidTransition ConditionReason
}{
	Idle:              "Idle",
	Completed:         "Completed",
	Failed:            "Failed",
	TimedOut:          "TimedOut",
	InProgress:        "InProgress",
	Aborting:          "Aborting",
	AbortCompleted:    "AbortCompleted",
	AbortFailed:       "AbortFailed",
	Finalizing:        "Finalizing",
	FinalizeCompleted: "FinalizeCompleted",
	FinalizeFailed:    "FinalizeFailed",
	InvalidTransition: "InvalidTransition",
}

// Common condition messages
// Note: This is not a complete list and does not include the custom messages
const (
	InProgress        = "In progress"
	Finalizing        = "Finalizing"
	Aborting          = "Aborting"
	PrepCompleted     = "Prep completed"
	PrepFailed        = "Prep failed"
	UpgradeCompleted  = "Upgrade completed"
	UpgradeFailed     = "Upgrade failed"
	RollbackCompleted = "Rollback completed"
	RollbackFailed    = "Rollback failed"
	RollbackRequested = "Rollback requested"
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
	if condition != nil {
		return true
	}
	return false
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
