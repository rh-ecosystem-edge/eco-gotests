package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterProvisionTimeout returns the ClusterProvisioned wait budget for the given spoke node count.
func ClusterProvisionTimeout(nodeCount int) time.Duration {
	if nodeCount < 0 {
		nodeCount = 0
	}

	return tsparams.ClusterProvisionBaseTimeout +
		time.Duration(nodeCount)*tsparams.ClusterProvisionPerNodeTimeout
}

// ConfigurationAppliedTimeout returns the ConfigurationApplied wait budget for the given master count.
func ConfigurationAppliedTimeout(masterCount int) time.Duration {
	if masterCount < 0 {
		masterCount = 0
	}

	return tsparams.ConfigurationAppliedBaseTimeout +
		time.Duration(masterCount)*tsparams.ConfigurationAppliedPerMasterTimeout
}

// FormatPRWaitTimeoutError builds a timeout error that includes the last observed PR phase, details, and conditions.
func FormatPRWaitTimeoutError(
	prName string,
	expected metav1.Condition,
	timeout time.Duration,
	phase, details string,
	conditions []metav1.Condition,
) error {
	var messageBuilder strings.Builder

	fmt.Fprintf(&messageBuilder,
		"timed out after %s waiting for ProvisioningRequest %s condition type=%q status=%q reason=%q",
		timeout, prName, expected.Type, expected.Status, expected.Reason)

	fmt.Fprintf(&messageBuilder, "; provisioningPhase=%q provisioningDetails=%q", phase, details)

	if len(conditions) == 0 {
		messageBuilder.WriteString("; conditions=[]")
	} else {
		messageBuilder.WriteString("; conditions=[")

		for i, condition := range conditions {
			if i > 0 {
				messageBuilder.WriteString("; ")
			}

			fmt.Fprintf(&messageBuilder, "{type=%q status=%q reason=%q message=%q lastTransitionTime=%s}",
				condition.Type, condition.Status, condition.Reason, condition.Message,
				condition.LastTransitionTime.UTC().Format(time.RFC3339))
		}

		messageBuilder.WriteString("]")
	}

	return fmt.Errorf("%s", messageBuilder.String())
}

// conditionMatchesExpected reports whether observed matches the non-empty fields of expected.
func conditionMatchesExpected(observed, expected metav1.Condition) bool {
	if expected.Type != "" && observed.Type != expected.Type {
		return false
	}

	if expected.Status != "" && observed.Status != expected.Status {
		return false
	}

	if expected.Reason != "" && observed.Reason != expected.Reason {
		return false
	}

	if expected.Message != "" && !strings.Contains(observed.Message, expected.Message) {
		return false
	}

	return true
}

// prStatusSnapshot is a compact view of PR status fields used for change detection and errors.
type prStatusSnapshot struct {
	phase      string
	details    string
	conditions []metav1.Condition
}

func snapshotPRStatus(pr *oran.ProvisioningRequestBuilder) prStatusSnapshot {
	if pr == nil || pr.Object == nil {
		return prStatusSnapshot{}
	}

	status := pr.Object.Status

	return prStatusSnapshot{
		phase:      string(status.ProvisioningStatus.ProvisioningPhase),
		details:    status.ProvisioningStatus.ProvisioningDetails,
		conditions: append([]metav1.Condition(nil), status.Conditions...),
	}
}

func (s prStatusSnapshot) equal(other prStatusSnapshot) bool {
	if s.phase != other.phase || s.details != other.details {
		return false
	}

	if len(s.conditions) != len(other.conditions) {
		return false
	}

	for i := range s.conditions {
		left, right := s.conditions[i], other.conditions[i]
		if left.Type != right.Type || left.Status != right.Status || left.Reason != right.Reason ||
			left.Message != right.Message || !left.LastTransitionTime.Equal(&right.LastTransitionTime) {
			return false
		}
	}

	return true
}

func logPRStatusChange(prName string, snap prStatusSnapshot) {
	klog.V(tsparams.LogLevel).Infof(
		"ProvisioningRequest %s status changed: phase=%q details=%q",
		prName, snap.phase, snap.details)

	for _, condition := range snap.conditions {
		klog.V(tsparams.LogLevel).Infof(
			"ProvisioningRequest %s condition: type=%q status=%q reason=%q message=%q lastTransitionTime=%s",
			prName, condition.Type, condition.Status, condition.Reason, condition.Message,
			condition.LastTransitionTime.UTC().Format(time.RFC3339))
	}
}

// WaitForPRCondition polls until the ProvisioningRequest matches expected, logging only when status fields change.
// On timeout the error includes phase, details, and all conditions. onStatusChange is optional and invoked after each
// logged change (errors from onStatusChange are ignored).
//
// prBuilder must use a Kubernetes client that returns status.conditions (hub kube API). The O2IMS provisioning API
// client maps phase and details only — conditions are always empty there — so waits against it never succeed.
func WaitForPRCondition(
	prBuilder *oran.ProvisioningRequestBuilder,
	expected metav1.Condition,
	timeout time.Duration,
	onStatusChange func(*oran.ProvisioningRequestBuilder),
) error {
	if prBuilder == nil {
		return fmt.Errorf("cannot wait on nil ProvisioningRequestBuilder")
	}

	prName := prBuilder.Definition.Name
	klog.V(tsparams.LogLevel).Infof(
		"Waiting up to %s for ProvisioningRequest %s condition type=%q status=%q reason=%q",
		timeout, prName, expected.Type, expected.Status, expected.Reason)

	var lastSnap prStatusSnapshot

	haveSnap := false

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var getErr error

			prBuilder.Object, getErr = prBuilder.Get()
			if getErr != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to get ProvisioningRequest %s: %v", prName, getErr)

				return false, nil
			}

			prBuilder.Definition = prBuilder.Object
			snap := snapshotPRStatus(prBuilder)

			if !haveSnap || !snap.equal(lastSnap) {
				logPRStatusChange(prName, snap)

				if onStatusChange != nil {
					onStatusChange(prBuilder)
				}

				lastSnap = snap
				haveSnap = true
			}

			for _, condition := range snap.conditions {
				if conditionMatchesExpected(condition, expected) {
					return true, nil
				}
			}

			return false, nil
		})
	if err == nil {
		return nil
	}

	snap := lastSnap
	if prBuilder.Object != nil {
		snap = snapshotPRStatus(prBuilder)
	}

	timeoutErr := FormatPRWaitTimeoutError(prName, expected, timeout, snap.phase, snap.details, snap.conditions)

	if expected.Type == string(provisioningv1alpha1.PRconditionTypes.ConfigurationApplied) {
		if policyMsg := nonCompliantPolicyMessages(RANConfig.Spoke1Name); policyMsg != "" {
			timeoutErr = fmt.Errorf("%w; nonCompliantPolicies=%s", timeoutErr, policyMsg)
		}
	}

	return timeoutErr
}

// nonCompliantPolicyMessages returns a compact summary of NonCompliant policies in namespace for timeout errors.
func nonCompliantPolicyMessages(namespace string) string {
	if HubAPIClient == nil || namespace == "" {
		return ""
	}

	policies, err := ocm.ListPoliciesInAllNamespaces(
		HubAPIClient, runtimeclient.ListOptions{Namespace: namespace})
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to list policies in %s for timeout details: %v", namespace, err)

		return ""
	}

	var parts []string

	for _, policy := range policies {
		if policy.Definition.Status.ComplianceState != policiesv1.NonCompliant {
			continue
		}

		msg := string(policy.Definition.Status.ComplianceState)
		if details := policy.Definition.Status.Details; len(details) > 0 && len(details[0].History) > 0 {
			msg = details[0].History[0].Message
		}

		parts = append(parts, fmt.Sprintf("%s/%s: %s",
			policy.Definition.Namespace, policy.Definition.Name, msg))
	}

	return strings.Join(parts, " | ")
}

// LogAssistedInstallProgress logs Agent and AgentClusterInstall progress for namespace.
// Failures are logged and ignored.
func LogAssistedInstallProgress(apiClient *clients.Settings, namespace string) {
	if apiClient == nil || namespace == "" {
		return
	}

	aci, err := assisted.PullAgentClusterInstall(apiClient, namespace, namespace)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Assisted progress: AgentClusterInstall %s/%s not available: %v", namespace, namespace, err)
	} else if aci.Object != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Assisted progress: ACI %s/%s state=%q stateInfo=%q totalPercentage=%d",
			namespace, namespace,
			aci.Object.Status.DebugInfo.State, aci.Object.Status.DebugInfo.StateInfo,
			aci.Object.Status.Progress.TotalPercentage)
	}

	agents, err := assisted.ListAgents(apiClient, runtimeclient.InNamespace(namespace))
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Assisted progress: list Agents in %s: %v", namespace, err)

		return
	}

	for _, agent := range agents {
		if agent.Object == nil {
			continue
		}

		klog.V(tsparams.LogLevel).Infof(
			"Assisted progress: Agent %s/%s state=%q stateInfo=%q",
			agent.Object.Namespace, agent.Object.Name,
			agent.Object.Status.DebugInfo.State, agent.Object.Status.DebugInfo.StateInfo)
	}
}
