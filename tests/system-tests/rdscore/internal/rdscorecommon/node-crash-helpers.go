package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// waitForNodeToBeNotReady waits for the node to be not ready.
func waitForNodeToBeNotReady(ctx SpecContext, nodeName string, pollingInterval, timeout time.Duration) {
	By(fmt.Sprintf("Waiting for node %q to get into NotReady state", nodeName))

	Eventually(func() bool {
		currentNode, err := nodes.Pull(APIClient, nodeName)
		if err != nil {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == rdscoreparams.ConditionTypeReadyString {
				if condition.Status != rdscoreparams.ConstantTrueString {
					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)

		return false
	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Node %q hasn't reached NotReady state", nodeName)
}

// verifyVmcoreDumpGenerated verifies that vmcore dump was generated in /var/crash.
//
//nolint:unused
func verifyVmcoreDumpGenerated(ctx SpecContext, nodeName string) {
	By("Assert vmcore dump was generated")

	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking if vmcore dump was generated on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "ls", "/var/crash"}

	Eventually(func() bool {
		coreDumps, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Executing command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute command: %v", err)

			return false
		}

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("\tGenerated VMCore dumps: %v", coreDumps)

		return len(strings.Fields(coreDumps)) >= 1
	}).WithContext(ctx).WithTimeout(5*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
		"error: vmcore dump was not generated on node %q", nodeName)
}

// cleanupVarCrashDirectory cleans up the /var/crash directory on the specified node.
//
//nolint:unused
func cleanupVarCrashDirectory(ctx SpecContext, nodeName string) {
	By(fmt.Sprintf("Cleaning up /var/crash directory on node %q", nodeName))

	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cleaning up /var/crash directory on node %q", nodeName)

	cmdToExec := []string{"chroot", "/rootfs", "rm", "-rf", "/var/crash/*"}

	Eventually(func() bool {
		output, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Executing cleanup command: %q on node %q",
			strings.Join(cmdToExec, " "), nodeName)

		if err != nil {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute cleanup command: %v", err)

			return false
		}

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("\tCleanup output: %v", output)

		return true
	}).WithContext(ctx).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		"error: failed to cleanup /var/crash directory on node %q", nodeName)

	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully cleaned up /var/crash directory on node %q", nodeName)
}

// DumpNodeStatus dumps comprehensive node status information for all nodes in the cluster.
// This function is typically called in AfterEach hooks when a test fails to provide
// debugging information about the cluster state.
//
//nolint:gocognit,funlen
func DumpNodeStatus(ctx SpecContext) {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node Status Dump - Test Failed")
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Check if the incoming context was already canceled
	if ctx.Err() != nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"WARNING: SpecContext was already canceled (%v), using fresh context for dump", ctx.Err())
	}

	var allNodes []*nodes.Builder

	// Create a fresh context to ensure dump works even if spec context is canceled
	dumpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(dumpCtx, 15*time.Second, 1*time.Minute, true,
		func(context.Context) (bool, error) {
			var listErr error

			allNodes, listErr = nodes.List(APIClient)
			if listErr != nil {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes (retrying...): %v", listErr)

				return false, nil
			}

			if len(allNodes) == 0 {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("No nodes found in the cluster (retrying...)")

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to retrieve node list after retries: %v", err)

		return
	}

	for _, node := range allNodes {
		if node.Object == nil {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping node with nil Object")

			continue
		}

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node: %s", node.Object.Name)
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		// Dump Spec Information
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Spec Information:")
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Unschedulable: %v", node.Object.Spec.Unschedulable)

		if len(node.Object.Spec.Taints) > 0 {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Taints:")

			for _, taint := range node.Object.Spec.Taints {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - Key: %s, Value: %s, Effect: %s",
					taint.Key, taint.Value, taint.Effect)
			}
		} else {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Taints: <none>")
		}

		// Dump Status Information
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Status Information:")

		// Dump Allocatable Resources
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Allocatable Resources:")

		if len(node.Object.Status.Allocatable) > 0 {
			for resourceName, quantity := range node.Object.Status.Allocatable {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - %s: %s", resourceName, quantity.String())
			}
		} else {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <none>")
		}

		// Dump Capacity Resources
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Capacity Resources:")

		if len(node.Object.Status.Capacity) > 0 {
			for resourceName, quantity := range node.Object.Status.Capacity {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - %s: %s", resourceName, quantity.String())
			}
		} else {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <none>")
		}

		// Dump Conditions
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Conditions:")

		if len(node.Object.Status.Conditions) > 0 {
			for _, condition := range node.Object.Status.Conditions {
				glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - Type: %s, Status: %s, Reason: %s, Message: %s",
					condition.Type, condition.Status, condition.Reason, condition.Message)
			}
		} else {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <none>")
		}
	}

	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of Node Status Dump")
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}
