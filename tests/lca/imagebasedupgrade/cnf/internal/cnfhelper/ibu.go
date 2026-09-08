package cnfhelper

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// IBUAvailableTimeout is how long to wait for the spoke IBU CR after an ostree pivot.
	IBUAvailableTimeout = 15 * time.Minute

	spokePollInterval  = 5 * time.Second
	abortPlanTimeout   = 10 * time.Minute
	ensureReadyTimeout = 1 * time.Minute

	stageIdle     = "Idle"
	stagePrep     = "Prep"
	stageUpgrade  = "Upgrade"
	stageRollback = "Rollback"

	conditionIdle               = "Idle"
	conditionPrepCompleted      = "PrepCompleted"
	conditionUpgradeCompleted   = "UpgradeCompleted"
	conditionRollbackCompleted  = "RollbackCompleted"
	conditionPrepInProgress     = "PrepInProgress"
	conditionUpgradeInProgress  = "UpgradeInProgress"
	conditionRollbackInProgress = "RollbackInProgress"

	reasonCompleted      = "Completed"
	reasonFailed         = "Failed"
	reasonTimedOut       = "TimedOut"
	reasonAbortFailed    = "AbortFailed"
	reasonFinalizeFailed = "FinalizeFailed"
)

// SpokeIBUState is an expected terminal spoke IBU state.
type SpokeIBUState string

const (
	// SpokeIdle is Idle with the Idle condition True.
	SpokeIdle SpokeIBUState = "Idle"
	// SpokePrepCompleted is Prep completed successfully.
	SpokePrepCompleted SpokeIBUState = "PrepCompleted"
	// SpokeUpgradeCompleted is Upgrade completed successfully.
	SpokeUpgradeCompleted SpokeIBUState = "UpgradeCompleted"
	// SpokeUpgradeInProgress is Upgrade started and not yet completed. Abort
	// is only valid until pre-pivot, so a successful pull at this state means
	// the API is still up and pivot has not begun.
	SpokeUpgradeInProgress SpokeIBUState = "UpgradeInProgress"
	// SpokeRollbackCompleted is Rollback completed successfully.
	SpokeRollbackCompleted SpokeIBUState = "RollbackCompleted"
	// SpokeUpgradeFailed is UpgradeCompleted with Reason Failed.
	SpokeUpgradeFailed SpokeIBUState = "UpgradeFailed"
)

// WaitForSpokeIBU waits until the spoke IBU reaches expected, failing immediately
// if a stage reports Failed and that failure is not the expected end state.
// When expected is SpokeRollbackCompleted, SpokeIdle also counts as success
// because FinalizeRollback can clear RollbackCompleted before the next poll.
// When expected is SpokeUpgradeInProgress, a completed Upgrade fails immediately
// because the pre-pivot abort window has already closed.
func WaitForSpokeIBU(expected SpokeIBUState, timeout time.Duration) error {
	return waitForSpokeIBUState(expected, timeout, metav1.Condition{})
}

// waitForSpokeIBUState waits for expected. ignoreFailure, when set, is the
// pre-existing Idle AbortFailed/FinalizeFailed condition that Recover already
// observed; that instance is not treated as a new terminal failure.
func waitForSpokeIBUState(expected SpokeIBUState, timeout time.Duration, ignoreFailure metav1.Condition) error {
	var lastStatus string

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), spokePollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			imageUpgrade, err := pullSpokeIBU()
			if err != nil {
				klog.V(cnfparams.CNFLogLevel).Infof("waiting for spoke IBU %s: pull failed: %v", expected, err)

				return false, nil
			}

			lastStatus = formatIBU(imageUpgrade)

			if failErr := unexpectedIBUFailureIgnoring(imageUpgrade, expected, ignoreFailure); failErr != nil {
				return false, failErr
			}

			if expected == SpokeUpgradeInProgress && matchesSpokeState(imageUpgrade, SpokeUpgradeCompleted) {
				return false, fmt.Errorf(
					"spoke IBU completed Upgrade before abort (pivot already happened): %s", lastStatus)
			}

			if matchesSpokeState(imageUpgrade, expected) {
				return true, nil
			}

			if expected == SpokeRollbackCompleted && matchesSpokeState(imageUpgrade, SpokeIdle) {
				return true, nil
			}

			klog.V(cnfparams.CNFLogLevel).Infof("waiting for spoke IBU %s; current: %s", expected, lastStatus)

			return false, nil
		})
	if pollErr != nil {
		return fmt.Errorf("spoke IBU did not reach %s within %s: %w (last status: %s)",
			expected, timeout, pollErr, lastStatus)
	}

	return nil
}

// EnsureSpokeReadyForNextIbgu is a fail-fast BeforeEach sanity check. It retries
// IBU pulls for up to a minute, then immediately fails if the spoke is not Idle
// with Prep valid or if a single health snapshot fails. It does not wait for
// operators to stay healthy; BeforeSuite and the previous spec's WaitUntilHealthy
// are responsible for that. It does not recover leftover state; Recover in
// AfterEach does that.
func EnsureSpokeReadyForNextIbgu() error {
	imageUpgrade, err := waitForSpokeIBUObject(ensureReadyTimeout)
	if err != nil {
		return err
	}

	if !isPrepReady(imageUpgrade) {
		return notPrepReadyError(imageUpgrade)
	}

	report := Check()
	if !report.Healthy() {
		return fmt.Errorf("spoke is not healthy: %s", report.FailureSummary())
	}

	return nil
}

// WaitUntilIBUAvailable waits until the spoke ImageBasedUpgrade CR exists. After
// an ostree pivot the CR is missing until LCA recreates it, and a single pull
// can also return a nil object when the API is still coming up.
func WaitUntilIBUAvailable(timeout time.Duration) error {
	_, err := waitForSpokeIBUObject(timeout)

	return err
}

// waitForSpokeIBUObject retries PullImageBasedUpgrade until the IBU object exists.
// After an ostree pivot the CR is missing until LCA recreates it.
func waitForSpokeIBUObject(timeout time.Duration) (*lca.ImageBasedUpgradeBuilder, error) {
	var imageUpgrade *lca.ImageBasedUpgradeBuilder

	var lastErr error

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), spokePollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			pulled, err := pullSpokeIBU()
			if err != nil {
				lastErr = err
				klog.V(cnfparams.CNFLogLevel).Infof("waiting for spoke IBU object: %v", err)

				return false, nil
			}

			imageUpgrade = pulled

			return true, nil
		})
	if pollErr != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("spoke IBU was not available within %s: %w", timeout, lastErr)
		}

		return nil, fmt.Errorf("spoke IBU was not available within %s: %w", timeout, pollErr)
	}

	return imageUpgrade, nil
}

// pullSpokeIBU pulls the spoke IBU and requires a real object. eco-goinfra Exists
// treats non-NotFound API errors as exists, which can return a nil object.
func pullSpokeIBU() (*lca.ImageBasedUpgradeBuilder, error) {
	imageUpgrade, err := lca.PullImageBasedUpgrade(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		return nil, err
	}

	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return nil, fmt.Errorf("imagebasedupgrade object upgrade does not exist")
	}

	return imageUpgrade, nil
}

// waitUntilPrepReady waits until the spoke IBU is Idle with Prep in
// validNextStages. After reboot LCA can report Idle=False/Blocked (IPC is not
// idle) while ostree IPC is still busy; Abort cannot clear that.
func waitUntilPrepReady(timeout time.Duration) error {
	var lastStatus string

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), spokePollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			imageUpgrade, err := pullSpokeIBU()
			if err != nil {
				klog.V(cnfparams.CNFLogLevel).Infof(
					"waiting for spoke IBU Idle with Prep valid: pull failed: %v", err)

				return false, nil
			}

			lastStatus = formatIBU(imageUpgrade)

			if failErr := unexpectedIBUFailure(imageUpgrade, SpokeIdle); failErr != nil {
				return false, failErr
			}

			if isPrepReady(imageUpgrade) {
				return true, nil
			}

			klog.V(cnfparams.CNFLogLevel).Infof(
				"waiting for spoke IBU Idle with Prep valid; current: %s", lastStatus)

			return false, nil
		})
	if pollErr != nil {
		return fmt.Errorf("spoke IBU did not become Idle with Prep valid within %s: %w (last status: %s)",
			timeout, pollErr, lastStatus)
	}

	return nil
}

// isPrepReady reports whether the spoke IBU is Idle with the Idle condition
// True and Prep in validNextStages.
func isPrepReady(imageUpgrade *lca.ImageBasedUpgradeBuilder) bool {
	return isIdleWithPrepValid(imageUpgrade) &&
		conditionStatus(imageUpgrade, conditionIdle) == metav1.ConditionTrue
}

// isIdleWithPrepValid reports whether spec.stage is Idle and Prep is a valid
// next stage. The Idle condition may still be False.
func isIdleWithPrepValid(imageUpgrade *lca.ImageBasedUpgradeBuilder) bool {
	return currentStage(imageUpgrade) == stageIdle && hasValidNext(imageUpgrade, stagePrep)
}

// HasSuccessfulUpgrade reports whether the spoke IBU completed Upgrade and
// Rollback is a valid next stage. Ordered e2e rollback (69058) uses this so it
// can run alone after a previous successful 68954 job.
func HasSuccessfulUpgrade() (bool, error) {
	imageUpgrade, err := pullSpokeIBU()
	if err != nil {
		return false, err
	}

	return isSuccessfulUpgrade(imageUpgrade), nil
}

// isSuccessfulUpgrade reports whether Upgrade completed and Rollback is a valid
// next stage, so Recover must not abort it.
func isSuccessfulUpgrade(imageUpgrade *lca.ImageBasedUpgradeBuilder) bool {
	return conditionStatus(imageUpgrade, conditionUpgradeCompleted) == metav1.ConditionTrue &&
		conditionReason(imageUpgrade, conditionUpgradeCompleted) == reasonCompleted &&
		hasValidNext(imageUpgrade, stageRollback)
}

// needsManualCleanup reports whether Idle is AbortFailed or FinalizeFailed and
// LCA is waiting for the manual-cleanup-done annotation.
func needsManualCleanup(imageUpgrade *lca.ImageBasedUpgradeBuilder) bool {
	reason := conditionReason(imageUpgrade, conditionIdle)

	return reason == reasonAbortFailed || reason == reasonFinalizeFailed
}

// currentStage returns the IBU spec.stage, or empty if the object is missing.
func currentStage(imageUpgrade *lca.ImageBasedUpgradeBuilder) string {
	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return ""
	}

	return string(imageUpgrade.Object.Spec.Stage)
}

// hasValidNext reports whether stage is listed in IBU status.validNextStages.
func hasValidNext(imageUpgrade *lca.ImageBasedUpgradeBuilder, stage string) bool {
	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return false
	}

	for _, nextStage := range imageUpgrade.Object.Status.ValidNextStages {
		if string(nextStage) == stage {
			return true
		}
	}

	return false
}

// conditionStatus returns the status of the named IBU condition, or empty if it
// is not present.
func conditionStatus(imageUpgrade *lca.ImageBasedUpgradeBuilder, conditionType string) metav1.ConditionStatus {
	condition, found := findCondition(imageUpgrade, conditionType)
	if !found {
		return ""
	}

	return condition.Status
}

// conditionReason returns the reason of the named IBU condition, or empty if it
// is not present.
func conditionReason(imageUpgrade *lca.ImageBasedUpgradeBuilder, conditionType string) string {
	condition, found := findCondition(imageUpgrade, conditionType)
	if !found {
		return ""
	}

	return condition.Reason
}

// findCondition returns the named IBU condition if it is present.
func findCondition(imageUpgrade *lca.ImageBasedUpgradeBuilder, conditionType string) (metav1.Condition, bool) {
	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return metav1.Condition{}, false
	}

	for _, condition := range imageUpgrade.Object.Status.Conditions {
		if condition.Type == conditionType {
			return condition, true
		}
	}

	return metav1.Condition{}, false
}

// matchesSpokeState reports whether the IBU currently matches expected.
func matchesSpokeState(imageUpgrade *lca.ImageBasedUpgradeBuilder, expected SpokeIBUState) bool {
	switch expected {
	case SpokeIdle:
		return currentStage(imageUpgrade) == stageIdle &&
			conditionStatus(imageUpgrade, conditionIdle) == metav1.ConditionTrue
	case SpokePrepCompleted:
		return completedCondition(imageUpgrade, conditionPrepCompleted, conditionPrepInProgress, "Prep completed")
	case SpokeUpgradeCompleted:
		return completedCondition(imageUpgrade, conditionUpgradeCompleted, conditionUpgradeInProgress, "Upgrade completed")
	case SpokeUpgradeInProgress:
		return currentStage(imageUpgrade) == stageUpgrade &&
			conditionStatus(imageUpgrade, conditionUpgradeInProgress) == metav1.ConditionTrue
	case SpokeRollbackCompleted:
		return completedCondition(imageUpgrade, conditionRollbackCompleted, conditionRollbackInProgress, "Rollback completed")
	case SpokeUpgradeFailed:
		return conditionStatus(imageUpgrade, conditionUpgradeCompleted) == metav1.ConditionFalse &&
			conditionReason(imageUpgrade, conditionUpgradeCompleted) == reasonFailed
	default:
		return false
	}
}

// completedCondition reports a successful stage via Completed=True/Completed or
// the older InProgress=False/Completed message used by some LCA versions.
func completedCondition(
	imageUpgrade *lca.ImageBasedUpgradeBuilder, completedType, inProgressType, message string) bool {
	if conditionStatus(imageUpgrade, completedType) == metav1.ConditionTrue &&
		conditionReason(imageUpgrade, completedType) == reasonCompleted {
		return true
	}

	condition, found := findCondition(imageUpgrade, inProgressType)
	if !found {
		return false
	}

	return condition.Status == metav1.ConditionFalse &&
		condition.Reason == reasonCompleted &&
		condition.Message == message
}

// unexpectedIBUFailure fail-fasts on Failed/TimedOut for the stage being waited
// for. Other stages are ignored because LCA keeps prior conditions (for example
// UpgradeCompleted=Failed) until ResetStatusConditions when Idle is reached.
func unexpectedIBUFailure(imageUpgrade *lca.ImageBasedUpgradeBuilder, expected SpokeIBUState) error {
	return unexpectedIBUFailureIgnoring(imageUpgrade, expected, metav1.Condition{})
}

// unexpectedIBUFailureIgnoring is unexpectedIBUFailure, except ignoreFailure is
// skipped until Type, Reason, or LastTransitionTime changes.
func unexpectedIBUFailureIgnoring(
	imageUpgrade *lca.ImageBasedUpgradeBuilder, expected SpokeIBUState, ignoreFailure metav1.Condition) error {
	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return nil
	}

	watched := failureWatchTypes(expected)
	if len(watched) == 0 {
		return nil
	}

	for _, condition := range imageUpgrade.Object.Status.Conditions {
		if !slices.Contains(watched, condition.Type) || !isFailureReason(condition.Reason) {
			continue
		}

		if isIgnoredFailure(condition, ignoreFailure) {
			continue
		}

		return fmt.Errorf("spoke IBU failed while waiting for %s: type=%s status=%s reason=%s message=%s (%s)",
			expected, condition.Type, condition.Status, condition.Reason, condition.Message, formatIBU(imageUpgrade))
	}

	return nil
}

// isIgnoredFailure reports whether condition is the same AbortFailed/FinalizeFailed
// instance Recover already observed before setting manual-cleanup-done.
func isIgnoredFailure(condition, ignoreFailure metav1.Condition) bool {
	if ignoreFailure.Type == "" || ignoreFailure.LastTransitionTime.IsZero() {
		return false
	}

	return condition.Type == ignoreFailure.Type &&
		condition.Reason == ignoreFailure.Reason &&
		condition.LastTransitionTime.Equal(&ignoreFailure.LastTransitionTime)
}

// failureWatchTypes returns the IBU condition types that should fail-fast while
// waiting for expected. SpokeUpgradeFailed watches nothing so Failed is the goal.
func failureWatchTypes(expected SpokeIBUState) []string {
	switch expected {
	case SpokePrepCompleted:
		return []string{conditionPrepCompleted, conditionPrepInProgress}
	case SpokeUpgradeCompleted, SpokeUpgradeInProgress:
		return []string{conditionUpgradeCompleted, conditionUpgradeInProgress}
	case SpokeRollbackCompleted:
		return []string{conditionRollbackCompleted, conditionRollbackInProgress}
	case SpokeIdle:
		return []string{conditionIdle}
	case SpokeUpgradeFailed:
		return nil
	default:
		return nil
	}
}

// isFailureReason reports whether reason is a terminal IBU failure.
func isFailureReason(reason string) bool {
	switch reason {
	case reasonFailed, reasonTimedOut, reasonAbortFailed, reasonFinalizeFailed:
		return true
	default:
		return false
	}
}

// formatIBU returns a compact stage/validNextStages/conditions string for logs.
func formatIBU(imageUpgrade *lca.ImageBasedUpgradeBuilder) string {
	if imageUpgrade == nil || imageUpgrade.Object == nil {
		return "ibu=<nil>"
	}

	return fmt.Sprintf("stage=%s validNextStages=%v conditions=%s",
		imageUpgrade.Object.Spec.Stage,
		imageUpgrade.Object.Status.ValidNextStages,
		formatConditions(imageUpgrade.Object.Status.Conditions))
}

// formatConditions joins condition type/status/reason/message for log output.
func formatConditions(conditions []metav1.Condition) string {
	parts := make([]string, 0, len(conditions))

	for _, condition := range conditions {
		parts = append(parts, fmt.Sprintf("%s=%s/%s (%s)",
			condition.Type, condition.Status, condition.Reason, condition.Message))
	}

	return strings.Join(parts, "; ")
}

// notPrepReadyError describes an IBU that is not Idle with Prep in validNextStages.
func notPrepReadyError(imageUpgrade *lca.ImageBasedUpgradeBuilder) error {
	return fmt.Errorf("spoke IBU is not Idle with Prep in validNextStages: %s", formatIBU(imageUpgrade))
}
