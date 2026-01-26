package nmi

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
)

const (
	// ConditionTypeReadyString constant for node Ready condition type.
	ConditionTypeReadyString = "Ready"
	// ConstantTrueString constant for True status.
	ConstantTrueString = "True"
)

// BMCCredentials holds the BMC authentication details for a node.
type BMCCredentials struct {
	BMCAddress string
	Username   string
	Password   string
}

// TriggerNMIViaRedfish triggers an NMI (Non-Maskable Interrupt) on a node via Redfish BMC interface.
// This will cause a kernel crash if kdump is configured, generating a vmcore dump.
func TriggerNMIViaRedfish(
	ctx context.Context,
	nodeName string,
	bmcCredentials BMCCredentials,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) error {
	By(fmt.Sprintf("Trigger NMI via Redfish on node %q", nodeName))

	klog.V(logLevel).Infof("Triggering NMI via Redfish on %q", nodeName)
	klog.V(logLevel).Infof("Creating BMC client for node %s", nodeName)

	bmcClient := bmc.New(bmcCredentials.BMCAddress).
		WithRedfishUser(bmcCredentials.Username, bmcCredentials.Password).
		WithRedfishTimeout(6 * time.Minute)

	By(fmt.Sprintf("Sending NMI reset action to %q", nodeName))

	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			if err := bmcClient.SystemResetAction(redfish.NmiResetType); err != nil {
				klog.V(logLevel).Infof("Failed to trigger NMI on %s -> %v", nodeName, err)

				return false, nil
			}

			klog.V(logLevel).Infof("Successfully triggered NMI on %s", nodeName)

			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to trigger NMI on node %s: %w", nodeName, err)
	}

	return nil
}

// WaitForNodeToBecomeUnavailable waits for the node to become unavailable.
// The behavior differs based on the deployment type:
//   - SNO (Single Node OpenShift): Once the NMI interrupt is triggered, the node will restart,
//     causing the API to become unreachable (client connection lost).
//   - Multi-node deployment: The node will change its status to NotReady, but the API will
//     continue to respond and the node status change will be observable.
func WaitForNodeToBecomeUnavailable(
	ctx SpecContext,
	apiClient *clients.Settings,
	nodeName string,
	isSNO bool,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) {
	By(fmt.Sprintf("Waiting for node %q to become unavailable (SNO: %t)", nodeName, isSNO))

	Eventually(func() bool {
		_, apiErr := apiClient.K8sClient.Discovery().ServerVersion()
		if apiErr != nil {
			klog.V(logLevel).Infof("API server check failed: %v", apiErr)

			if isSNO && (strings.Contains(apiErr.Error(), "client connection lost") ||
				strings.Contains(apiErr.Error(), "connection refused") ||
				strings.Contains(apiErr.Error(), "no route to host") ||
				strings.Contains(apiErr.Error(), "i/o timeout") ||
				strings.Contains(apiErr.Error(), "net/http: request canceled") ||
				strings.Contains(apiErr.Error(), "context deadline exceeded")) {
				klog.V(logLevel).Infof("API unreachable - node has become unavailable (SNO)")

				return true
			}

			return false
		}

		currentNode, err := nodes.Pull(apiClient, nodeName)
		if err != nil {
			klog.V(logLevel).Infof("Failed to pull node %q: %v", nodeName, err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == ConditionTypeReadyString {
				if string(condition.Status) != ConstantTrueString {
					klog.V(logLevel).Infof("Node %q is NotReady", nodeName)
					klog.V(logLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		klog.V(logLevel).Infof("Node %q is still Ready", nodeName)

		return false

	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Node %q hasn't become unreachable", nodeName)
}

// WaitForNodeToBecomeReady waits for the node to return to Ready state.
// The behavior differs based on the deployment type:
//   - SNO (Single Node OpenShift): Before checking node status, it first verifies that the
//     API server is reachable again after the node restart.
//   - Multi-node deployment: Directly checks the node status since the API remains available.
func WaitForNodeToBecomeReady(
	ctx SpecContext,
	apiClient *clients.Settings,
	nodeName string,
	isSNO bool,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) {
	By(fmt.Sprintf("Waiting for node %q to return to Ready state (SNO: %t)", nodeName, isSNO))

	Eventually(func() bool {
		// For SNO deployments, first check if the API server is reachable again
		if isSNO {
			_, apiErr := apiClient.K8sClient.Discovery().ServerVersion()
			if apiErr != nil {
				klog.V(logLevel).Infof("API server not yet reachable: %v", apiErr)

				return false
			}

			klog.V(logLevel).Infof("API server is reachable again")
		}

		currentNode, err := nodes.Pull(apiClient, nodeName)
		if err != nil {
			klog.V(logLevel).Infof("Failed to pull node %q: %v", nodeName, err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == ConditionTypeReadyString {
				if string(condition.Status) == ConstantTrueString {
					klog.V(logLevel).Infof("Node %q is Ready", nodeName)
					klog.V(logLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		klog.V(logLevel).Infof("Node %q is not yet Ready", nodeName)

		return false
	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Node %q hasn't reached Ready state", nodeName)
}

// VerifyVmcoreDumpGenerated verifies that vmcore dump was generated in /var/crash.
func VerifyVmcoreDumpGenerated(ctx SpecContext, nodeName string, logLevel klog.Level) {
	By("Assert vmcore dump was generated")

	klog.V(logLevel).Infof("Checking if vmcore dump was generated on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "ls", "/var/crash"}

	Eventually(func() bool {
		coreDumps, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		klog.V(logLevel).Infof("Executing command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			klog.V(logLevel).Infof("Failed to execute command: %v", err)

			return false
		}

		klog.V(logLevel).Infof("\tGenerated VMCore dumps: %v", coreDumps)

		return len(strings.Fields(coreDumps)) >= 1
	}).WithContext(ctx).WithTimeout(5*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
		"error: vmcore dump was not generated on node %q", nodeName)
}

// CleanupVarCrashDirectory cleans up the /var/crash directory on the specified node.
func CleanupVarCrashDirectory(ctx SpecContext, nodeName string, logLevel klog.Level) {
	By(fmt.Sprintf("Cleaning up /var/crash directory on node %q", nodeName))

	klog.V(logLevel).Infof("Cleaning up /var/crash directory on node %q", nodeName)

	// Use sh -c to enable shell glob expansion for the wildcard pattern
	cmdToExec := []string{"chroot", "/rootfs", "sh", "-c", "rm -rf /var/crash/*"}

	Eventually(func() bool {
		output, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		klog.V(logLevel).Infof("Executing cleanup command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			klog.V(logLevel).Infof("Failed to execute cleanup command: %v", err)

			return false
		}

		klog.V(logLevel).Infof("\tCleanup output: %v", output)

		return true
	}).WithContext(ctx).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		"error: failed to cleanup /var/crash directory on node %q", nodeName)

	klog.V(logLevel).Infof("Successfully cleaned up /var/crash directory on node %q", nodeName)
}
