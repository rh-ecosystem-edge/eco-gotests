package cnfhelper

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/safeapirequest"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	sriovSettleTimeout      = 2 * time.Minute
	rollbackPlanTimeout     = 30 * time.Minute
	bslWaitTimeout          = 10 * time.Minute
	rebootCommandRetries    = 3
	rebootRetryInterval     = 10 * time.Second
	manualCleanupAnnotation = "lca.openshift.io/manual-cleanup-done"
)

// Recover snapshots spoke health and IBU stage, then dispatches leftover abort,
// rollback, manual cleanup, or an SR-IOV reboot. SR-IOV is recovered before
// IBU Abort/Rollback because a bad SriovNetworkNodeState blocks LCA health
// gates. It always finishes with WaitUntilHealthy. Unless a completed Upgrade
// is being left for Ordered e2e rollback, it then waits for Idle with Prep
// valid so the next spec's fail-fast BeforeEach does not race LCA
// Idle=False/Blocked after reboot.
//
// Call Recover only after a failed spec that entered the It body. A passing It
// already waited for spoke health to stay stable, and recovering on success can
// reboot after 68954 and break the post-upgrade reboot count check. Recover
// after a BeforeEach/BeforeAll failure cannot fix that setup error and can hang
// the suite.
//
// If leaveSuccessfulUpgrade is true, a completed Upgrade is left in place so
// Ordered e2e test 69058 can still roll it back. Independent failed specs must
// pass false so leftover Upgrade is rolled back to Idle.
func Recover(leaveSuccessfulUpgrade bool) error {
	report := Check()
	if report.Healthy() {
		klog.V(cnfparams.CNFLogLevel).Info("Recover: spoke health checks passed")
	} else {
		klog.V(cnfparams.CNFLogLevel).Infof("Recover: spoke health: %s", report.FailureSummary())
	}

	if sriovNeedsReboot(report) {
		if recoverErr := recoverSriov(report); recoverErr != nil {
			return recoverErr
		}
	}

	imageUpgrade, err := waitForSpokeIBUObject(IBUAvailableTimeout)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Recover: failed to pull IBU, continuing with health recovery: %v", err)
	} else {
		klog.V(cnfparams.CNFLogLevel).Infof("Recover: spoke IBU %s", formatIBU(imageUpgrade))

		if recoverErr := recoverIBU(imageUpgrade, leaveSuccessfulUpgrade); recoverErr != nil {
			return recoverErr
		}
	}

	if healthErr := WaitUntilHealthy(RecoverHealthTimeout); healthErr != nil {
		return healthErr
	}

	if leaveSuccessfulUpgrade {
		return nil
	}

	return waitUntilPrepReady(abortPlanTimeout)
}

// recoverIBU drives leftover IBU state to Idle, or annotates for manual cleanup
// after AbortFailed/FinalizeFailed.
func recoverIBU(imageUpgrade *lca.ImageBasedUpgradeBuilder, leaveSuccessfulUpgrade bool) error {
	if needsManualCleanup(imageUpgrade) {
		idleFailure, _ := findCondition(imageUpgrade, conditionIdle)

		if err := recoverManualCleanup(); err != nil {
			return err
		}

		// WaitForSpokeIBU fail-fasts on Idle AbortFailed/FinalizeFailed. Ignore
		// the pre-annotation instance until LCA reconciles manual-cleanup-done.
		return waitForSpokeIBUState(SpokeIdle, abortPlanTimeout, idleFailure)
	}

	plan := recoverIdlePlan(imageUpgrade, leaveSuccessfulUpgrade)
	if len(plan) == 0 {
		return nil
	}

	return applyPlanAndWaitIdle(plan)
}

// recoverManualCleanup waits for BackupStorageLocation, then sets
// lca.openshift.io/manual-cleanup-done so LCA retries abort/finalize cleanup.
func recoverManualCleanup() error {
	klog.V(cnfparams.CNFLogLevel).Info(
		"Recover: waiting for BackupStorageLocation before manual-cleanup-done annotation")

	if err := wait.PollUntilContextTimeout(
		context.TODO(), healthPollInterval, bslWaitTimeout, true,
		func(_ context.Context) (bool, error) {
			return !Check().unhealthy(checkBackupStorage), nil
		}); err != nil {
		return fmt.Errorf("BackupStorageLocation did not become available for manual cleanup: %w", err)
	}

	return safeapirequest.Do(func() error {
		imageUpgrade, err := lca.PullImageBasedUpgrade(cnfinittools.TargetSNOAPIClient)
		if err != nil {
			return err
		}

		if imageUpgrade.Definition.Annotations == nil {
			imageUpgrade.Definition.Annotations = map[string]string{}
		}

		imageUpgrade.Definition.Annotations[manualCleanupAnnotation] = "true"
		_, err = imageUpgrade.Update()

		return err
	})
}

// recoverSriov waits briefly for SriovNetworkNodeState to recover, then soft
// reboots the SNO and waits for cluster recovery if it stays unhealthy.
func recoverSriov(report Report) error {
	result, found := report.result(checkSriov)
	message := "unknown SR-IOV failure"

	if found {
		message = result.Message
	}

	klog.V(cnfparams.CNFLogLevel).Infof(
		"Recover: SR-IOV is unhealthy (%s), waiting %s before reboot", message, sriovSettleTimeout)

	settleErr := wait.PollUntilContextTimeout(
		context.TODO(), healthPollInterval, sriovSettleTimeout, true,
		func(_ context.Context) (bool, error) {
			return !Check().unhealthy(checkSriov), nil
		})
	if settleErr == nil {
		klog.V(cnfparams.CNFLogLevel).Info("Recover: SR-IOV became healthy without reboot")

		return nil
	}

	klog.V(cnfparams.CNFLogLevel).Info("Recover: SR-IOV still unhealthy, issuing SNO soft reboot")

	rebootErr := cluster.SoftRebootSNO(
		cnfinittools.TargetSNOAPIClient, rebootCommandRetries, rebootRetryInterval)
	klog.V(cnfparams.CNFLogLevel).Infof("Recover: soft reboot result: %v", rebootErr)

	namespaces := []string{oadpNamespace}
	if cnfinittools.CNFConfig != nil {
		namespaces = []string{
			cnfinittools.CNFConfig.SriovOperatorNamespace,
			cnfinittools.CNFConfig.MCONamespace,
		}
	}

	if err := cluster.WaitForRecover(
		cnfinittools.TargetSNOAPIClient, namespaces, RecoverHealthTimeout); err != nil {
		return fmt.Errorf("cluster did not recover after SR-IOV reboot: %w", err)
	}

	return nil
}

// sriovNeedsReboot reports whether SR-IOV health failed with a syncStatus or
// empty-list error that a reboot can recover.
func sriovNeedsReboot(report Report) bool {
	result, found := report.result(checkSriov)
	if !found || result.Healthy || result.Skipped {
		return false
	}

	return strings.Contains(result.Message, "syncStatus") ||
		strings.Contains(result.Message, "no SriovNetworkNodeStates")
}

// recoverIdlePlan returns the IBGU actions needed to return leftover IBU state
// to Idle. Spec.stage Idle is left untouched because Abort is not a valid next
// stage once LCA has already moved to Idle. A completed Upgrade is left only
// when leaveSuccessfulUpgrade is true.
func recoverIdlePlan(imageUpgrade *lca.ImageBasedUpgradeBuilder, leaveSuccessfulUpgrade bool) []string {
	if currentStage(imageUpgrade) == stageIdle {
		if !isPrepReady(imageUpgrade) {
			klog.V(cnfparams.CNFLogLevel).Infof(
				"Recover: spoke IBU already at Idle, skipping abort: %s",
				formatIBU(imageUpgrade))
		}

		return nil
	}

	if leaveSuccessfulUpgrade && isSuccessfulUpgrade(imageUpgrade) {
		klog.V(cnfparams.CNFLogLevel).Info("Recover: leaving successful Upgrade in place for Ordered e2e rollback")

		return nil
	}

	return idlePlan(imageUpgrade)
}

// idlePlan selects Abort, Rollback+FinalizeRollback, or FinalizeRollback from
// the IBU validNextStages and current stage.
func idlePlan(imageUpgrade *lca.ImageBasedUpgradeBuilder) []string {
	if hasValidNext(imageUpgrade, stageRollback) {
		return []string{ibguv1alpha1.Rollback, ibguv1alpha1.FinalizeRollback}
	}

	if hasValidNext(imageUpgrade, stageIdle) && currentStage(imageUpgrade) == stageRollback {
		return []string{ibguv1alpha1.FinalizeRollback}
	}

	return []string{ibguv1alpha1.Abort}
}

// applyPlanAndWaitIdle creates a recover IBGU with actions, waits for spoke Idle
// and IBGU Completed, then deletes the recover IBGU. TALM Rollback/FinalizeRollback
// CGUs only select clusters with the matching lcm.openshift.io/ibgu-*-completed
// labels, so those labels are applied first when the original IBGU was deleted
// before it could set them.
func applyPlanAndWaitIdle(actions []string) error {
	if err := deleteIbgu(RecoverIbguName); err != nil {
		return err
	}

	if err := labelClustersForPlan(actions); err != nil {
		return err
	}

	builder, err := createPlanIbgu(RecoverIbguName, actions)
	if err != nil {
		return err
	}

	timeout := abortPlanTimeout
	if slices.Contains(actions, ibguv1alpha1.Rollback) ||
		slices.Contains(actions, ibguv1alpha1.FinalizeRollback) {
		timeout = rollbackPlanTimeout
	}

	if err := waitForRecoverPlanIdle(builder, timeout); err != nil {
		return err
	}

	if err := WaitForIbgu(builder, IbguCompleted, timeout); err != nil {
		return err
	}

	return deleteIbgu(RecoverIbguName)
}

// waitForRecoverPlanIdle waits for spoke Idle while fail-fasting if the recover
// IBGU completes without selecting any clusters.
func waitForRecoverPlanIdle(builder *ibgu.IbguBuilder, timeout time.Duration) error {
	var lastIBU, lastIBGU string

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), spokePollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			imageUpgrade, err := pullSpokeIBU()
			if err != nil {
				klog.V(cnfparams.CNFLogLevel).Infof("waiting for spoke IBU Idle: pull failed: %v", err)

				return false, nil
			}

			lastIBU = formatIBU(imageUpgrade)
			if failErr := unexpectedIBUFailure(imageUpgrade, SpokeIdle); failErr != nil {
				return false, failErr
			}

			if builder != nil {
				object, getErr := builder.Get()
				if getErr == nil {
					builder.Object = object
					builder.Definition = object
					lastIBGU = formatIbgu(object)

					if failErr := unexpectedIbguState(object, IbguCompleted); failErr != nil {
						return false, failErr
					}
				}
			}

			if matchesSpokeState(imageUpgrade, SpokeIdle) {
				return true, nil
			}

			klog.V(cnfparams.CNFLogLevel).Infof(
				"waiting for spoke IBU Idle; current: %s; ibgu: %s", lastIBU, lastIBGU)

			return false, nil
		})
	if pollErr != nil {
		return fmt.Errorf("spoke IBU did not reach Idle within %s: %w (last status: %s; ibgu: %s)",
			timeout, pollErr, lastIBU, lastIBGU)
	}

	return nil
}
