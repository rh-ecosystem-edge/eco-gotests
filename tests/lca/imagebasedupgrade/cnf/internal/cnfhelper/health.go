package cnfhelper

import (
	"context"
	"fmt"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/certificate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oadp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	oadpv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/oadp/api/v1alpha1"
	olmv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/velero"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultHealthTimeout is the overall timeout used by tests when claiming
	// the spoke is healthy, including DefaultHealthStablePeriod.
	DefaultHealthTimeout = 15 * time.Minute
	// DefaultHealthStablePeriod is how long Check must stay healthy before
	// WaitUntilHealthy succeeds. An unhealthy snapshot restarts this window.
	DefaultHealthStablePeriod = 3 * time.Minute
	// RecoverHealthTimeout is the overall timeout Recover uses after abort,
	// reboot, or cleanup, including DefaultHealthStablePeriod.
	RecoverHealthTimeout = 45 * time.Minute

	healthPollInterval     = 15 * time.Second
	sriovSyncSucceeded     = "Succeeded"
	oadpNamespace          = "openshift-adp"
	nodeRoleControlPlane   = "node-role.kubernetes.io/control-plane"
	nodeRoleMaster         = "node-role.kubernetes.io/master"
	nodeRoleWorker         = "node-role.kubernetes.io/worker"
	lifecycleAgentCSVToken = "lifecycle-agent"
)

// CheckName identifies a single spoke health check.
type CheckName string

const (
	checkClusterOperators CheckName = "clusterOperators"
	checkMachineConfig    CheckName = "machineConfigPools"
	checkSriov            CheckName = "sriovNetworkNodeState"
	checkDPA              CheckName = "dataProtectionApplication"
	checkBackupStorage    CheckName = "backupStorageLocation"
	checkNode             CheckName = "node"
	checkCSV              CheckName = "clusterServiceVersion"
	checkCSR              CheckName = "certificateSigningRequest"
)

// CheckResult is the outcome of one health check.
type CheckResult struct {
	Name    CheckName
	Healthy bool
	Skipped bool
	Message string
}

// Report is a point-in-time snapshot of spoke health.
type Report struct {
	Results []CheckResult
}

// Healthy reports whether every check passed or was skipped.
func (report Report) Healthy() bool {
	for _, result := range report.Results {
		if !result.Healthy {
			return false
		}
	}

	return true
}

// FailureSummary returns a joined description of failed checks.
func (report Report) FailureSummary() string {
	var failures []string

	for _, result := range report.Results {
		if result.Healthy {
			continue
		}

		failures = append(failures, fmt.Sprintf("%s: %s", result.Name, result.Message))
	}

	return strings.Join(failures, "; ")
}

// result returns the named check from the report, if present.
func (report Report) result(name CheckName) (CheckResult, bool) {
	for _, result := range report.Results {
		if result.Name == name {
			return result, true
		}
	}

	return CheckResult{}, false
}

// unhealthy reports whether the named check ran and failed.
func (report Report) unhealthy(name CheckName) bool {
	result, found := report.result(name)

	return found && !result.Healthy
}

// skippedResult is a passing check that was not applicable on this spoke.
func skippedResult(name CheckName, message string) CheckResult {
	return CheckResult{Name: name, Healthy: true, Skipped: true, Message: message}
}

// failedResult is a failing check with the given message.
func failedResult(name CheckName, message string) CheckResult {
	return CheckResult{Name: name, Message: message}
}

// passedResult is a successful check with the given message.
func passedResult(name CheckName, message string) CheckResult {
	return CheckResult{Name: name, Healthy: true, Message: message}
}

// Check snapshots spoke health without waiting. API errors are recorded as failed
// checks so WaitUntilHealthy can retry while the spoke API is unreachable.
func Check() Report {
	apiClient := cnfinittools.TargetSNOAPIClient
	if apiClient == nil {
		return Report{Results: []CheckResult{
			failedResult(checkClusterOperators, "target SNO API client is nil"),
		}}
	}

	dpaResult := checkDPAReconciled(apiClient)
	backupResult := skippedResult(checkBackupStorage, "skipped: no reconciled DataProtectionApplication")

	if dpaResult.Healthy && !dpaResult.Skipped {
		backupResult = checkBackupStorageAvailable(apiClient)
	}

	return Report{Results: []CheckResult{
		checkClusterOperatorsReady(apiClient),
		checkMachineConfigPoolsReady(apiClient),
		checkSriovReady(apiClient),
		dpaResult,
		backupResult,
		checkNodeReady(apiClient),
		checkCSVsReady(apiClient),
		checkCSRsApproved(apiClient),
	}}
}

// WaitUntilHealthy polls Check until every check has stayed passing for
// DefaultHealthStablePeriod, or timeout is reached. An unhealthy snapshot
// restarts the stability window.
func WaitUntilHealthy(timeout time.Duration) error {
	var last Report

	var healthySince time.Time

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), healthPollInterval, timeout, true,
		func(_ context.Context) (bool, error) {
			last = Check()

			return healthStayedStable(last, &healthySince, time.Now()), nil
		})
	if pollErr != nil {
		return waitUntilHealthyTimeoutError(timeout, last, healthySince)
	}

	return nil
}

// healthStayedStable updates healthySince from report at now. It returns true
// when report has been healthy for DefaultHealthStablePeriod. An unhealthy
// snapshot restarts the window.
func healthStayedStable(report Report, healthySince *time.Time, now time.Time) bool {
	if !report.Healthy() {
		if !healthySince.IsZero() {
			klog.V(cnfparams.CNFLogLevel).Infof(
				"spoke health flipped after %s, restarting %s stability window: %s",
				now.Sub(*healthySince).Truncate(time.Second),
				DefaultHealthStablePeriod, report.FailureSummary())
		} else {
			klog.V(cnfparams.CNFLogLevel).Infof("spoke health not ready: %s", report.FailureSummary())
		}

		*healthySince = time.Time{}

		return false
	}

	if healthySince.IsZero() {
		*healthySince = now

		klog.V(cnfparams.CNFLogLevel).Infof(
			"spoke health is green, waiting %s for stability", DefaultHealthStablePeriod)

		return false
	}

	return now.Sub(*healthySince) >= DefaultHealthStablePeriod
}

// waitUntilHealthyTimeoutError describes a WaitUntilHealthy failure after timeout.
func waitUntilHealthyTimeoutError(timeout time.Duration, last Report, healthySince time.Time) error {
	if last.Healthy() && !healthySince.IsZero() {
		return fmt.Errorf("spoke did not stay healthy for %s within %s (last healthy streak: %s)",
			DefaultHealthStablePeriod, timeout, time.Since(healthySince).Truncate(time.Second))
	}

	return fmt.Errorf("spoke did not stay healthy for %s within %s: %s",
		DefaultHealthStablePeriod, timeout, last.FailureSummary())
}

// checkClusterOperatorsReady reports whether every ClusterOperator is available
// and neither progressing nor degraded.
func checkClusterOperatorsReady(apiClient *clients.Settings) CheckResult {
	operators, err := clusteroperator.List(apiClient)
	if err != nil {
		return failedResult(checkClusterOperators, err.Error())
	}

	var notReady []string

	for _, operator := range operators {
		if !operator.IsAvailable() || operator.IsProgressing() || operator.IsDegraded() {
			notReady = append(notReady, operator.Object.Name)
		}
	}

	if len(notReady) > 0 {
		return failedResult(checkClusterOperators,
			fmt.Sprintf("one or more ClusterOperators not yet ready: %s", strings.Join(notReady, ", ")))
	}

	return passedResult(checkClusterOperators, "all cluster operators are ready")
}

// checkMachineConfigPoolsReady reports whether every MachineConfigPool has all
// machines ready.
func checkMachineConfigPoolsReady(apiClient *clients.Settings) CheckResult {
	pools, err := mco.ListMCP(apiClient)
	if err != nil {
		return failedResult(checkMachineConfig, err.Error())
	}

	var notReady []string

	for _, pool := range pools {
		if pool.Object.Status.MachineCount != pool.Object.Status.ReadyMachineCount {
			notReady = append(notReady, pool.Object.Name)
		}
	}

	if len(notReady) > 0 {
		return failedResult(checkMachineConfig,
			fmt.Sprintf("one or more MachineConfigPools not yet ready: %s", strings.Join(notReady, ", ")))
	}

	return passedResult(checkMachineConfig, "all machine config pools are ready")
}

// checkSriovReady reports whether every SriovNetworkNodeState has SyncStatus
// Succeeded. A missing CRD is skipped; an empty list is a failure.
func checkSriovReady(apiClient *clients.Settings) CheckResult {
	if cnfinittools.CNFConfig == nil {
		return failedResult(checkSriov, "CNF config is nil")
	}

	namespace := cnfinittools.CNFConfig.SriovOperatorNamespace

	nodeStates, err := sriov.ListNetworkNodeState(
		apiClient, namespace, client.ListOptions{Namespace: namespace})
	if err != nil {
		if isMissingCRD(err) {
			return skippedResult(checkSriov, "skipped: SriovNetworkNodeState CRD is not present")
		}

		return failedResult(checkSriov, err.Error())
	}

	if len(nodeStates) == 0 {
		return failedResult(checkSriov, "no SriovNetworkNodeStates present")
	}

	var notReady []string

	for _, nodeState := range nodeStates {
		if nodeState.Objects.Status.SyncStatus != sriovSyncSucceeded {
			notReady = append(notReady,
				fmt.Sprintf("%s syncStatus=%s", nodeState.Objects.Name, nodeState.Objects.Status.SyncStatus))
		}
	}

	if len(notReady) > 0 {
		return failedResult(checkSriov,
			fmt.Sprintf("sriovNetworkNodeState not ready: %s", strings.Join(notReady, ", ")))
	}

	return passedResult(checkSriov, "all SriovNetworkNodeStates are Succeeded")
}

// checkDPAReconciled reports whether at least one DataProtectionApplication is
// reconciled. A missing CRD or empty list is skipped.
func checkDPAReconciled(apiClient *clients.Settings) CheckResult {
	dpaList, err := oadp.ListDataProtectionApplication(apiClient, oadpNamespace)
	if err != nil {
		if isMissingCRD(err) {
			return skippedResult(checkDPA, "skipped: DataProtectionApplication CRD is not present")
		}

		return failedResult(checkDPA, err.Error())
	}

	if len(dpaList) == 0 {
		return skippedResult(checkDPA, "skipped: no DataProtectionApplication present")
	}

	for _, dpaBuilder := range dpaList {
		if isDPAReconciled(dpaBuilder.Object) {
			return passedResult(checkDPA,
				fmt.Sprintf("DataProtectionApplication %s is reconciled", dpaBuilder.Object.Name))
		}
	}

	return failedResult(checkDPA, "dataProtectionApplication not reconciled")
}

// isDPAReconciled reports whether the DPA Reconciled condition is True.
func isDPAReconciled(dpaObject *oadpv1alpha1.DataProtectionApplication) bool {
	if dpaObject == nil {
		return false
	}

	for _, condition := range dpaObject.Status.Conditions {
		if condition.Type == oadpv1alpha1.ConditionReconciled && condition.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// checkBackupStorageAvailable reports whether every BackupStorageLocation is
// Available. A missing CRD or empty list is a failure because a reconciled DPA
// should have created the locations.
func checkBackupStorageAvailable(apiClient *clients.Settings) CheckResult {
	locations, err := velero.ListBackupStorageLocationBuilder(apiClient, oadpNamespace)
	if err != nil {
		if isMissingCRD(err) {
			return failedResult(checkBackupStorage, "backupsStorageLocations not yet created")
		}

		return failedResult(checkBackupStorage, err.Error())
	}

	if len(locations) == 0 {
		return failedResult(checkBackupStorage, "backupsStorageLocations not yet created")
	}

	for _, location := range locations {
		if location.Object.Status.Phase != velerov1.BackupStorageLocationPhaseAvailable {
			return failedResult(checkBackupStorage,
				fmt.Sprintf("backupStorageLocation %s not available", location.Object.Name))
		}
	}

	return passedResult(checkBackupStorage, "all BackupStorageLocations are available")
}

// checkNodeReady reports SNO node Ready, network, and role-label health.
// Non-SNO topologies are skipped.
func checkNodeReady(apiClient *clients.Settings) CheckResult {
	infra, err := infrastructure.Pull(apiClient)
	if err != nil {
		return failedResult(checkNode, err.Error())
	}

	if infra.Object.Status.InfrastructureTopology != configv1.SingleReplicaTopologyMode {
		return skippedResult(checkNode,
			fmt.Sprintf("skipped: InfrastructureTopology is %s", infra.Object.Status.InfrastructureTopology))
	}

	nodeList, err := nodes.List(apiClient)
	if err != nil {
		return failedResult(checkNode, err.Error())
	}

	for _, nodeBuilder := range nodeList {
		if result := snoNodeCheck(nodeBuilder); !result.Healthy {
			return result
		}
	}

	return passedResult(checkNode, "node is ready")
}

// snoNodeCheck reports whether one node is Ready, networked, and labeled for
// control-plane, master, and worker roles.
func snoNodeCheck(nodeBuilder *nodes.Builder) CheckResult {
	ready, err := nodeBuilder.IsReady()
	if err != nil {
		return failedResult(checkNode, err.Error())
	}

	if !ready {
		return failedResult(checkNode, fmt.Sprintf("node is not yet ready: %s", nodeBuilder.Object.Name))
	}

	if nodeConditionTrue(nodeBuilder.Object.Status.Conditions, corev1.NodeNetworkUnavailable) {
		return failedResult(checkNode, fmt.Sprintf("node network unavailable: %s", nodeBuilder.Object.Name))
	}

	labels := nodeBuilder.Object.GetLabels()
	required := []string{nodeRoleControlPlane, nodeRoleMaster, nodeRoleWorker}

	for _, label := range required {
		if _, found := labels[label]; !found {
			return failedResult(checkNode,
				fmt.Sprintf("node missing %s label: %s", label, nodeBuilder.Object.Name))
		}
	}

	return passedResult(checkNode, "node is ready")
}

// nodeConditionTrue reports whether the named node condition is True.
func nodeConditionTrue(conditions []corev1.NodeCondition, conditionType corev1.NodeConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// checkCSVsReady reports whether every ClusterServiceVersion except
// lifecycle-agent is Succeeded. LCA is skipped because IBU stages restart it.
func checkCSVsReady(apiClient *clients.Settings) CheckResult {
	csvList, err := olm.ListClusterServiceVersionInAllNamespaces(apiClient)
	if err != nil {
		return failedResult(checkCSV, err.Error())
	}

	var notReady []string

	for _, csvBuilder := range csvList {
		if strings.Contains(csvBuilder.Object.Name, lifecycleAgentCSVToken) {
			continue
		}

		if csvBuilder.Object.Status.Phase != olmv1alpha1.CSVPhaseSucceeded {
			notReady = append(notReady, csvBuilder.Object.Name)
		}
	}

	if len(notReady) > 0 {
		return failedResult(checkCSV,
			fmt.Sprintf("one or more ClusterServiceVersions not yet ready: %s", strings.Join(notReady, ", ")))
	}

	return passedResult(checkCSV, "all CSVs are ready")
}

// checkCSRsApproved reports whether every CertificateSigningRequest is Approved.
func checkCSRsApproved(apiClient *clients.Settings) CheckResult {
	csrList, err := certificate.ListSigningRequests(apiClient)
	if err != nil {
		return failedResult(checkCSR, err.Error())
	}

	for _, csrBuilder := range csrList {
		if csrBuilder.Object == nil || !csrApproved(*csrBuilder.Object) {
			name := "<nil>"
			if csrBuilder.Object != nil {
				name = csrBuilder.Object.Name
			}

			return failedResult(checkCSR,
				fmt.Sprintf("certificateSigningRequest (csr) %s not yet approved", name))
		}
	}

	return passedResult(checkCSR, "all CSRs are approved")
}

// csrApproved reports whether the CSR has an Approved=True condition.
func csrApproved(csrObject certificatesv1.CertificateSigningRequest) bool {
	for _, condition := range csrObject.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// isMissingCRD reports whether err is a missing-kind or unregistered-type API error.
func isMissingCRD(err error) bool {
	if err == nil {
		return false
	}

	if meta.IsNoMatchError(err) || k8sruntime.IsNotRegisteredError(err) {
		return true
	}

	message := err.Error()

	return strings.Contains(message, "no matches for kind") ||
		strings.Contains(message, "no kind is registered") ||
		strings.Contains(message, "could not find the requested resource")
}
