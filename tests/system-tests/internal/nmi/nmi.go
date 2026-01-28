package nmi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stmcginnis/gofish/redfish"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
)

// BMCCredentials holds the BMC authentication details for a node.
type BMCCredentials struct {
	BMCAddress string
	Username   string
	Password   string
}

// CleanupVarCrashDirectory cleans up the /var/crash directory on the specified node.
func CleanupVarCrashDirectory(
	ctx context.Context,
	nodeName string,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) error {
	klog.V(logLevel).Infof("Cleaning up /var/crash directory on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "sh", "-c", "rm -rf /var/crash/*"}

	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			output, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

			klog.V(logLevel).Infof("Executing cleanup command: %q on node %q",
				strings.Join(cmdToExec, " "), nodeName)

			if err != nil {
				klog.V(logLevel).Infof("Failed to execute cleanup command: %v", err)

				return false, nil
			}

			klog.V(logLevel).Infof("\tCleanup output: %v", output)

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to cleanup /var/crash directory on node %q: %w", nodeName, err)
	}

	klog.V(logLevel).Infof("Successfully cleaned up /var/crash directory on node %q", nodeName)

	return nil
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
	klog.V(logLevel).Infof("Triggering NMI via Redfish on %q", nodeName)
	klog.V(logLevel).Infof("Creating BMC client for node %s", nodeName)

	bmcClient := bmc.New(bmcCredentials.BMCAddress).
		WithRedfishUser(bmcCredentials.Username, bmcCredentials.Password).
		WithRedfishTimeout(6 * time.Minute)

	klog.V(logLevel).Infof("Sending NMI reset action to %q", nodeName)

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
	ctx context.Context,
	apiClient *clients.Settings,
	nodeName string,
	isSNO bool,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) error {
	klog.V(logLevel).Infof("Waiting for node %q to become unavailable (SNO: %t)", nodeName, isSNO)

	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
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

					return true, nil
				}

				return false, nil
			}

			currentNode, err := nodes.Pull(apiClient, nodeName)
			if err != nil {
				klog.V(logLevel).Infof("Failed to pull node %q: %v", nodeName, err)

				return false, nil
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status != corev1.ConditionTrue {
						klog.V(logLevel).Infof("Node %q is NotReady", nodeName)
						klog.V(logLevel).Infof("  Reason: %s", condition.Reason)

						return true, nil
					}
				}
			}

			klog.V(logLevel).Infof("Node %q is still Ready", nodeName)

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("node %q hasn't become unreachable: %w", nodeName, err)
	}

	return nil
}

// WaitForNodeToBecomeReady waits for the node to return to Ready state.
// It first verifies that the API server is reachable, then waits for the node to become Ready.
func WaitForNodeToBecomeReady(
	ctx context.Context,
	apiClient *clients.Settings,
	nodeName string,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) error {
	klog.V(logLevel).Infof("Waiting for node %q to return to Ready state", nodeName)

	startTime := time.Now()

	klog.V(logLevel).Infof("Waiting for API server to become reachable")

	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, apiErr := apiClient.K8sClient.Discovery().ServerVersion()
			if apiErr != nil {
				klog.V(logLevel).Infof("API server not yet reachable: %v", apiErr)

				return false, nil
			}

			klog.V(logLevel).Infof("API server is reachable")

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("API server didn't become reachable: %w", err)
	}

	remainingTimeout := timeout - time.Since(startTime)

	if remainingTimeout <= 0 {
		return fmt.Errorf("timeout exceeded while waiting for API server to become reachable")
	}

	currentNode, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to pull node %q: %w", nodeName, err)
	}

	err = currentNode.WaitUntilReady(remainingTimeout)
	if err != nil {
		return fmt.Errorf("node %q hasn't reached Ready state: %w", nodeName, err)
	}

	klog.V(logLevel).Infof("Node %q is Ready", nodeName)

	return nil
}

// VerifyVmcoreDumpGenerated verifies that vmcore dump was generated in /var/crash.
func VerifyVmcoreDumpGenerated(
	ctx context.Context,
	nodeName string,
	logLevel klog.Level,
	pollingInterval,
	timeout time.Duration,
) error {
	klog.V(logLevel).Infof("Checking if vmcore dump was generated on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "ls", "/var/crash"}

	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			coreDumps, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

			klog.V(logLevel).Infof("Executing command: %q on node %q",
				strings.Join(cmdToExec, " "), nodeName)

			if err != nil {
				klog.V(logLevel).Infof("Failed to execute command: %v", err)

				return false, nil
			}

			klog.V(logLevel).Infof("\tGenerated VMCore dumps: %v", coreDumps)

			return len(strings.Fields(coreDumps)) >= 1, nil
		})
	if err != nil {
		return fmt.Errorf("vmcore dump was not generated on node %q: %w", nodeName, err)
	}

	return nil
}
