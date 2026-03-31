package rdscorecommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// VerifyNMStateNamespaceExists asserts namespace for NMState operator exists.
func VerifyNMStateNamespaceExists(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify namespace %q exists",
		RDSCoreConfig.NMStateOperatorNamespace)

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := namespace.Pull(APIClient, RDSCoreConfig.NMStateOperatorNamespace)
			if pullErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to pull in namespace %q - %v",
					RDSCoreConfig.NMStateOperatorNamespace, pullErr)

				return false, pullErr
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull %q namespace", RDSCoreConfig.NMStateOperatorNamespace))
}

// VerifyNMStateInstanceExists assert that NMState instance exists.
func VerifyNMStateInstanceExists(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify NMState instance exists")

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := nmstate.PullNMstate(APIClient, rdscoreparams.NMStateInstanceName)
			if pullErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to pull in NMState instance %q - %v",
					rdscoreparams.NMStateInstanceName, pullErr)

				return false, pullErr
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull in NMState instance %q", rdscoreparams.NMStateInstanceName))
}

// dumpNNCPDetails dumps comprehensive diagnostic information for a failed NNCP.
// It returns a formatted string containing policy status, generation, conditions, and desired state configuration.
func dumpNNCPDetails(nncp *nmstate.PolicyBuilder) string {
	var details strings.Builder

	fmt.Fprintf(&details, "\n=== NNCP: %s ===\n", nncp.Definition.Name)
	fmt.Fprintf(&details, "  Namespace: %s\n", nncp.Definition.Namespace)
	fmt.Fprintf(&details, "  Generation: %d\n", nncp.Object.Generation)
	fmt.Fprintf(&details, "  Created: %s\n", nncp.Object.CreationTimestamp.Format(time.RFC3339))

	// Node selector
	if len(nncp.Definition.Spec.NodeSelector) > 0 {
		fmt.Fprintf(&details, "  NodeSelector: %v\n", nncp.Definition.Spec.NodeSelector)
	}

	// Max unavailable
	if nncp.Definition.Spec.MaxUnavailable != nil {
		fmt.Fprintf(&details, "  MaxUnavailable: %v\n", nncp.Definition.Spec.MaxUnavailable)
	}

	// Conditions with full details
	details.WriteString("  Conditions:\n")

	if len(nncp.Object.Status.Conditions) == 0 {
		details.WriteString("    (No conditions reported)\n")
	} else {
		for _, condition := range nncp.Object.Status.Conditions {
			fmt.Fprintf(&details, "    - Type: %s\n", condition.Type)
			fmt.Fprintf(&details, "      Status: %s\n", condition.Status)

			if condition.Reason != "" {
				fmt.Fprintf(&details, "      Reason: %s\n", condition.Reason)
			}

			if condition.Message != "" {
				fmt.Fprintf(&details, "      Message: %s\n", condition.Message)
			}

			if !condition.LastTransitionTime.IsZero() {
				fmt.Fprintf(&details, "      LastTransitionTime: %s\n",
					condition.LastTransitionTime.Format(time.RFC3339))
			}

			if !condition.LastHeartbeatTime.IsZero() {
				fmt.Fprintf(&details, "      LastHeartbeatTime: %s\n",
					condition.LastHeartbeatTime.Format(time.RFC3339))
			}
		}
	}

	// Dump desired state configuration (helpful for understanding what was being applied)
	if len(nncp.Definition.Spec.DesiredState.Raw) > 0 {
		details.WriteString("  DesiredState:\n")

		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, nncp.Definition.Spec.DesiredState.Raw, "    ", "  "); err == nil {
			fmt.Fprintf(&details, "%s\n", prettyJSON.String())
		} else {
			// Fallback to raw if JSON formatting fails
			fmt.Fprintf(&details, "    (Raw) %s\n", string(nncp.Definition.Spec.DesiredState.Raw))
		}
	}

	return details.String()
}

// buildFailureReport builds a comprehensive failure report for NNCP validation failures.
// It aggregates diagnostics for all non-available, degraded, and progressing NNCPs.
func buildFailureReport(
	nncps []*nmstate.PolicyBuilder,
	nonAvailableNNCP map[string]string,
	degradedNNCP map[string]string,
	progressingNNCP map[string]string) string {
	var report strings.Builder

	if len(nonAvailableNNCP) > 0 {
		report.WriteString("\n\n========================================\n")
		fmt.Fprintf(&report, "NON-AVAILABLE NNCPs: %d\n", len(nonAvailableNNCP))
		report.WriteString("========================================\n")

		for policyName, message := range nonAvailableNNCP {
			fmt.Fprintf(&report, "\nPolicy: %s\n", policyName)
			fmt.Fprintf(&report, "Condition Message: %s\n", message)

			// Find and dump the policy details
			for _, nncp := range nncps {
				if nncp.Definition.Name == policyName {
					report.WriteString(dumpNNCPDetails(nncp))

					break
				}
			}
		}
	}

	if len(degradedNNCP) > 0 {
		report.WriteString("\n\n========================================\n")
		fmt.Fprintf(&report, "DEGRADED NNCPs: %d\n", len(degradedNNCP))
		report.WriteString("========================================\n")

		for policyName, message := range degradedNNCP {
			fmt.Fprintf(&report, "\nPolicy: %s\n", policyName)
			fmt.Fprintf(&report, "Condition Message: %s\n", message)

			for _, nncp := range nncps {
				if nncp.Definition.Name == policyName {
					report.WriteString(dumpNNCPDetails(nncp))

					break
				}
			}
		}
	}

	if len(progressingNNCP) > 0 {
		report.WriteString("\n\n========================================\n")
		fmt.Fprintf(&report, "PROGRESSING NNCPs: %d\n", len(progressingNNCP))
		report.WriteString("========================================\n")

		for policyName, message := range progressingNNCP {
			fmt.Fprintf(&report, "\nPolicy: %s\n", policyName)
			fmt.Fprintf(&report, "Condition Message: %s\n", message)

			for _, nncp := range nncps {
				if nncp.Definition.Name == policyName {
					report.WriteString(dumpNNCPDetails(nncp))

					break
				}
			}
		}
	}

	return report.String()
}

// VerifyAllNNCPsAreOK assert all available NNCPs are Available, not progressing and not degraded.
func VerifyAllNNCPsAreOK(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify NodeNetworkConfigurationPolicies are Available")

	const ConditionTypeTrue = "True"

	nncps, err := nmstate.ListPolicy(APIClient, goclient.ListOptions{})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to list NodeNetworkConfigurationPolicies: %v", err))
	Expect(len(nncps)).ToNot(Equal(0), "0 NodeNetworkConfigurationPolicies found")

	nonAvailableNNCP := make(map[string]string)
	progressingNNCP := make(map[string]string)
	degradedNNCP := make(map[string]string)

	for _, nncp := range nncps {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("\t Processing %s NodeNetworkConfigurationPolicy", nncp.Definition.Name))

		for _, condition := range nncp.Object.Status.Conditions {
			//nolint:nolintlint
			switch condition.Type { //nolint:exhaustive
			//nolint:goconst
			case "Available":
				if condition.Status != ConditionTypeTrue {
					nonAvailableNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is not Available: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Available: %s\n", nncp.Definition.Name, condition.Message))
				}
			case "Degraded":
				if condition.Status == ConditionTypeTrue {
					degradedNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Degraded: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Not-Degraded\n", nncp.Definition.Name))
				}
			case "Progressing":
				if condition.Status == ConditionTypeTrue {
					progressingNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Progressing: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Not-Progressing\n", nncp.Definition.Name))
				}
			}
		}
	}

	// Build comprehensive failure report if there are any issues
	var failureReport string

	hasFailures := len(nonAvailableNNCP) > 0 || len(degradedNNCP) > 0 || len(progressingNNCP) > 0

	if hasFailures {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			"NNCP validation failures detected - generating detailed report")

		failureReport = buildFailureReport(nncps, nonAvailableNNCP, degradedNNCP, progressingNNCP)

		// Log the full report
		klog.Errorf("NNCP Validation Failed:\n%s", failureReport)
	}

	// Enhanced assertions with detailed reporting
	Expect(len(nonAvailableNNCP)).To(Equal(0),
		fmt.Sprintf("There are %d NonAvailable NodeNetworkConfigurationPolicies. "+
			"See detailed report above in logs.", len(nonAvailableNNCP)))

	Expect(len(degradedNNCP)).To(Equal(0),
		fmt.Sprintf("There are %d Degraded NodeNetworkConfigurationPolicies. "+
			"See detailed report above in logs.", len(degradedNNCP)))

	// Validate progressing policies (previously this incorrectly checked nonAvailableNNCP)
	Expect(len(progressingNNCP)).To(Equal(0),
		fmt.Sprintf("There are %d Progressing NodeNetworkConfigurationPolicies. "+
			"See detailed report above in logs.", len(progressingNNCP)))
} // func VerifyNNCP (ctx SpecContext)

// VerifyNMStateSuite container that contains tests for NMState verification.
func VerifyNMStateSuite() {
	Describe(
		"NMState validation",
		Label(rdscoreparams.LabelValidateNMState), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", RDSCoreConfig.NMStateOperatorNamespace),
				Label("nmstate-ns"), VerifyNMStateNamespaceExists)

			It("Verifies NMState instance exists",
				Label("nmstate-instance"), reportxml.ID("67027"), VerifyNMStateInstanceExists)

			It("Verifies all NodeNetworkConfigurationPolicies are Available",
				Label("nmstate-nncp"), reportxml.ID("71846"), VerifyAllNNCPsAreOK)
		})
}
