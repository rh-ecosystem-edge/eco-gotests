package cnfhelper

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RecoverIbguName is created by Recover and deleted before/after that plan.
	RecoverIbguName = "recoveribgu"
	// IbguNamespace is the hub namespace used for TALM IBGUs.
	IbguNamespace = "default"

	ibguPollInterval   = 10 * time.Second
	ibguDeleteTimeout  = 1 * time.Minute
	labelUpdateRetries = 5

	ibguPrepCompletedLabel     = "lcm.openshift.io/ibgu-prep-completed"
	ibguUpgradeCompletedLabel  = "lcm.openshift.io/ibgu-upgrade-completed"
	ibguRollbackCompletedLabel = "lcm.openshift.io/ibgu-rollback-completed"
	localClusterLabel          = "local-cluster"
	ibguConditionProgressing   = "Progressing"
)

var (
	// ClusterLabelSelector is the hub cluster label applied to TALM IBGUs.
	ClusterLabelSelector = map[string]string{"common": "true"}
)

// IbguState is an expected terminal hub IBGU state.
type IbguState string

const (
	// IbguCompleted is Progressing=False Reason=Completed.
	IbguCompleted IbguState = "Completed"
	// IbguFailed is a failed or timed-out IBGU, including cluster failedActions.
	IbguFailed IbguState = "Failed"
)

// NewIbgu returns a hub IBGU builder with the CNF seed image, OADP content,
// and cluster selector. The caller must set the plan and Create it.
func NewIbgu(name string) *ibgu.IbguBuilder {
	return ibgu.NewIbguBuilder(cnfinittools.TargetHubAPIClient, name, IbguNamespace).
		WithClusterLabelSelectors(ClusterLabelSelector).
		WithSeedImageRef(cnfinittools.CNFConfig.IbguSeedImage, cnfinittools.CNFConfig.IbguSeedImageVersion).
		WithOadpContent(cnfinittools.CNFConfig.IbguOadpCmName, cnfinittools.CNFConfig.IbguOadpCmNamespace)
}

// DeleteIbgus deletes the named hub IBGUs if they exist. It does not recover spoke health.
func DeleteIbgus(names ...string) error {
	var deleteErrs []string

	for _, name := range names {
		if err := deleteIbgu(name); err != nil {
			deleteErrs = append(deleteErrs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	if len(deleteErrs) > 0 {
		return fmt.Errorf("failed to delete leftover IBGUs: %s", strings.Join(deleteErrs, "; "))
	}

	return nil
}

// deleteIbgu deletes the named hub IBGU if it exists.
func deleteIbgu(name string) error {
	_, err := ibgu.NewIbguBuilder(cnfinittools.TargetHubAPIClient, name, IbguNamespace).
		DeleteAndWait(ibguDeleteTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete IBGU %s: %w", name, err)
	}

	return nil
}

// createPlanIbgu creates a hub IBGU named name with the given TALM actions.
// AutoRollbackOnFailure is copied from the spoke IBU so TALM does not try to
// clear it; LCA rejects that change while the IBU is in progress.
func createPlanIbgu(name string, actions []string) (*ibgu.IbguBuilder, error) {
	if cnfinittools.CNFConfig == nil {
		return nil, fmt.Errorf("CNF config is nil")
	}

	builder := NewIbgu(name)

	if imageUpgrade, err := pullSpokeIBU(); err == nil {
		if autoRollback := imageUpgrade.Object.Spec.AutoRollbackOnFailure; autoRollback != nil {
			builder = builder.WithAutoRollbackOnFailure(autoRollback.InitMonitorTimeoutSeconds)
		}
	}

	created, err := builder.WithPlan(actions, 5, 30).Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create IBGU %s with plan %v: %w", name, actions, err)
	}

	return created, nil
}

// WaitForIbgu waits until the hub IBGU reaches expected, failing immediately
// when the opposite terminal state is observed.
func WaitForIbgu(builder *ibgu.IbguBuilder, expected IbguState, timeout time.Duration) error {
	if builder == nil {
		return fmt.Errorf("ibgu builder is nil")
	}

	var lastStatus string

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), ibguPollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			object, err := builder.Get()
			if err != nil {
				klog.V(cnfparams.CNFLogLevel).Infof("waiting for IBGU %s: get failed: %v", expected, err)

				return false, nil
			}

			builder.Object = object
			builder.Definition = object
			lastStatus = formatIbgu(object)

			if failErr := unexpectedIbguState(object, expected); failErr != nil {
				return false, failErr
			}

			if matchesIbguState(object, expected) {
				return true, nil
			}

			klog.V(cnfparams.CNFLogLevel).Infof("waiting for IBGU %s; current: %s", expected, lastStatus)

			return false, nil
		})
	if pollErr != nil {
		return fmt.Errorf("IBGU did not reach %s within %s: %w (last status: %s)",
			expected, timeout, pollErr, lastStatus)
	}

	return nil
}

// matchesIbguState reports whether the hub IBGU currently matches expected.
func matchesIbguState(object *ibguv1alpha1.ImageBasedGroupUpgrade, expected IbguState) bool {
	switch expected {
	case IbguCompleted:
		return ibguReasonIs(object, reasonCompleted) && !ibguHasFailedActions(object)
	case IbguFailed:
		return ibguReasonIs(object, reasonFailed) ||
			ibguReasonIs(object, reasonTimedOut) ||
			ibguHasFailedActions(object)
	default:
		return false
	}
}

// unexpectedIbguState fail-fasts when the IBGU reached the opposite terminal
// state from expected.
func unexpectedIbguState(object *ibguv1alpha1.ImageBasedGroupUpgrade, expected IbguState) error {
	if expected == IbguCompleted && ibguCompletedWithoutClusters(object) {
		return fmt.Errorf("IBGU completed without selecting any clusters: %s", formatIbgu(object))
	}

	if expected == IbguCompleted && (ibguReasonIs(object, reasonFailed) ||
		ibguReasonIs(object, reasonTimedOut) || ibguHasFailedActions(object)) {
		return fmt.Errorf("IBGU failed while waiting for Completed: %s", formatIbgu(object))
	}

	if expected == IbguFailed && ibguReasonIs(object, reasonCompleted) && !ibguHasFailedActions(object) {
		return fmt.Errorf("IBGU completed while waiting for Failed: %s", formatIbgu(object))
	}

	return nil
}

// ibguCompletedWithoutClusters reports a vacuous TALM completion when no
// clusters were selected for the plan.
func ibguCompletedWithoutClusters(object *ibguv1alpha1.ImageBasedGroupUpgrade) bool {
	if object == nil {
		return false
	}

	return ibguReasonIs(object, reasonCompleted) && len(object.Status.Clusters) == 0
}

// ibguReasonIs reports whether Progressing is False with the given reason.
func ibguReasonIs(object *ibguv1alpha1.ImageBasedGroupUpgrade, reason string) bool {
	if object == nil {
		return false
	}

	for _, condition := range object.Status.Conditions {
		if condition.Type == ibguConditionProgressing &&
			condition.Status == metav1.ConditionFalse &&
			condition.Reason == reason {
			return true
		}
	}

	return false
}

// ibguHasFailedActions reports whether any cluster in the IBGU status lists a
// failed action.
func ibguHasFailedActions(object *ibguv1alpha1.ImageBasedGroupUpgrade) bool {
	if object == nil {
		return false
	}

	for _, clusterState := range object.Status.Clusters {
		if len(clusterState.FailedActions) > 0 {
			return true
		}
	}

	return false
}

// formatIbgu returns a compact name/conditions/clusters string for logs.
func formatIbgu(object *ibguv1alpha1.ImageBasedGroupUpgrade) string {
	if object == nil {
		return "ibgu=<nil>"
	}

	return fmt.Sprintf("name=%s/%s conditions=%s clusters=%v",
		object.Namespace, object.Name, formatConditions(object.Status.Conditions), object.Status.Clusters)
}

// labelClustersForPlan sets the TALM IBGU action labels so Rollback/Abort CGUs
// actually select the spoke. Rollback requires ibgu-upgrade-completed; Abort
// requires that label to be absent.
func labelClustersForPlan(actions []string) error {
	if cnfinittools.TargetHubAPIClient == nil {
		return fmt.Errorf("target hub API client is nil")
	}

	clusters, err := ocm.ListManagedClusters(
		cnfinittools.TargetHubAPIClient, goclient.MatchingLabels(ClusterLabelSelector))
	if err != nil {
		return fmt.Errorf("failed to list managed clusters for IBGU plan: %w", err)
	}

	var labeled []string

	for _, managedCluster := range clusters {
		if managedCluster == nil || managedCluster.Definition == nil || isLocalManagedCluster(managedCluster) {
			continue
		}

		if err := patchClusterActionLabels(managedCluster, actions); err != nil {
			return err
		}

		labeled = append(labeled, managedCluster.Definition.Name)
	}

	if len(labeled) == 0 {
		return fmt.Errorf("no managed clusters matched %v for IBGU plan %v", ClusterLabelSelector, actions)
	}

	klog.V(cnfparams.CNFLogLevel).Infof("labeled managed clusters %v for IBGU plan %v", labeled, actions)

	return nil
}

// patchClusterActionLabels adds or removes IBGU stage labels required by TALM
// for the given recover plan actions.
func patchClusterActionLabels(cluster *ocm.ManagedClusterBuilder, actions []string) error {
	var lastErr error

	for attempt := 0; attempt < labelUpdateRetries; attempt++ {
		current, err := cluster.Get()
		if err != nil {
			return fmt.Errorf("failed to get managedcluster %s: %w", cluster.Definition.Name, err)
		}

		if current.Labels == nil {
			current.Labels = map[string]string{}
		}

		if !mutateActionLabels(current.Labels, actions) {
			return nil
		}

		cluster.Definition = current

		_, lastErr = cluster.Update()
		if lastErr == nil {
			return nil
		}

		if !k8serrors.IsConflict(lastErr) {
			return fmt.Errorf("failed to update managedcluster %s labels: %w", current.Name, lastErr)
		}
	}

	return fmt.Errorf("failed to update managedcluster %s labels: %w", cluster.Definition.Name, lastErr)
}

// mutateActionLabels updates labels for TALM IBGU action selection. It reports
// whether any label changed.
func mutateActionLabels(labels map[string]string, actions []string) bool {
	changed := false

	if slices.Contains(actions, ibguv1alpha1.Rollback) {
		if _, found := labels[ibguUpgradeCompletedLabel]; !found {
			labels[ibguUpgradeCompletedLabel] = ""
			changed = true
		}
	}

	if slices.Contains(actions, ibguv1alpha1.Abort) {
		if _, found := labels[ibguUpgradeCompletedLabel]; found {
			delete(labels, ibguUpgradeCompletedLabel)

			changed = true
		}
	}

	if slices.Contains(actions, ibguv1alpha1.FinalizeRollback) &&
		!slices.Contains(actions, ibguv1alpha1.Rollback) {
		if _, found := labels[ibguRollbackCompletedLabel]; !found {
			labels[ibguRollbackCompletedLabel] = ""
			changed = true
		}
	}

	if slices.Contains(actions, ibguv1alpha1.Abort) ||
		slices.Contains(actions, ibguv1alpha1.Rollback) {
		if _, found := labels[ibguPrepCompletedLabel]; !found {
			labels[ibguPrepCompletedLabel] = ""
			changed = true
		}
	}

	return changed
}

// isLocalManagedCluster reports whether the cluster is the ACM hub local-cluster.
func isLocalManagedCluster(cluster *ocm.ManagedClusterBuilder) bool {
	if cluster == nil || cluster.Definition == nil || cluster.Definition.Labels == nil {
		return false
	}

	return cluster.Definition.Labels[localClusterLabel] == "true"
}
