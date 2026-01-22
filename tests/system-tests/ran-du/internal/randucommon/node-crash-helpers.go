package randucommon

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
)

const (
	// ConditionTypeReadyString constant for node Ready condition type.
	ConditionTypeReadyString = "Ready"
	// ConstantTrueString constant for True status.
	ConstantTrueString = "True"
)

// WaitForNodeToBeNotReady waits for the node to be not ready.
func WaitForNodeToBeNotReady(ctx SpecContext, nodeName string, pollingInterval, timeout time.Duration) {
	By(fmt.Sprintf("Waiting for node %q to get into NotReady state", nodeName))

	Eventually(func() bool {
		currentNode, err := nodes.Pull(APIClient, nodeName)
		if err != nil {
			klog.V(randuparams.RanDuLogLevel).Infof("Failed to pull node: %v", err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == ConditionTypeReadyString {
				if string(condition.Status) != ConstantTrueString {
					klog.V(randuparams.RanDuLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
					klog.V(randuparams.RanDuLogLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		klog.V(randuparams.RanDuLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)

		return false
	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Node %q hasn't reached NotReady state", nodeName)
}

// VerifyVmcoreDumpGenerated verifies that vmcore dump was generated in /var/crash.
func VerifyVmcoreDumpGenerated(ctx SpecContext, nodeName string) {
	By("Assert vmcore dump was generated")

	klog.V(randuparams.RanDuLogLevel).Infof("Checking if vmcore dump was generated on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "ls", "/var/crash"}

	Eventually(func() bool {
		coreDumps, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		klog.V(randuparams.RanDuLogLevel).Infof("Executing command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			klog.V(randuparams.RanDuLogLevel).Infof("Failed to execute command: %v", err)

			return false
		}

		klog.V(randuparams.RanDuLogLevel).Infof("\tGenerated VMCore dumps: %v", coreDumps)

		return len(strings.Fields(coreDumps)) >= 1
	}).WithContext(ctx).WithTimeout(5*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
		"error: vmcore dump was not generated on node %q", nodeName)
}

// CleanupVarCrashDirectory cleans up the /var/crash directory on the specified node.
func CleanupVarCrashDirectory(ctx SpecContext, nodeName string) {
	klog.V(randuparams.RanDuLogLevel).Infof("Cleaning up /var/crash directory on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "rm", "-rf", "/var/crash/*"}

	Eventually(func() bool {
		output, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		klog.V(randuparams.RanDuLogLevel).Infof("Executing cleanup command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			klog.V(randuparams.RanDuLogLevel).Infof("Failed to execute cleanup command: %v", err)

			return false
		}

		klog.V(randuparams.RanDuLogLevel).Infof("\tCleanup output: %v", output)

		return true
	}).WithContext(ctx).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		"error: failed to cleanup /var/crash directory on node %q", nodeName)

	klog.V(randuparams.RanDuLogLevel).Infof("Successfully cleaned up /var/crash directory on node %q", nodeName)
}
