package rdscorecommon

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/reboot"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// readSysRqValue reads and parses the kernel.sysrq value from the specified node.
func readSysRqValue(ctx SpecContext, nodeName string) int {
	var currentValue int

	checkCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "cat /proc/sys/kernel/sysrq"}

	Eventually(func() bool {
		output, err := remote.ExecuteOnNodeWithDebugPod(checkCmd, nodeName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to read sysrq value on node %q: %v", nodeName, err)

			return false
		}

		currentValue, err = strconv.Atoi(strings.TrimSpace(output))
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to parse sysrq value %q on node %q: %v", output, nodeName, err)

			return false
		}

		return true
	}).WithContext(ctx).WithTimeout(30*time.Second).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to read sysrq value on node %q", nodeName))

	return currentValue
}

// setSysRqValue sets and verifies the kernel.sysrq value on the specified node.
func setSysRqValue(ctx SpecContext, nodeName string, newValue int) {
	const crashBit = 128

	setCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
		fmt.Sprintf("echo %d > /proc/sys/kernel/sysrq", newValue)}
	checkCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "cat /proc/sys/kernel/sysrq"}

	Eventually(func() bool {
		_, err := remote.ExecuteOnNodeWithDebugPod(setCmd, nodeName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to set sysrq value on node %q: %v", nodeName, err)

			return false
		}

		// Verify the value was set correctly
		output, err := remote.ExecuteOnNodeWithDebugPod(checkCmd, nodeName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to verify sysrq value on node %q: %v", nodeName, err)

			return false
		}

		verifyValue, err := strconv.Atoi(strings.TrimSpace(output))
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to parse verified sysrq value on node %q: %v", nodeName, err)

			return false
		}

		// Check if crash bit is now enabled
		if (verifyValue & crashBit) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"SysRq crash bit not enabled after setting on node %q (expected: %d, got: %d)",
				nodeName, newValue, verifyValue)

			return false
		}

		return true
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to set and verify sysrq value on node %q", nodeName))
}

// ensureSysRqCrashEnabled checks if kernel.sysrq has the crash bit (128) enabled.
// If not, it temporarily enables it for the current boot session.
// Changes are automatically reverted after node reboot.
func ensureSysRqCrashEnabled(ctx SpecContext, nodeName string) {
	By(fmt.Sprintf("Validating SysRq configuration on node %q", nodeName))

	currentValue := readSysRqValue(ctx, nodeName)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Current kernel.sysrq value on node %q: %d", nodeName, currentValue)

	// Check if crash bit (128) is enabled
	const crashBit = 128
	if (currentValue & crashBit) != 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"SysRq crash bit already enabled on node %q", nodeName)

		return
	}

	// Enable required bits: sync (16) + remount-ro (32) + crash (128)
	const requiredMask = 16 | 32 | 128 // = 176

	newValue := currentValue | requiredMask

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Temporarily enabling SysRq crash bit on node %q: %d -> %d (auto-reverts after reboot)",
		nodeName, currentValue, newValue)

	setSysRqValue(ctx, nodeName, newValue)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Successfully enabled SysRq crash bit on node %q (temporary until reboot)", nodeName)
}

// fetchNodeBootIDWithRetry fetches a node with retries and returns its boot ID.
func fetchNodeBootIDWithRetry(ctx SpecContext, nodeName string) string {
	var bootID string

	Eventually(func() bool {
		var pullErr error

		currentNode, pullErr := nodes.Pull(APIClient, nodeName)
		if pullErr != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to fetch node %q (retrying): %v", nodeName, pullErr)

			return false
		}

		bootID = currentNode.Object.Status.NodeInfo.BootID
		if bootID == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Node %q boot ID is empty (retrying)", nodeName)

			return false
		}

		return true
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		"Failed to fetch current boot ID for node %q", nodeName)

	return bootID
}

// fetchNodeWithRetry fetches a node with retries and returns the node object.
func fetchNodeWithRetry(ctx SpecContext, nodeName string) *nodes.Builder {
	var currentNode *nodes.Builder

	Eventually(func() bool {
		var pullErr error

		currentNode, pullErr = nodes.Pull(APIClient, nodeName)
		if pullErr != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to fetch node %q (retrying): %v", nodeName, pullErr)

			return false
		}

		return true
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		"Failed to fetch node %q", nodeName)

	return currentNode
}

func crashNodeKDump(ctx SpecContext, nodeLabel string) {
	var (
		nodeList []*nodes.Builder
		err      error
	)

	if nodeLabel == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node Label is empty. Skipping...")

		Skip("Empty node selector label")
	}

	By("Retrieve nodes list")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Find nodes matching label %q", nodeLabel)

	Eventually(func() bool {
		nodeList, err = nodes.List(
			APIClient,
			metav1.ListOptions{LabelSelector: nodeLabel},
		)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes: %v", err)

			return false
		}

		return len(nodeList) > 0
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to find nodes matching label: %q", nodeLabel))

	for _, node := range nodeList {
		nodeName := node.Definition.Name

		By(fmt.Sprintf("Cleaning up /var/crash directory on node %q", nodeName))
		cleanupVarCrashDirectory(ctx, nodeName)

		By(fmt.Sprintf("Ensuring SysRq crash capability on node %q", nodeName))

		ensureSysRqCrashEnabled(ctx, nodeName)

		By(fmt.Sprintf("Capturing current boot ID for node %q", nodeName))

		// Re-fetch node to get the current boot ID (not stale from earlier nodeList)
		originalBootID := fetchNodeBootIDWithRetry(ctx, nodeName)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %q current boot ID: %s", nodeName, originalBootID)

		By("Trigger kernel crash")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Trigerring kernel crash on %q",
			nodeName)

		err = reboot.KernelCrashKdump(nodeName)
		Expect(err).ToNot(HaveOccurred(), "Error triggering a kernel crash on the node.")

		By("Waiting for node to reboot (boot ID change)")

		waitForBootIDChange(ctx, nodeName, originalBootID, 15*time.Minute)

		By("Waiting for node to go into Ready state")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking node %q got into Ready state",
			nodeName)

		// Re-fetch node after reboot to get fresh object for WaitUntilReady
		rebootedNode := fetchNodeWithRetry(ctx, nodeName)

		err = rebootedNode.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Error waiting for node to go into Ready state")

		verifyVmcoreDumpGenerated(ctx, nodeName)

		cleanupVarCrashDirectory(ctx, nodeName)
	}
}

// VerifyKDumpOnControlPlane check KDump service on Control Plane nodes.
func VerifyKDumpOnControlPlane(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	crashNodeKDump(ctx, RDSCoreConfig.KDumpCPNodeLabel)
}

// VerifyKDumpOnWorkerMCP check KDump service on nodes in "Worker" MCP.
func VerifyKDumpOnWorkerMCP(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	crashNodeKDump(ctx, RDSCoreConfig.KDumpWorkerMCPNodeLabel)
}

// VerifyKDumpOnCNFMCP check KDump service on nodes in "CNF" MCP.
func VerifyKDumpOnCNFMCP(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	crashNodeKDump(ctx, RDSCoreConfig.KDumpCNFMCPNodeLabel)
}

// CleanupUnexpectedAdmissionPodsCP cleans up pods with UnexpectedAdmissionError status
// on the Control Plane nodes.
func CleanupUnexpectedAdmissionPodsCP(ctx SpecContext) {
	cleanupUnexpectedPods(RDSCoreConfig.KDumpCPNodeLabel)
}

// CleanupUnexpectedAdmissionPodsWorker cleans up pods with UnexpectedAdmissionError status
// on the Worker nodes.
func CleanupUnexpectedAdmissionPodsWorker(ctx SpecContext) {
	cleanupUnexpectedPods(RDSCoreConfig.KDumpWorkerMCPNodeLabel)
}

// CleanupUnexpectedAdmissionPodsCNF cleans up pods with UnexpectedAdmissionError status
// on the CNF nodes.
func CleanupUnexpectedAdmissionPodsCNF(ctx SpecContext) {
	cleanupUnexpectedPods(RDSCoreConfig.KDumpCNFMCPNodeLabel)
}

func cleanupUnexpectedPods(nodeLabel string) {
	listOptions := metav1.ListOptions{
		FieldSelector: "status.phase=Failed",
	}

	var (
		nodeList []*nodes.Builder
		podsList []*pod.Builder
		err      error
		ctx      SpecContext
	)

	By("Searching for pods with UnexpectedAdmissionError status")

	Eventually(func() bool {
		podsList, err = pod.ListInAllNamespaces(APIClient, listOptions)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods matching search criteria",
			len(podsList))

		for _, failedPod := range podsList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q ns matches search criteria",
				failedPod.Definition.Name, failedPod.Definition.Namespace)
		}

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to search for pods with UnexpectedAdmissionError status")

	if len(podsList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No pods with UnexpectedAdmissionError status found")

		return
	}

	By("Retrieving nodes list")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Find nodes matching label %q", nodeLabel)

	Eventually(func() bool {
		nodeList, err = nodes.List(
			APIClient,
			metav1.ListOptions{LabelSelector: nodeLabel},
		)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes: %v", err)

			return false
		}

		return len(nodeList) > 0
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to find node(s) matching label: %q", nodeLabel))

	for _, _node := range nodeList {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q macthes label %q",
			_node.Definition.Name, nodeLabel)
	}

	By("Filtering pods with UnexpectedAdmissionError that run on the target node(s)")

	for _, failedPod := range podsList {
		if failedPod.Definition.Status.Reason == "UnexpectedAdmissionError" {
			for _, _node := range nodeList {
				if _node.Definition.Name == failedPod.Definition.Spec.NodeName {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting pod %q in %q ns running on %q",
						failedPod.Definition.Name, failedPod.Definition.Namespace, _node.Definition.Name)

					_, err := failedPod.DeleteAndWait(5 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "could not delete pod in UnexpectedAdmissionError state")
				}
			}
		}
	}
}
