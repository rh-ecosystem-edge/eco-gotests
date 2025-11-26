package sriovenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	sriovconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovconfig"
	ocpsriovinittools "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsSriovDeployed checks if SR-IOV operator is deployed in the given namespace.
// This function is kept for backward compatibility but should use sriovoperator.IsSriovDeployed instead.
//
// Deprecated: Use sriovoperator.IsSriovDeployed instead.
func IsSriovDeployed(apiClient *clients.Settings, config interface{}) error {
	var namespace string

	switch sriovConfig := config.(type) {
	case *sriovconfig.SriovOcpConfig:
		if sriovConfig == nil {
			return fmt.Errorf("sriov config cannot be nil")
		}

		namespace = sriovConfig.OcpSriovOperatorNamespace
	default:
		return fmt.Errorf("unsupported config type")
	}

	// Delegate to centralized function
	return sriovoperator.IsSriovDeployed(apiClient, namespace)
}

// PullTestImageOnNodes pulls given image on range of relevant nodes based on nodeSelector
// Note: Image pulling is deferred to first pod creation. When the first test pod is created,
// the kubelet will automatically pull the image from the registry.
func PullTestImageOnNodes(apiClient *clients.Settings, nodeSelector, image string, pullTimeout int) error {
	// Image pulling is deferred to first pod creation. The kubelet automatically pulls images
	// when pods are created, avoiding unnecessary pulls for skipped tests. Trade-off: first pod
	// creation may take longer. If pre-pulling is needed, deploy a DaemonSet to pre-pull images.
	if apiClient == nil {
		klog.V(90).Info("API client is nil in PullTestImageOnNodes; continuing because image pull is deferred to kubelet")
	}

	klog.V(90).Infof(
		"Image pulling deferred to pod creation. Image: %q, nodeSelector: %q, pullTimeoutSeconds: %d. "+
			"Images will be pulled on first pod creation; this may take extra time on first pod launch.",
		image, nodeSelector, pullTimeout)

	return nil
}

// CleanAllNetworksByTargetNamespace cleans all networks by target namespace.
func CleanAllNetworksByTargetNamespace(apiClient *clients.Settings, sriovOpNs, targetNs string) error {
	klog.V(90).Infof("Cleaning up SR-IOV networks for target namespace %q (operator_namespace: %q)", targetNs, sriovOpNs)

	// List all SriovNetwork resources in the operator namespace
	sriovNetworks, err := sriov.List(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		// Log at warning level with context so callers can distinguish API failures from "no networks found"
		klog.Warningf("Failed to list SR-IOV networks in namespace %q for cleanup: %v", sriovOpNs, err)
		// Return wrapped error so callers can handle API failures appropriately
		return fmt.Errorf("failed to list SR-IOV networks in namespace %q: %w", sriovOpNs, err)
	}

	networksCleaned := 0

	for _, network := range sriovNetworks {
		// Check if this network targets the given namespace
		// SR-IOV networks have a networkNamespace field that indicates the target namespace
		if network.Definition.Spec.NetworkNamespace != targetNs {
			continue
		}

		klog.V(90).Infof("Deleting SR-IOV network %q in namespace %q (target_namespace: %q)",
			network.Definition.Name, sriovOpNs, targetNs)

		// Delete the SriovNetwork CR
		err := network.Delete()
		if err != nil && !apierrors.IsNotFound(err) {
			klog.V(90).Infof("Error deleting SR-IOV network %q: %v", network.Definition.Name, err)

			continue
		}

		networksCleaned++

		// Also delete the corresponding NetworkAttachmentDefinition in the target namespace
		nadName := network.Definition.Name

		nadBuilder := nad.NewBuilder(apiClient, nadName, targetNs)
		if nadBuilder.Exists() {
			klog.V(90).Infof("Deleting NetworkAttachmentDefinition %q in namespace %q", nadName, targetNs)

			err := nadBuilder.Delete()
			if err != nil && !apierrors.IsNotFound(err) {
				klog.V(90).Infof("Error deleting NetworkAttachmentDefinition %q: %v", nadName, err)
				// Continue even if NAD deletion fails
			}
		}
	}

	klog.V(90).Infof("Cleanup complete: cleaned %d networks for target namespace %q", networksCleaned, targetNs)

	// No need to sleep - deletions are handled asynchronously by Kubernetes
	// The caller should wait for resources to be deleted if needed

	return nil
}

// cleanupTestNamespaces removes leftover e2e test namespaces.
func cleanupTestNamespaces(apiClient *clients.Settings) error {
	namespaceList, err := namespace.List(apiClient, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	for _, nsBuilder := range namespaceList {
		if strings.HasPrefix(nsBuilder.Definition.Name, "e2e-") {
			klog.V(90).Infof("Removing leftover namespace %q", nsBuilder.Definition.Name)

			if err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout); err != nil {
				_ = nsBuilder.Delete() // Try force delete
			}
		}
	}

	return nil
}

// cleanupTestNetworks removes leftover SR-IOV networks matching test patterns.
func cleanupTestNetworks(apiClient *clients.Settings, sriovOpNs string) {
	networks, err := sriov.List(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Failed to list networks for cleanup: %v", err)

		return
	}

	pattern := regexp.MustCompile(`^\d{5}-|\w+dpdknet$`)

	for _, net := range networks {
		if pattern.MatchString(net.Definition.Name) {
			klog.V(90).Infof("Removing leftover network %q", net.Definition.Name)
			_ = net.Delete()
		}
	}
}

// getTestDeviceNames returns the list of test device names for cleanup.
func getTestDeviceNames() []string {
	deviceConfigs := tsparams.GetDeviceConfig()
	names := make([]string, 0, len(deviceConfigs)+1)
	seen := make(map[string]bool)

	for _, device := range deviceConfigs {
		names = append(names, device.Name)
		seen[device.Name] = true
	}

	// Include legacy device name
	if !seen["cx5ex"] {
		names = append(names, "cx5ex")
	}

	return names
}

// cleanupTestPolicies removes leftover SR-IOV policies matching test device names.
func cleanupTestPolicies(apiClient *clients.Settings, sriovOpNs string) {
	policies, err := sriov.ListPolicy(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Failed to list policies for cleanup: %v", err)

		return
	}

	deviceNames := getTestDeviceNames()

	for _, policy := range policies {
		policyName := policy.Definition.Name

		for _, deviceName := range deviceNames {
			if policyName == deviceName || strings.HasPrefix(policyName, deviceName) {
				klog.V(90).Infof("Removing leftover policy %q", policyName)

				_ = policy.Delete()

				break
			}
		}
	}
}

// CleanupLeftoverResources cleans up any leftover resources from previous failed test runs.
// This should be called at the beginning of the test suite to ensure a clean state.
func CleanupLeftoverResources(apiClient *clients.Settings, sriovOperatorNamespace string) error {
	klog.V(90).Info("Starting cleanup of leftover resources")

	if err := cleanupTestNamespaces(apiClient); err != nil {
		return err
	}

	cleanupTestNetworks(apiClient, sriovOperatorNamespace)
	cleanupTestPolicies(apiClient, sriovOperatorNamespace)

	klog.V(90).Info("Cleanup completed")

	return nil
}

// isPolicyNotFoundError checks if the error indicates the policy doesn't exist.
// This handles both Kubernetes API NotFound errors and eco-goinfra custom errors.
func isPolicyNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if apierrors.IsNotFound(err) {
		return true
	}

	// eco-goinfra returns custom error messages like "does not exist"
	errMsg := err.Error()

	return strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found")
}

// RemoveSriovPolicy removes a SRIOV policy by name if it exists.
func RemoveSriovPolicy(apiClient *clients.Settings, name, sriovOpNs string, timeout time.Duration) error {
	klog.V(90).Infof("Removing SRIOV policy %q if it exists in namespace %q", name, sriovOpNs)

	// Use PullPolicy to check if the policy exists (doesn't require resourceName)
	policyBuilder, err := sriov.PullPolicy(apiClient, name, sriovOpNs)
	if err != nil {
		if isPolicyNotFoundError(err) {
			klog.V(90).Infof("SRIOV policy %q does not exist, skipping deletion", name)

			return nil
		}

		return fmt.Errorf("failed to check policy %q: %w", name, err)
	}

	err = policyBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete SRIOV policy %q: %w", name, err)
	}

	// Wait for policy to be deleted using wait.PollUntilContextTimeout
	klog.V(90).Infof("Waiting for SRIOV policy %q to be deleted", name)

	err = wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := sriov.PullPolicy(apiClient, name, sriovOpNs)

			return isPolicyNotFoundError(pullErr), nil
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for SRIOV policy %q to be deleted from namespace %q: %w",
			name, sriovOpNs, err)
	}

	klog.V(90).Infof("SRIOV policy %q successfully deleted", name)

	return nil
}

// ErrNetworkNotFound is returned when a SRIOV network is not found.
var ErrNetworkNotFound = fmt.Errorf("sriov network not found")

// findSriovNetwork finds a SRIOV network by name and returns its target namespace.
// Returns ErrNetworkNotFound if the network does not exist.
func findSriovNetwork(
	apiClient *clients.Settings,
	name, sriovOpNs string,
) (*sriov.NetworkBuilder, string, error) {
	networks, err := sriov.List(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list networks: %w", err)
	}

	for _, network := range networks {
		if network.Object.Name == name {
			targetNs := network.Object.Spec.NetworkNamespace
			if targetNs == "" {
				targetNs = sriovOpNs
			}

			builder := sriov.NewNetworkBuilder(
				apiClient, name, sriovOpNs, targetNs, network.Object.Spec.ResourceName)

			return builder, targetNs, nil
		}
	}

	return nil, "", fmt.Errorf("%w: %q in namespace %q", ErrNetworkNotFound, name, sriovOpNs)
}

// isNADNotFoundError checks if the error indicates the NAD doesn't exist.
// This handles both Kubernetes API NotFound errors and eco-goinfra custom errors.
func isNADNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if apierrors.IsNotFound(err) {
		return true
	}

	// eco-goinfra returns custom error messages like "does not exist"
	errMsg := err.Error()

	return strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found")
}

// isNetworkNotFoundError checks if the error indicates the SriovNetwork doesn't exist.
// This handles both Kubernetes API NotFound errors and eco-goinfra custom errors.
func isNetworkNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	if apierrors.IsNotFound(err) {
		return true
	}

	// eco-goinfra returns custom error messages like "does not exist"
	errMsg := err.Error()

	return strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found")
}

// waitForNADDeletion waits for a NAD to be deleted.
func waitForNADDeletion(apiClient *clients.Settings, name, namespace string) error {
	// Check if NAD exists first
	if _, err := nad.Pull(apiClient, name, namespace); err != nil {
		if isNADNotFoundError(err) {
			return nil
		}

		return err
	}

	// Wait for deletion
	err := wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.NADTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := nad.Pull(apiClient, name, namespace)

			return isNADNotFoundError(pullErr), nil
		})
	if err == nil {
		return nil
	}

	// Try force delete
	if nadBuilder, pullErr := nad.Pull(apiClient, name, namespace); pullErr == nil && nadBuilder != nil {
		_ = nadBuilder.Delete()
	}

	// Final check
	if _, finalErr := nad.Pull(apiClient, name, namespace); isNADNotFoundError(finalErr) {
		return nil
	}

	return fmt.Errorf("NAD %q not deleted in namespace %q within timeout", name, namespace)
}

// RemoveSriovNetwork removes a SRIOV network by name from the operator namespace.
// The timeout parameter governs the SriovNetwork CR deletion phase.
// NAD deletion in the target namespace uses a fixed tsparams.NADTimeout.
// If a single timeout budget for the entire cleanup is needed, callers should
// account for both phases or adjust tsparams.NADTimeout accordingly.
func RemoveSriovNetwork(apiClient *clients.Settings, name, sriovOpNs string, timeout time.Duration) error {
	klog.V(90).Infof("Removing SRIOV network %q", name)

	targetNetwork, targetNs, err := findSriovNetwork(apiClient, name, sriovOpNs)
	if err != nil {
		// If network not found, it's already deleted - not an error
		if errors.Is(err, ErrNetworkNotFound) {
			klog.V(90).Infof("Network %q not found or already deleted", name)

			return nil
		}

		return err
	}

	if targetNetwork == nil || !targetNetwork.Exists() {
		klog.V(90).Infof("Network %q not found or already deleted", name)

		return nil
	}

	if err := targetNetwork.Delete(); err != nil {
		return fmt.Errorf("failed to delete network %q: %w", name, err)
	}

	// Wait for network deletion - SriovNetwork CR is in the operator namespace
	err = wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := sriov.PullNetwork(apiClient, name, sriovOpNs)
			if pullErr == nil {
				// Network still exists
				return false, nil
			}

			// Check if it's a NotFound error (network deleted successfully)
			if apierrors.IsNotFound(pullErr) || isNetworkNotFoundError(pullErr) {
				return true, nil
			}

			// Unexpected API failure - surface it with context
			return false, fmt.Errorf("failed to verify network %q deletion: %w", name, pullErr)
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for network %q deletion: %w", name, err)
	}

	// Wait for NAD deletion if in different namespace
	if targetNs != sriovOpNs {
		if err := waitForNADDeletion(apiClient, name, targetNs); err != nil {
			return err
		}
	}

	klog.V(90).Infof("Network %q deleted", name)

	return nil
}

// WaitForPodWithLabelReady waits for a pod with specific label to be ready.
// The timeout parameter is used only for per-pod readiness checks (WaitUntilReady).
// Pod discovery uses the fixed tsparams.PodLabelReadyTimeout constant.
func WaitForPodWithLabelReady(
	apiClient *clients.Settings,
	namespace, labelSelector string,
	timeout time.Duration,
) error {
	klog.V(90).Infof("Waiting for pod with label %q to be ready in namespace %q",
		labelSelector, namespace)

	// Wait for pod to appear using wait.PollUntilContextTimeout
	var (
		podList []*pod.Builder
		listErr error
	)

	err := wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.PodLabelReadyTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			podList, listErr = pod.List(
				apiClient, namespace, metav1.ListOptions{LabelSelector: labelSelector})
			if listErr != nil {
				klog.V(90).Infof("Failed to list pods, will retry: %v (namespace: %q, label: %q)",
					listErr, namespace, labelSelector)

				return false, nil
			}

			return len(podList) > 0, nil
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for pod with label %q in namespace %q: %w",
			labelSelector, namespace, err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("no pods found with label %q in namespace %q", labelSelector, namespace)
	}

	// Wait for each pod to be ready
	for _, podBuilder := range podList {
		klog.V(90).Infof("Waiting for pod %q to be ready in namespace %q",
			podBuilder.Definition.Name, namespace)

		err := podBuilder.WaitUntilReady(timeout)
		if err != nil {
			// Log pod status for debugging
			if podBuilder.Definition != nil {
				klog.V(90).Infof("Pod status details - name: %q, phase: %q, reason: %q, message: %q",
					podBuilder.Definition.Name,
					podBuilder.Definition.Status.Phase,
					podBuilder.Definition.Status.Reason,
					podBuilder.Definition.Status.Message)

				// Log container statuses
				for _, cs := range podBuilder.Definition.Status.ContainerStatuses {
					klog.V(90).Infof("Container status - name: %q, ready: %v, state: %+v",
						cs.Name, cs.Ready, cs.State)
				}

				// Log conditions
				for _, cond := range podBuilder.Definition.Status.Conditions {
					klog.V(90).Infof("Pod condition - type: %q, status: %q, reason: %q, message: %q",
						cond.Type, cond.Status, cond.Reason, cond.Message)
				}
			}

			return fmt.Errorf("pod %q not ready in namespace %q: %w",
				podBuilder.Definition.Name, namespace, err)
		}
	}

	klog.V(90).Infof("All pods with label %q are ready in namespace %q", labelSelector, namespace)

	return nil
}

// checkSriovNodeStatesSynced checks if all SR-IOV node states are synced.
func checkSriovNodeStatesSynced(apiClient *clients.Settings, sriovOpNs string) (bool, int) {
	nodeStates, err := sriov.ListNetworkNodeState(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Failed to list SR-IOV node states: %v", err)

		return false, 0
	}

	if len(nodeStates) == 0 {
		klog.V(90).Info("No SR-IOV node states found")

		return false, 0
	}

	for _, nodeState := range nodeStates {
		if nodeState.Objects != nil && nodeState.Objects.Status.SyncStatus != "Succeeded" {
			return false, len(nodeStates)
		}
	}

	return true, len(nodeStates)
}

// mcpMatchesLabel checks if an MCP matches the given label selector.
func mcpMatchesLabel(mcp *mco.MCPBuilder, mcpLabel string) bool {
	if mcpLabel == "" {
		return true
	}

	parts := strings.SplitN(mcpLabel, "=", 2)
	if len(parts) != 2 || mcp.Object.Labels == nil {
		return false
	}

	val, ok := mcp.Object.Labels[parts[0]]

	return ok && val == parts[1]
}

// checkMCPStable checks if all matching MCPs are stable.
func checkMCPStable(apiClient *clients.Settings, mcpLabel string) bool {
	mcpList, err := mco.ListMCP(apiClient)
	if err != nil {
		if strings.Contains(err.Error(), "no kind is registered") {
			return true // MCP not available, skip check
		}

		klog.V(90).Infof("Failed to list MachineConfigPools: %v", err)

		return false
	}

	for _, mcp := range mcpList {
		if !mcpMatchesLabel(mcp, mcpLabel) {
			continue
		}

		isUpdated := false

		for _, cond := range mcp.Object.Status.Conditions {
			if cond.Type == machineconfigv1.MachineConfigPoolUpdated && cond.Status == corev1.ConditionTrue {
				isUpdated = true
			}

			if cond.Type == machineconfigv1.MachineConfigPoolDegraded && cond.Status == corev1.ConditionTrue {
				return false
			}
		}

		if !isUpdated {
			return false
		}
	}

	return true
}

// nodeMatchesWorkerLabel checks if a node matches the worker label selector.
func nodeMatchesWorkerLabel(node *nodes.Builder, workerLabel, labelKey string) bool {
	if workerLabel != "" {
		labelSelector, err := labels.Parse(workerLabel)
		if err == nil {
			return labelSelector.Matches(labels.Set(node.Definition.Labels))
		}
		// Fallback to simple key check
		_, hasLabel := node.Definition.Labels[labelKey]

		return hasLabel
	}

	// Default: check for standard worker label
	_, hasLabel := node.Definition.Labels[labelKey]

	return hasLabel
}

// isNodeReady checks if a node is ready and has no resource pressure.
// Returns (isReady, hasPressure).
func isNodeReady(node *nodes.Builder) (bool, bool) {
	for _, cond := range node.Definition.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return true, false
		}

		if (cond.Type == corev1.NodeMemoryPressure || cond.Type == corev1.NodeDiskPressure) &&
			cond.Status == corev1.ConditionTrue {
			return false, true
		}
	}

	return false, false
}

// checkWorkerNodesReady checks if all worker nodes are ready.
// workerLabel should be in format "key=value" or "key=". If empty, defaults to "node-role.kubernetes.io/worker".
func checkWorkerNodesReady(apiClient *clients.Settings, workerLabel string) (bool, int) {
	nodeList, err := nodes.List(apiClient, metav1.ListOptions{})
	if err != nil {
		klog.V(90).Infof("Failed to list nodes while checking worker readiness: %v", err)

		return false, 0
	}

	// Parse worker label to extract key
	labelKey := "node-role.kubernetes.io/worker"

	if workerLabel != "" {
		if parts := strings.SplitN(workerLabel, "=", 2); len(parts) > 0 && parts[0] != "" {
			labelKey = parts[0]
		}
	}

	readyCount := 0
	workerCount := 0

	for _, node := range nodeList {
		if !nodeMatchesWorkerLabel(node, workerLabel, labelKey) {
			continue
		}

		workerCount++

		ready, hasPressure := isNodeReady(node)
		if hasPressure || !ready {
			return false, readyCount
		}

		readyCount++
	}

	if workerCount == 0 {
		klog.Warningf("No worker nodes found matching label %q - check if worker label is configured correctly",
			labelKey)

		return false, 0
	}

	return true, readyCount
}

// WaitForSriovAndMCPStable waits for SRIOV and MCP to be stable.
// workerLabel should be in format "key=value" or "key=". If empty, defaults to "node-role.kubernetes.io/worker".
func WaitForSriovAndMCPStable(
	apiClient *clients.Settings,
	timeout time.Duration,
	interval time.Duration,
	mcpLabel, sriovOpNs, workerLabel string,
) error {
	klog.V(90).Infof("Waiting for SR-IOV and MCP to be stable (timeout: %v)", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextCancel(ctx, interval, false, func(ctx context.Context) (bool, error) {
		// Check SR-IOV node states
		synced, nodeCount := checkSriovNodeStatesSynced(apiClient, sriovOpNs)
		if !synced {
			return false, nil
		}

		// Check MCP stability
		if !checkMCPStable(apiClient, mcpLabel) {
			return false, nil
		}

		// Check worker nodes
		ready, readyCount := checkWorkerNodesReady(apiClient, workerLabel)
		if !ready {
			return false, nil
		}

		klog.V(90).Infof("SUCCESS: All checks passed (sriov_nodes: %d, workers_ready: %d)",
			nodeCount, readyCount)

		return true, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timeout waiting for SRIOV and MCP stability after %v", timeout)
		}

		return fmt.Errorf("failed waiting for SRIOV and MCP stability: %w", err)
	}

	return nil
}

// normalizeWorkerLabel ensures the worker label is in the correct format for Kubernetes label selectors.
// If the label doesn't contain "=", it appends "=" to make it a valid selector (e.g., "key" -> "key=").
// This handles cases where the label is provided as just a key name without the "=" separator.
func normalizeWorkerLabel(label string) string {
	if label == "" {
		return label
	}

	if !strings.Contains(label, "=") {
		return label + "="
	}

	return label
}

// VerifyVFResourcesAvailable checks if VF resources are advertised and available on worker nodes.
func VerifyVFResourcesAvailable(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	resourceName string,
) (bool, error) {
	if apiClient == nil {
		return false, fmt.Errorf("API client is nil, cannot verify VF resources")
	}

	if config == nil {
		return false, fmt.Errorf("config is nil - cannot verify VF resources")
	}

	// Normalize worker label to ensure it's in the correct format for label selector
	normalizedLabel := normalizeWorkerLabel(config.OcpWorkerLabel)

	// Guard against empty worker label to prevent scanning all nodes (including masters)
	if normalizedLabel == "" {
		return false, fmt.Errorf("OcpWorkerLabel is empty - cannot determine worker nodes")
	}

	// Get all worker nodes
	workerNodes, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: normalizedLabel})
	if err != nil {
		return false, fmt.Errorf("failed to list worker nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return false, fmt.Errorf("no worker nodes found")
	}

	// Check each node for available VF resources
	for _, node := range workerNodes {
		nodeName := node.Definition.Name

		// Check node allocatable resources
		allocatable := node.Definition.Status.Allocatable
		capacity := node.Definition.Status.Capacity

		// Look for the resource in both allocatable and capacity
		resourceKey := fmt.Sprintf("openshift.io/%s", resourceName)

		capacityValue, hasCapacity := capacity[corev1.ResourceName(resourceKey)]
		allocatableValue, hasAllocatable := allocatable[corev1.ResourceName(resourceKey)]

		if !hasCapacity && !hasAllocatable {
			klog.V(90).Infof("VF resource %q not found on node %q", resourceKey, nodeName)

			continue
		}

		// Check if there are available resources (allocatable > 0)
		if hasAllocatable {
			allocQty := allocatableValue.Value()
			if allocQty > 0 {
				klog.V(90).Infof("VF resources available on node %q (resource: %q, capacity: %s, allocatable: %s)",
					nodeName, resourceKey, capacityValue.String(), allocatableValue.String())

				return true, nil
			}
		}

		if hasCapacity {
			capQty := capacityValue.Value()
			if capQty > 0 && !hasAllocatable {
				klog.V(90).Infof("VF resources exist but not allocatable on node %q (resource: %q, capacity: %s)",
					nodeName, resourceKey, capacityValue.String())

				continue
			}
		}

		klog.V(90).Infof("No allocatable VF resources on node %q (resource: %q, capacity: %s, allocatable: %s)",
			nodeName, resourceKey, capacityValue.String(), allocatableValue.String())
	}

	// If we get here, no nodes have available resources
	klog.V(90).Infof("VF resources %q not available on any worker node", resourceName)

	return false, nil
}

// SriovNetworkConfig represents configuration for creating a SRIOV network.
type SriovNetworkConfig struct {
	Name             string
	ResourceName     string
	NetworkNamespace string
	Namespace        string
	SpoofCheck       string
	Trust            string
	VlanID           int
	VlanQoS          int
	MinTxRate        int
	MaxTxRate        int
	LinkState        string
}

// buildSriovNetworkBuilder creates and configures a SRIOV network builder with the given config.
// Returns an error if any configuration value exceeds valid ranges.
func buildSriovNetworkBuilder(
	apiClient *clients.Settings,
	config *SriovNetworkConfig,
) (*sriov.NetworkBuilder, error) {
	networkBuilder := sriov.NewNetworkBuilder(
		apiClient,
		config.Name,
		config.Namespace,
		config.NetworkNamespace,
		config.ResourceName,
	).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")

	// Set optional parameters
	if config.SpoofCheck != "" {
		networkBuilder.WithSpoof(config.SpoofCheck == "on")
	}

	if config.Trust != "" {
		networkBuilder.WithTrustFlag(config.Trust == "on")
	}

	// maxUint16 is the maximum value for uint16 fields (VLAN, QoS, rates).
	// Fail fast if values exceed this limit to prevent silent truncation in test infrastructure.
	const maxUint16 = 65535

	if config.VlanID > 0 {
		if config.VlanID > maxUint16 {
			return nil, fmt.Errorf("VlanID %d exceeds maximum allowed value %d", config.VlanID, maxUint16)
		}

		networkBuilder.WithVLAN(uint16(config.VlanID))
	}

	if config.VlanQoS > 0 {
		if config.VlanQoS > maxUint16 {
			return nil, fmt.Errorf("VlanQoS %d exceeds maximum allowed value %d", config.VlanQoS, maxUint16)
		}

		networkBuilder.WithVlanQoS(uint16(config.VlanQoS))
	}

	if config.MinTxRate > 0 {
		if config.MinTxRate > maxUint16 {
			return nil, fmt.Errorf("MinTxRate %d exceeds maximum allowed value %d", config.MinTxRate, maxUint16)
		}

		networkBuilder.WithMinTxRate(uint16(config.MinTxRate))
	}

	if config.MaxTxRate > 0 {
		if config.MaxTxRate > maxUint16 {
			return nil, fmt.Errorf("MaxTxRate %d exceeds maximum allowed value %d", config.MaxTxRate, maxUint16)
		}

		networkBuilder.WithMaxTxRate(uint16(config.MaxTxRate))
	}

	// Set LinkState with default to "auto" if not specified
	linkState := config.LinkState
	if linkState == "" {
		linkState = "auto"

		klog.V(90).Infof("LinkState defaulting to 'auto' for network %q", config.Name)
	}

	networkBuilder.WithLinkState(linkState)

	return networkBuilder, nil
}

// waitForSriovPolicy waits for the SRIOV policy to exist.
func waitForSriovPolicy(
	apiClient *clients.Settings,
	resourceName, namespace string,
) error {
	klog.V(90).Infof("Verifying SRIOV policy exists for resource %q", resourceName)

	return wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.NamespaceTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			policy, err := sriov.PullPolicy(apiClient, resourceName, namespace)
			if err == nil && policy != nil && policy.Object != nil {
				klog.V(90).Infof("SRIOV policy found - name: %q, numVfs: %d",
					resourceName, policy.Object.Spec.NumVfs)

				return true, nil
			}

			return false, nil
		})
}

// waitForNAD waits for the NetworkAttachmentDefinition to be created.
func waitForNAD(apiClient *clients.Settings, name, namespace string) error {
	klog.V(90).Infof("Waiting for NAD %q in namespace %q", name, namespace)

	return wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.NADTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(apiClient, name, namespace)

			return err == nil, nil
		})
}

// CreateSriovNetwork creates a SRIOV network and waits for it to be ready.
// Actual waiting behavior is governed by tsparams constants:
// - NamespaceTimeout for policy verification
// - NADTimeout for NetworkAttachmentDefinition creation
// - VFResourceTimeout for VF resource availability.
func CreateSriovNetwork(apiClient *clients.Settings, config *SriovNetworkConfig) error {
	klog.V(90).Infof("Creating SRIOV network %q in namespace %q (resource: %q)",
		config.Name, config.Namespace, config.ResourceName)

	networkBuilder, err := buildSriovNetworkBuilder(apiClient, config)
	if err != nil {
		return fmt.Errorf("invalid network configuration: %w", err)
	}

	sriovNetwork, err := networkBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create SRIOV network %q: %w", config.Name, err)
	}

	// Log creation success
	if sriovNetwork != nil && sriovNetwork.Object != nil {
		klog.V(90).Infof("SRIOV network created - name: %q, resource: %q",
			config.Name, sriovNetwork.Object.Spec.ResourceName)
	}

	// Verify policy exists before waiting for NAD
	if err := waitForSriovPolicy(apiClient, config.ResourceName, config.Namespace); err != nil {
		return fmt.Errorf("SRIOV policy %q must exist before NAD creation: %w",
			config.ResourceName, err)
	}

	// Wait for NAD creation
	if err := waitForNAD(apiClient, config.Name, config.NetworkNamespace); err != nil {
		return fmt.Errorf("failed to wait for NAD %q: %w", config.Name, err)
	}

	// Check VF resources (non-blocking)
	klog.V(90).Infof("Verifying VF resources for %q", config.ResourceName)

	err = wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.VFResourcePollingInterval,
		tsparams.VFResourceTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			available, err := VerifyVFResourcesAvailable(
				apiClient, ocpsriovinittools.SriovOcpConfig, config.ResourceName)

			return available && err == nil, nil
		})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			klog.V(90).Infof("VF resources %q not yet available (may still be provisioning): %v. "+
				"Will proceed and re-check when pods are created.", config.ResourceName, err)
			// Proceed: VF availability will be re-checked by pod creation path
		} else {
			return fmt.Errorf("failed to verify VF resources %q: %w", config.ResourceName, err)
		}
	}

	klog.V(90).Infof("SRIOV network %q created and ready", config.Name)

	return nil
}

const (
	// DefaultMcpLabel is the default MachineConfigPool label for worker nodes.
	DefaultMcpLabel = "machineconfiguration.openshift.io/role=worker"
)

// CheckSriovOperatorStatus checks if SR-IOV operator is running and healthy.
func CheckSriovOperatorStatus(apiClient *clients.Settings, config *sriovconfig.SriovOcpConfig) error {
	if config == nil {
		return fmt.Errorf("SriovOcpConfig cannot be nil")
	}

	klog.V(90).Infof("Checking SR-IOV operator status in namespace %q", config.OcpSriovOperatorNamespace)
	// Use the centralized function from sriovoperator package

	return sriovoperator.IsSriovDeployed(apiClient, config.OcpSriovOperatorNamespace)
}

// WaitForSriovPolicyReady waits for SR-IOV policy to be ready and MCP to be stable.
func WaitForSriovPolicyReady(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	timeout time.Duration,
) error {
	if config == nil {
		return fmt.Errorf("SriovOcpConfig cannot be nil")
	}

	klog.V(90).Infof("Waiting for SR-IOV policy to be ready (timeout: %v)", timeout)

	return WaitForSriovAndMCPStable(
		apiClient, timeout, tsparams.MCPStableInterval, DefaultMcpLabel,
		config.OcpSriovOperatorNamespace, config.OcpWorkerLabel)
}

// checkSingleNodeReady checks if a single node is ready and returns any issues found.
func checkSingleNodeReady(apiClient *clients.Settings, nodeName string) error {
	refreshedNode, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to pull node: %w", err)
	}

	if refreshedNode == nil || refreshedNode.Definition == nil {
		return fmt.Errorf("node definition is nil")
	}

	hasReadyCondition := false

	for _, cond := range refreshedNode.Definition.Status.Conditions {
		// Check for reboot indicators first
		if strings.Contains(cond.Reason, "NodeNotReady") ||
			strings.Contains(cond.Reason, "Rebooting") ||
			strings.Contains(cond.Reason, "KernelDeadlock") {
			return fmt.Errorf("node appears unstable (reason: %q)", cond.Reason)
		}

		// Check specific condition types
		if cond.Type == corev1.NodeReady {
			switch cond.Status {
			case corev1.ConditionTrue:
				hasReadyCondition = true
			case corev1.ConditionFalse:
				return fmt.Errorf("node not ready (reason: %q)", cond.Reason)
			case corev1.ConditionUnknown:
				return fmt.Errorf("node ready status unknown")
			}
		}

		if (cond.Type == corev1.NodeMemoryPressure || cond.Type == corev1.NodeDiskPressure) &&
			cond.Status == corev1.ConditionTrue {
			return fmt.Errorf("node has %s", cond.Type)
		}
	}

	if !hasReadyCondition {
		return fmt.Errorf("node not in Ready state")
	}

	return nil
}

// VerifyWorkerNodesReady verifies that all worker nodes are stable and ready for SRIOV initialization.
func VerifyWorkerNodesReady(
	apiClient *clients.Settings,
	workerNodes []*nodes.Builder,
	sriovOpNs string,
) error {
	if apiClient == nil {
		return fmt.Errorf("API client is nil")
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no worker nodes provided")
	}

	klog.V(90).Infof("Verifying %d worker nodes for namespace %q", len(workerNodes), sriovOpNs)

	var lastErr error

	for _, node := range workerNodes {
		nodeName := node.Definition.Name

		if err := checkSingleNodeReady(apiClient, nodeName); err != nil {
			klog.V(90).Infof("Node %q not ready: %v", nodeName, err)
			lastErr = fmt.Errorf("node %q: %w", nodeName, err)
		}
	}

	if lastErr != nil {
		return fmt.Errorf("worker nodes not ready: %w", lastErr)
	}

	klog.V(90).Info("All worker nodes ready for SRIOV")

	return nil
}

// discoverInterfaceName discovers the actual interface name on a node by matching Vendor and DeviceID.
func discoverInterfaceName(apiClient *clients.Settings, nodeName, sriovOpNs, vendor, deviceID string) (string, error) {
	nodeState := sriov.NewNetworkNodeStateBuilder(apiClient, nodeName, sriovOpNs)
	if err := nodeState.Discover(); err != nil {
		return "", fmt.Errorf("failed to discover node state for node %q: %w", nodeName, err)
	}

	if nodeState.Objects == nil || nodeState.Objects.Status.Interfaces == nil {
		return "", fmt.Errorf("node state has no interfaces for node %q", nodeName)
	}

	// Find interface matching vendor and deviceID
	for _, iface := range nodeState.Objects.Status.Interfaces {
		if iface.Vendor == vendor && iface.DeviceID == deviceID {
			klog.V(90).Infof("Found interface %q on node %q matching vendor %q and deviceID %q",
				iface.Name, nodeName, vendor, deviceID)

			return iface.Name, nil
		}
	}

	return "", fmt.Errorf("no interface found on node %q matching vendor %q and deviceID %q", nodeName, vendor, deviceID)
}

// tryCreatePolicyOnNode attempts to create and apply a SRIOV policy on a single node.
// Returns true if successful, false with error message if failed.
func tryCreatePolicyOnNode(
	apiClient *clients.Settings,
	node *nodes.Builder,
	name, deviceID, interfaceName, vendor, sriovOpNs, devType string,
	vfNum int,
) (bool, string) {
	nodeName := node.Definition.Name
	actualInterfaceName := interfaceName

	// Discover actual interface name if vendor and deviceID are provided
	if vendor != "" && deviceID != "" {
		if discovered, err := discoverInterfaceName(
			apiClient, nodeName, sriovOpNs, vendor, deviceID); err == nil {
			actualInterfaceName = discovered
		}
	}

	pfSelector := fmt.Sprintf("%s#0-%d", actualInterfaceName, vfNum-1)
	klog.V(90).Infof("Creating SRIOV policy - name: %q, node: %q, pf: %q", name, nodeName, pfSelector)

	// Create SRIOV policy
	sriovPolicy := sriov.NewPolicyBuilder(
		apiClient, name, sriovOpNs, name, vfNum,
		[]string{pfSelector},
		map[string]string{"kubernetes.io/hostname": nodeName},
	).WithDevType(devType)

	if vendor != "" {
		sriovPolicy.Definition.Spec.NicSelector.Vendor = vendor
	}

	if deviceID != "" {
		sriovPolicy.Definition.Spec.NicSelector.DeviceID = deviceID
	}

	if _, err := sriovPolicy.Create(); err != nil {
		_ = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.DefaultTimeout)

		return false, fmt.Sprintf("%s (create failed: %v)", nodeName, err)
	}

	// Wait for policy to be applied
	// Use worker label from global config if available, otherwise default to empty (standard worker label)
	workerLabel := ""
	if ocpsriovinittools.SriovOcpConfig != nil {
		workerLabel = ocpsriovinittools.SriovOcpConfig.OcpWorkerLabel
	}

	if err := WaitForSriovAndMCPStable(
		apiClient, tsparams.PolicyApplicationTimeout,
		tsparams.MCPStableInterval, DefaultMcpLabel, sriovOpNs, workerLabel); err != nil {
		_ = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.DefaultTimeout)

		return false, fmt.Sprintf("%s (apply wait failed: %v)", nodeName, err)
	}

	klog.V(90).Infof("SRIOV policy applied - name: %q, node: %q", name, nodeName)

	return true, ""
}

// initVFWithDevType is a common helper function for initializing VF with a specific device type.
func initVFWithDevType(
	apiClient *clients.Settings,
	_ *sriovconfig.SriovOcpConfig, // config reserved for future use
	name, deviceID, interfaceName, vendor, sriovOpNs, devType string,
	vfNum int,
	workerNodes []*nodes.Builder,
) (bool, error) {
	if vfNum <= 0 {
		return false, fmt.Errorf("vfNum must be greater than 0, got %d", vfNum)
	}

	klog.V(90).Infof("Initializing VF for device %q (vfNum: %d, devType: %q)",
		name, vfNum, devType)

	if err := VerifyWorkerNodesReady(apiClient, workerNodes, sriovOpNs); err != nil {
		return false, fmt.Errorf("worker nodes not ready: %w", err)
	}

	// Clean up any existing policy
	_ = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.NamespaceTimeout)

	// Try each worker node
	var failedNodes []string

	for _, node := range workerNodes {
		success, errMsg := tryCreatePolicyOnNode(
			apiClient, node, name, deviceID, interfaceName, vendor, sriovOpNs, devType, vfNum)
		if success {
			return true, nil
		}

		if errMsg != "" {
			failedNodes = append(failedNodes, errMsg)
		}
	}

	if len(failedNodes) > 0 {
		return false, fmt.Errorf("failed to create SRIOV policy %q. Failed nodes: %v",
			name, failedNodes)
	}

	return false, fmt.Errorf("failed to create SRIOV policy %q - no nodes attempted", name)
}

// InitVF initializes VF for the given device.
func InitVF(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, deviceID, interfaceName, vendor, sriovOpNs string,
	vfNum int,
	workerNodes []*nodes.Builder,
) (bool, error) {
	return initVFWithDevType(
		apiClient, config, name, deviceID, interfaceName, vendor,
		sriovOpNs, "netdevice", vfNum, workerNodes)
}

// CreateTestPod creates a test pod with SRIOV network.
func CreateTestPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, namespace, networkName, ipAddress, macAddress string) (*pod.Builder, error) {
	if config == nil {
		return nil, fmt.Errorf("SriovOcpConfig cannot be nil")
	}

	klog.V(90).Infof("Creating test pod %q in namespace %q with network %q (ip: %q, mac: %q)",
		name, namespace, networkName, ipAddress, macAddress)

	// Create network annotation
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(networkName, []string{ipAddress}, macAddress)

	podBuilder := pod.NewBuilder(
		apiClient,
		name,
		namespace,
		config.OcpSriovTestContainer,
	).WithPrivilegedFlag().WithSecondaryNetwork(networkAnnotation)

	createdPod, err := podBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create test pod %q: %w", name, err)
	}

	klog.V(90).Infof("Test pod %q created successfully", name)

	return createdPod, nil
}

// VerifyInterfaceReady verifies that a pod's network interface is ready.
func VerifyInterfaceReady(podObj *pod.Builder, interfaceName, podName string) error {
	klog.V(90).Infof("Verifying interface %q is ready on pod %q", interfaceName, podName)

	cmd := []string{"ip", "link", "show", interfaceName}
	output, err := podObj.ExecCommand(cmd)

	// Handle cases where pod is not accessible (already deleted, terminating, etc.)
	if err != nil {
		// Check if it's a network error (pod inaccessible)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			klog.V(90).Infof("Pod %q not accessible (may be terminating or deleted): %v", podName, err)

			return fmt.Errorf("pod %q is not accessible - likely being terminated or already deleted: %w", podName, err)
		}

		return fmt.Errorf("failed to get interface status for pod %q: %w", podName, err)
	}

	// Check if interface is UP
	outputStr := output.String()
	if !strings.Contains(outputStr, "UP") || strings.Contains(outputStr, "DOWN") {
		return fmt.Errorf("interface %q is not UP on pod %q (output: %q)", interfaceName, podName, outputStr)
	}

	klog.V(90).Infof("Interface %q is ready on pod %q", interfaceName, podName)

	return nil
}

// CheckInterfaceCarrier checks if interface has carrier (physical link is active).
func CheckInterfaceCarrier(podObj *pod.Builder, interfaceName string) (bool, error) {
	klog.V(90).Infof("Checking carrier status for interface %q", interfaceName)

	cmd := []string{"ip", "link", "show", interfaceName}

	output, err := podObj.ExecCommand(cmd)
	if err != nil {
		// Handle cases where pod is not accessible (already deleted, terminating, etc.)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			klog.V(90).Infof("Pod not accessible when checking carrier status (may be terminating): %v", err)

			return false, fmt.Errorf("pod not accessible when checking carrier status: %w", err)
		}

		return false, fmt.Errorf("failed to get interface status: %w", err)
	}

	outputStr := output.String()

	// Check for NO-CARRIER flag
	if strings.Contains(outputStr, "NO-CARRIER") {
		klog.V(90).Infof("Interface %q has NO-CARRIER status", interfaceName)

		return false, nil
	}

	klog.V(90).Infof("Interface %q carrier is active", interfaceName)

	return true, nil
}

// ExtractPodInterfaceMAC extracts the MAC address from a pod's interface.
func ExtractPodInterfaceMAC(podObj *pod.Builder, interfaceName string) (string, error) {
	klog.V(90).Infof("Extracting MAC address from interface %q", interfaceName)

	cmd := []string{"ip", "link", "show", interfaceName}

	output, err := podObj.ExecCommand(cmd)
	if err != nil {
		// Handle cases where pod is not accessible (already deleted, terminating, etc.)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			klog.V(90).Infof("Pod not accessible when extracting MAC (may be terminating): %v", err)

			return "", fmt.Errorf("pod not accessible: %w", err)
		}

		return "", fmt.Errorf("failed to get interface info: %w", err)
	}

	outputStr := output.String()
	lines := strings.Split(outputStr, "\n")

	// Look for MAC address in output
	// Format typically: "link/ether XX:XX:XX:XX:XX:XX"
	for _, line := range lines {
		if strings.Contains(line, "link/ether") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "link/ether" && i+1 < len(parts) {
					mac := parts[i+1]
					klog.V(90).Infof("Extracted MAC address %q from interface %q", mac, interfaceName)

					return mac, nil
				}
			}
		}
	}

	return "", fmt.Errorf("MAC address not found for interface %q", interfaceName)
}

// k8sDNSNameMaxLength is the maximum length for Kubernetes resource names per RFC 1123.
const k8sDNSNameMaxLength = 63

// debugPodConfig holds configuration for creating debug pods.
type debugPodConfig struct {
	namePrefix     string
	cleanupTimeout time.Duration
	maxNameLength  int
}

// defaultDebugPodConfig returns the default debug pod configuration.
func defaultDebugPodConfig() debugPodConfig {
	return debugPodConfig{
		namePrefix:     "sriov-debug-",
		cleanupTimeout: 15 * time.Second,
		maxNameLength:  k8sDNSNameMaxLength,
	}
}

// createDebugPod creates a privileged debug pod on the specified node.
func createDebugPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	podName, namespace, nodeName string,
) (*pod.Builder, error) {
	debugPod := pod.NewBuilder(
		apiClient, podName, namespace, config.OcpSriovTestContainer,
	).WithPrivilegedFlag().
		WithHostNetwork().
		WithHostPid(true).
		WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName}).
		WithRestartPolicy(corev1.RestartPolicyNever)

	return debugPod.Create()
}

// waitForDebugPodRunning waits for the debug pod to reach Running state.
func waitForDebugPodRunning(
	apiClient *clients.Settings,
	podName, namespace string,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			pulledPod, pullErr := pod.Pull(apiClient, podName, namespace)
			if pullErr != nil || pulledPod == nil || pulledPod.Object == nil {
				return false, nil
			}

			return pulledPod.Object.Status.Phase == corev1.PodRunning, nil
		})
}

// executeCommandOnNode executes a command on a specific node using a privileged debug pod.
// This creates a temporary privileged pod with host network access to execute the command.
func executeCommandOnNode(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	nodeName, namespace string,
	cmd []string,
	timeout time.Duration,
) (string, error) {
	cfg := defaultDebugPodConfig()

	debugPodNamespace := namespace
	if debugPodNamespace == "" {
		debugPodNamespace = tsparams.TestNamespaceName
	}

	// Generate unique pod name
	debugPodName := cfg.namePrefix + strings.ToLower(strings.ReplaceAll(nodeName, ".", "-"))
	if len(debugPodName) > cfg.maxNameLength {
		debugPodName = debugPodName[:cfg.maxNameLength]
	}

	// Clean up any existing debug pod
	if existingPod, err := pod.Pull(apiClient, debugPodName, debugPodNamespace); err == nil && existingPod != nil {
		_, _ = existingPod.DeleteAndWait(cfg.cleanupTimeout)
	}

	createdPod, err := createDebugPod(apiClient, config, debugPodName, debugPodNamespace, nodeName)
	if err != nil {
		return "", fmt.Errorf("failed to create debug pod on node %q: %w", nodeName, err)
	}

	defer func() {
		if createdPod != nil {
			if _, err := createdPod.DeleteAndWait(cfg.cleanupTimeout); err != nil {
				klog.V(90).Infof("Failed to cleanup debug pod %q: %v", debugPodName, err)

				_, _ = createdPod.Delete()
			}
		}
	}()

	if err := waitForDebugPodRunning(apiClient, debugPodName, debugPodNamespace, timeout); err != nil {
		return "", fmt.Errorf("debug pod did not reach Running state on node %q: %w", nodeName, err)
	}

	runningPod, err := pod.Pull(apiClient, debugPodName, debugPodNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull running debug pod: %w", err)
	}

	// Build nsenter command
	nsenterCmd := append([]string{
		"nsenter", "--target", "1",
		"--mount", "--uts", "--ipc", "--net", "--pid", "--",
	}, cmd...)

	output, err := runningPod.ExecCommand(nsenterCmd)
	if err != nil {
		return "", fmt.Errorf("failed to execute command on node %q: %w", nodeName, err)
	}

	return output.String(), nil
}

// PrepareVFSpoofCheckVerification verifies the actual spoof checking state on the node.
// This function performs real verification by executing commands on the node using a privileged debug pod.
//
// Implementation details:
// - Creates a temporary privileged debug pod on the target node
// - Uses nsenter to access the host namespace (similar to "oc debug node")
// - Executes: ip link show <nicName> with safe argument passing (no shell interpolation)
// - Filters output in Go to find lines containing <macAddress> (no shell grep)
// - Verifies that the output contains the expected spoof checking state
//
// Parameters:
//   - apiClient: Kubernetes API client
//   - config: SR-IOV configuration (for container image)
//   - nodeName: Name of the node where the VF is located
//   - namespace: Namespace where the debug pod should be created (typically the per-test namespace)
//   - nicName: Name of the network interface (PF name)
//   - podMAC: MAC address of the pod's VF interface
//   - expectedState: Expected spoof checking state ("on" or "off")
//
// Returns error if:
//   - Input validation fails
//   - Debug pod creation fails
//   - Command execution fails
//   - Actual spoof checking state does not match expected state
func PrepareVFSpoofCheckVerification(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	nodeName, namespace, nicName, podMAC, expectedState string) error {
	klog.V(90).Infof("Verifying spoof checking state on node %q for MAC %q (interface: %q, expected: %q)",
		nodeName, podMAC, nicName, expectedState)

	// Validate inputs
	if apiClient == nil {
		return fmt.Errorf("API client cannot be nil")
	}

	if config == nil {
		return fmt.Errorf("SR-IOV config cannot be nil")
	}

	if nodeName == "" {
		return fmt.Errorf("node name should not be empty")
	}

	if nicName == "" {
		return fmt.Errorf("interface name should not be empty")
	}

	if podMAC == "" {
		return fmt.Errorf("pod MAC should not be empty")
	}

	if expectedState != "on" && expectedState != "off" {
		return fmt.Errorf("expected state must be 'on' or 'off', got: %q", expectedState)
	}

	// Validate namespace
	if namespace == "" {
		return fmt.Errorf("namespace should not be empty")
	}

	// Execute command on node to get VF information
	// Use safe argument passing to avoid command injection:
	// - Call "ip link show" with nicName as a separate argument (no shell interpolation)
	// - Filter the output in Go to search for podMAC
	// This eliminates the risk of command injection from untrusted nicName or podMAC values
	cmd := []string{"ip", "link", "show", nicName}

	output, err := executeCommandOnNode(apiClient, config, nodeName, namespace, cmd, tsparams.PodReadyTimeout)
	if err != nil {
		return fmt.Errorf("failed to execute command on node %q: %w", nodeName, err)
	}

	klog.V(90).Infof("Command output from node %q: %q", nodeName, output)

	// Filter output in Go to find lines containing the pod's MAC address
	// This is safer than using shell grep with string interpolation
	lines := strings.Split(output, "\n")

	var matchingLines []string

	for _, line := range lines {
		if strings.Contains(line, podMAC) {
			matchingLines = append(matchingLines, line)
		}
	}

	if len(matchingLines) == 0 {
		return fmt.Errorf("no VF found with MAC address %q in output from node %q for interface %q. "+
			"Full output: %q", podMAC, nodeName, nicName, output)
	}

	// Use the first matching line for verification
	output = strings.Join(matchingLines, "\n")
	klog.V(90).Infof("Filtered output containing MAC %q: %q", podMAC, output)

	// Check if output contains the expected spoof checking state
	// The output format can vary:
	// - "spoof checking on" or "spoof checking off" (with space, as seen in actual output)
	// - "spoofchk on" or "spoofchk off" (abbreviated form)
	// - "spoofchk=on" or "spoofchk=off" (with equals sign)
	expectedPatterns := []string{
		fmt.Sprintf("spoof checking %s", expectedState), // Most common format: "spoof checking on"
		fmt.Sprintf("spoofchk %s", expectedState),       // Abbreviated: "spoofchk on"
		fmt.Sprintf("spoofchk=%s", expectedState),       // With equals: "spoofchk=on"
	}

	found := false

	for _, pattern := range expectedPatterns {
		if strings.Contains(output, pattern) {
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("spoof checking verification failed: expected state %q not found in output. "+
			"Output: %q. Expected patterns: %v", expectedState, output, expectedPatterns)
	}

	klog.V(90).Infof("Spoof checking verification successful: node %q, interface %q, MAC %q, state %q",
		nodeName, nicName, podMAC, expectedState)

	return nil
}

// VerifyVFSpoofCheck is deprecated. Use PrepareVFSpoofCheckVerification instead.
// This function is kept for backward compatibility but will be removed in a future version.
// Note: This deprecated function cannot perform actual verification without apiClient and config.
// It will return an error indicating that the new function signature should be used.
func VerifyVFSpoofCheck(nodeName, nicName, podMAC string) error {
	return fmt.Errorf(
		"VerifyVFSpoofCheck is deprecated. Use PrepareVFSpoofCheckVerification instead")
}

// VerifyLinkStateConfiguration verifies that link state configuration is applied without requiring connectivity.
// This function creates a test pod and verifies that the interface is up with the expected configuration.
// Note: NO-CARRIER status is acceptable for link-state-only checks (returns false, nil) as it indicates
// the configuration is valid but there's no physical connection. This is different from traffic tests
// (CheckVFStatusWithPassTraffic) which require carrier for connectivity testing.
func VerifyLinkStateConfiguration(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	networkName, namespace, description string,
	timeout time.Duration,
) (bool, error) {
	klog.V(90).Infof("Verifying link state: %q (network: %q, ns: %q)",
		description, networkName, namespace)

	// Create a single test pod to verify link state
	testPod, err := CreateTestPod(
		apiClient, config, "linkstate-test", namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return false, fmt.Errorf("failed to create test pod: %w", err)
	}

	// Defer cleanup of pod
	defer func() {
		klog.V(90).Info("Cleaning up link state test pod")

		if testPod != nil {
			_, err := testPod.DeleteAndWait(tsparams.CleanupTimeout)
			if err != nil {
				klog.V(90).Infof("Failed to clean up link state test pod: %v", err)
			}
		}
	}()

	// Wait for pod to be ready
	klog.V(90).Info("Waiting for test pod to be ready")

	err = testPod.WaitUntilReady(timeout)
	if err != nil {
		if testPod.Definition != nil {
			klog.V(90).Infof("Test pod status - phase: %q, reason: %q, message: %q",
				testPod.Definition.Status.Phase, testPod.Definition.Status.Reason, testPod.Definition.Status.Message)
		}

		return false, fmt.Errorf("test pod not ready: %w", err)
	}

	// Verify interface configuration on pod
	klog.V(90).Info("Verifying interface configuration on pod")

	err = VerifyInterfaceReady(testPod, "net1", "linkstate-test")
	if err != nil {
		return false, fmt.Errorf("failed to verify test pod interface: %w", err)
	}

	// Check carrier status to determine if connectivity tests can be run
	klog.V(90).Info("Checking interface carrier status")

	hasCarrier, err := CheckInterfaceCarrier(testPod, "net1")
	if err != nil {
		return false, fmt.Errorf("failed to check interface carrier status: %w", err)
	}

	if !hasCarrier {
		klog.V(90).Info("Interface has NO-CARRIER status - link state configuration is applied but no physical connection")

		return false, nil // No carrier, but configuration is valid
	}

	klog.V(90).Infof("Link state configuration verified successfully with carrier: %q", description)

	return true, nil // Has carrier, connectivity tests can proceed
}

// createAndWaitForTestPods creates client and server test pods and waits for them to be ready.
func createAndWaitForTestPods(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	networkName, namespace string,
	timeout time.Duration,
) (*pod.Builder, *pod.Builder, error) {
	clientPod, err := CreateTestPod(
		apiClient, config, "client", namespace, networkName,
		tsparams.TestPodClientIP, tsparams.TestPodClientMAC)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client pod: %w", err)
	}

	serverPod, err := CreateTestPod(
		apiClient, config, "server", namespace, networkName,
		tsparams.TestPodServerIP, tsparams.TestPodServerMAC)
	if err != nil {
		// Try to clean up client pod if server pod creation fails
		_, _ = clientPod.DeleteAndWait(tsparams.NamespaceTimeout)

		return nil, nil, fmt.Errorf("failed to create server pod: %w", err)
	}

	// Ensure cleanup on any readiness failure
	cleanup := func() {
		if clientPod != nil {
			_, _ = clientPod.DeleteAndWait(tsparams.CleanupTimeout)
		}

		if serverPod != nil {
			_, _ = serverPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}

	// Wait for client pod to be ready
	klog.V(90).Info("Waiting for client pod to be ready")

	err = clientPod.WaitUntilReady(timeout)
	if err != nil {
		if clientPod.Definition != nil {
			klog.V(90).Infof("Client pod status - phase: %q, reason: %q, message: %q",
				clientPod.Definition.Status.Phase, clientPod.Definition.Status.Reason, clientPod.Definition.Status.Message)
		}

		cleanup()

		return nil, nil, fmt.Errorf("client pod not ready: %w", err)
	}

	// Wait for server pod to be ready
	klog.V(90).Info("Waiting for server pod to be ready")

	err = serverPod.WaitUntilReady(timeout)
	if err != nil {
		if serverPod.Definition != nil {
			klog.V(90).Infof("Server pod status - phase: %q, reason: %q, message: %q",
				serverPod.Definition.Status.Phase, serverPod.Definition.Status.Reason, serverPod.Definition.Status.Message)
		}

		cleanup()

		return nil, nil, fmt.Errorf("server pod not ready: %w", err)
	}

	return clientPod, serverPod, nil
}

// isTransientNetworkError checks if an error (including wrapped errors) is a transient network error
// that should be retried. It traverses the error chain using errors.Unwrap() to check all levels.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check current error level
	errStr := err.Error()
	if strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "error dialing backend") {
		return true
	}

	// Unwrap and check nested errors
	unwrapped := errors.Unwrap(err)
	if unwrapped != nil {
		return isTransientNetworkError(unwrapped)
	}

	return false
}

// verifyPodInterfaces verifies that both client and server pod interfaces are ready
// It includes retry logic to handle transient network errors (e.g., kubelet connection issues).
func verifyPodInterfaces(clientPod, serverPod *pod.Builder) error {
	klog.V(90).Info("Verifying interface configuration on pods")

	// Retry logic for client pod interface verification
	var clientErr error

	err := wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.InterfaceVerifyTimeout, // Timeout for interface verification retries
		true,
		func(ctx context.Context) (bool, error) {
			// Check if pod still exists before retrying
			if !clientPod.Exists() {
				return false, fmt.Errorf("client pod no longer exists")
			}

			clientErr = VerifyInterfaceReady(clientPod, "net1", "client")
			if clientErr != nil {
				// Check if it's a transient network error that should be retried
				if isTransientNetworkError(clientErr) {
					klog.V(90).Infof("Transient network error verifying client pod interface, will retry: %v", clientErr)

					return false, nil // Retry
				}
				// Non-retryable error
				return false, clientErr
			}

			return true, nil // Success
		})
	if err != nil {
		if clientErr != nil {
			return fmt.Errorf("failed to verify client pod interface: %w", clientErr)
		}

		return fmt.Errorf("failed to verify client pod interface: %w", err)
	}

	// Retry logic for server pod interface verification
	var serverErr error

	err = wait.PollUntilContextTimeout(
		context.Background(),
		tsparams.PollingInterval,
		tsparams.InterfaceVerifyTimeout, // Timeout for interface verification retries
		true,
		func(ctx context.Context) (bool, error) {
			// Check if pod still exists before retrying
			if !serverPod.Exists() {
				return false, fmt.Errorf("server pod no longer exists")
			}

			serverErr = VerifyInterfaceReady(serverPod, "net1", "server")
			if serverErr != nil {
				// Check if it's a transient network error that should be retried
				if isTransientNetworkError(serverErr) {
					klog.V(90).Infof("Transient network error verifying server pod interface, will retry: %v", serverErr)

					return false, nil // Retry
				}
				// Non-retryable error
				return false, serverErr
			}

			return true, nil // Success
		})
	if err != nil {
		if serverErr != nil {
			return fmt.Errorf("failed to verify server pod interface: %w", serverErr)
		}

		return fmt.Errorf("failed to verify server pod interface: %w", err)
	}

	return nil
}

// verifyPodCarrier checks if the client pod interface has carrier and returns an error if NO-CARRIER.
// This is used by traffic tests (CheckVFStatusWithPassTraffic) where NO-CARRIER means the test should
// be skipped rather than failed. Callers should interpret this error as a skip condition for interfaces
// without physical connection, not as a generic failure.
func verifyPodCarrier(clientPod *pod.Builder) error {
	klog.V(90).Info("Checking interface link status")

	clientCarrier, err := CheckInterfaceCarrier(clientPod, "net1")
	if err != nil {
		return fmt.Errorf("failed to check client interface carrier status: %w", err)
	}

	if !clientCarrier {
		klog.V(90).Info("Interface has NO-CARRIER status (physical link down)")

		return fmt.Errorf("interface has NO-CARRIER status - no physical connection")
	}

	return nil
}

// verifySpoofCheckOnPod verifies the actual spoof checking state on the node for the VF.
// This function performs real verification by executing commands on the node.
func verifySpoofCheckOnPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	clientPod *pod.Builder,
	interfaceName, expectedState string) error {
	klog.V(90).Infof("Verifying spoof checking state on VF (expected: %q)", expectedState)
	// Refresh pod definition to get the latest node name after it was scheduled
	refreshedPod, err := pod.Pull(apiClient, clientPod.Definition.Name, clientPod.Definition.Namespace)
	if err != nil {
		return fmt.Errorf("failed to refresh client pod definition: %w", err)
	}

	clientPodNode := refreshedPod.Definition.Spec.NodeName
	if clientPodNode == "" {
		return fmt.Errorf("client pod node name should not be empty after scheduling")
	}

	klog.V(90).Infof("Client pod is running on node %q", clientPodNode)

	// Extract client pod's MAC address from net1 interface
	clientMAC, err := ExtractPodInterfaceMAC(clientPod, "net1")
	if err != nil {
		// Check if it's a network error (pod inaccessible)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "pod not accessible") {
			klog.V(90).Infof("Pod not accessible when extracting MAC (may be terminating): %v", err)

			return fmt.Errorf("pod is not accessible - likely being terminated or already deleted: %w", err)
		}

		return fmt.Errorf("failed to extract client pod MAC address: %w", err)
	}

	klog.V(90).Infof("Client pod MAC address extracted: %q", clientMAC)

	// Get the namespace from the client pod
	clientPodNamespace := refreshedPod.Definition.Namespace
	if clientPodNamespace == "" {
		return fmt.Errorf("client pod namespace should not be empty")
	}

	// Verify actual spoof checking state on the node
	err = PrepareVFSpoofCheckVerification(
		apiClient, config, clientPodNode, clientPodNamespace,
		interfaceName, clientMAC, expectedState)
	if err != nil {
		return fmt.Errorf("failed to verify VF spoof checking state: %w", err)
	}

	return nil
}

// testPodConnectivity tests connectivity between client and server pods using ping.
func testPodConnectivity(clientPod *pod.Builder, serverIP string) error {
	klog.V(90).Info("Testing connectivity between pods")

	pingCmd := []string{"ping", "-c", "3", serverIP}
	pingTimeout := tsparams.PingTimeout

	var pingOutput bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(
		ctx,
		tsparams.PingPollingInterval,
		pingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var execErr error

			pingOutput, execErr = clientPod.ExecCommand(pingCmd)
			if execErr != nil {
				errMsg := execErr.Error()
				// Check for permanent pod errors that shouldn't be retried
				if strings.Contains(errMsg, "not found") ||
					strings.Contains(errMsg, "does not exist") ||
					strings.Contains(errMsg, "pod is not running") {
					return false, fmt.Errorf("client pod unavailable: %w", execErr)
				}

				klog.V(90).Infof("Ping command failed, will retry: %v (output: %q)", execErr, pingOutput.String())

				return false, nil // Retry on transient error
			}

			return true, nil // Success
		})
	if err != nil {
		return fmt.Errorf("ping command timed out or failed after %v: %w", pingTimeout, err)
	}

	if pingOutput.Len() == 0 {
		return fmt.Errorf("ping command returned empty output")
	}

	pingOutputStr := pingOutput.String()

	// Check for explicit failure indicators (100% packet loss indicates complete failure)
	hasCompleteFailure := strings.Contains(pingOutputStr, "100% packet loss")
	if hasCompleteFailure {
		return fmt.Errorf("ping failed with 100%% packet loss (output: %q)", pingOutputStr)
	}

	// Check for any packet loss (not just 100%) - for SR-IOV tests, any loss may indicate a problem
	hasPartialLoss := strings.Contains(pingOutputStr, "packet loss") &&
		!strings.Contains(pingOutputStr, "0% packet loss")
	if hasPartialLoss {
		return fmt.Errorf("ping experienced partial packet loss (output: %q)", pingOutputStr)
	}

	// Check for ping success indicators (more robust than just checking "3 packets transmitted")
	hasSuccessIndicator := strings.Contains(pingOutputStr, "3 packets transmitted") ||
		strings.Contains(pingOutputStr, "3 received") ||
		strings.Contains(pingOutputStr, "0% packet loss")

	if !hasSuccessIndicator {
		return fmt.Errorf("ping did not complete successfully - no success indicators found (output: %q)", pingOutputStr)
	}

	return nil
}

// CheckVFStatusWithPassTraffic checks VF status and passes traffic between test pods
// This function orchestrates pod creation, interface verification, carrier checking,
// spoof check verification, and connectivity testing using helper functions.
func CheckVFStatusWithPassTraffic(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	networkName, interfaceName, namespace, description string,
	timeout time.Duration,
) error {
	klog.V(90).Infof("Checking VF status: %q (network: %q, iface: %q, ns: %q)",
		description, networkName, interfaceName, namespace)

	// Create and wait for test pods
	clientPod, serverPod, err := createAndWaitForTestPods(apiClient, config, networkName, namespace, timeout)
	if err != nil {
		return err
	}

	// Defer cleanup of pods
	defer func() {
		klog.V(90).Info("Cleaning up test pods")

		if clientPod != nil {
			_, err := clientPod.DeleteAndWait(tsparams.CleanupTimeout)
			if err != nil {
				klog.V(90).Infof("Failed to clean up client pod: %v", err)
			}
		}

		if serverPod != nil {
			_, err := serverPod.DeleteAndWait(tsparams.CleanupTimeout)
			if err != nil {
				klog.V(90).Infof("Failed to clean up server pod: %v", err)
			}
		}
	}()

	// Verify interface configuration on pods
	if err := verifyPodInterfaces(clientPod, serverPod); err != nil {
		return err
	}

	// Check for NO-CARRIER status
	// Note: verifyPodCarrier returns an error for NO-CARRIER, which should be interpreted as
	// a skip condition (interface without physical connection) rather than a test failure.
	// This allows traffic tests to be skipped gracefully when there's no physical link.
	if err := verifyPodCarrier(clientPod); err != nil {
		// Check if this is a NO-CARRIER error (skip condition) vs other errors (failures)
		if strings.Contains(err.Error(), "NO-CARRIER") || strings.Contains(err.Error(), "no physical connection") {
			klog.V(90).Infof("Skipping traffic test due to NO-CARRIER status: %v", err)

			return fmt.Errorf("skipping traffic test - interface has NO-CARRIER status (no physical connection): %w", err)
		}

		return err
	}

	// Verify spoof checking on VF (if description mentions spoof checking)
	// Extract expected state from description (e.g., "spoof checking on" -> "on", "spoof checking off" -> "off")
	if strings.Contains(description, "spoof checking") {
		expectedState := "on"
		if strings.Contains(description, "spoof checking off") {
			expectedState = "off"
		}

		if err := verifySpoofCheckOnPod(apiClient, config, clientPod, interfaceName, expectedState); err != nil {
			return err
		}
	}

	// Test connectivity between pods
	// Extract IP from TestPodServerIP (remove /24 CIDR notation)
	serverIP := strings.Split(tsparams.TestPodServerIP, "/")[0]
	if err := testPodConnectivity(clientPod, serverIP); err != nil {
		return err
	}

	klog.V(90).Infof("VF status verification with traffic completed successfully: %q", description)

	return nil
}

// InitDpdkVF initializes DPDK VF for the given device.
func InitDpdkVF(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, deviceID, interfaceName, vendor, sriovOpNs string,
	vfNum int,
	workerNodes []*nodes.Builder,
) (bool, error) {
	return initVFWithDevType(
		apiClient, config, name, deviceID, interfaceName, vendor,
		sriovOpNs, "vfio-pci", vfNum, workerNodes)
}

// GetPciAddress gets the PCI address for a pod from network status annotation.
// config parameter is reserved for future use and currently unused.
func GetPciAddress(
	apiClient *clients.Settings,
	_ *sriovconfig.SriovOcpConfig, // config reserved for future use
	namespace, podName, policyName string) (string, error) {
	klog.V(90).Infof("Getting PCI address for pod %q in namespace %q (policy: %q)", podName, namespace, policyName)

	// Pull the pod to get the latest annotations
	podObj, err := pod.Pull(apiClient, podName, namespace)
	if err != nil {
		return "", fmt.Errorf("pod %q does not exist or failed to pull in namespace %q: %w", podName, namespace, err)
	}

	if podObj == nil || podObj.Object == nil {
		return "", fmt.Errorf("pod object is nil for pod %q", podName)
	}

	// Get the network status annotation
	networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"

	podNetAnnotation := podObj.Object.Annotations[networkStatusAnnotation]
	if podNetAnnotation == "" {
		klog.V(90).Infof("Pod network annotation not found for pod %q in namespace %q", podName, namespace)

		return "", fmt.Errorf("pod %q does not have network status annotation", podName)
	}

	// Parse the network status annotation
	type PodNetworkStatusAnnotation struct {
		Name       string `json:"name"`
		Interface  string `json:"interface"`
		DeviceInfo struct {
			Type    string `json:"type"`
			Version string `json:"version"`
			Pci     struct {
				PciAddress string `json:"pci-address"`
			} `json:"pci"`
		} `json:"device-info,omitempty"`
	}

	var annotation []PodNetworkStatusAnnotation

	err = json.Unmarshal([]byte(podNetAnnotation), &annotation)
	if err != nil {
		klog.V(90).Infof("Failed to unmarshal pod network status for pod %q: %v", podName, err)

		return "", fmt.Errorf("failed to unmarshal pod network status annotation: %w", err)
	}

	// Find the network matching the policy name.
	// Network name format is typically "namespace/network-name".
	// We check for exact match on the network name part (after the slash) to avoid
	// accidental matches when one policy name is a substring of another.
	for _, networkAnnotation := range annotation {
		networkName := networkAnnotation.Name
		// Extract just the network name if it includes namespace prefix
		if idx := strings.LastIndex(networkName, "/"); idx >= 0 {
			networkName = networkName[idx+1:]
		}

		// Use exact match on network name
		if networkName == policyName {
			if networkAnnotation.DeviceInfo.Pci.PciAddress != "" {
				klog.V(90).Infof("PCI address found for pod %q: %q (network: %q)",
					podName, networkAnnotation.DeviceInfo.Pci.PciAddress, networkAnnotation.Name)

				return networkAnnotation.DeviceInfo.Pci.PciAddress, nil
			}
		}
	}

	klog.V(90).Infof("PCI address not found for pod %q with network %q", podName, policyName)

	return "", fmt.Errorf("PCI address not found for pod %q with network %q", podName, policyName)
}

// UpdateSriovPolicyMTU updates the MTU value of an existing SR-IOV policy
// using the eco-goinfra PolicyBuilder Update() helper.
func UpdateSriovPolicyMTU(apiClient *clients.Settings, policyName, sriovOpNs string, mtuValue int) error {
	klog.V(90).Infof("Updating SR-IOV policy %q MTU to %d in namespace %q", policyName, mtuValue, sriovOpNs)

	// Validate MTU range (1-9192 as per SR-IOV spec)
	if mtuValue < 1 || mtuValue > 9192 {
		return fmt.Errorf("invalid MTU value %d, must be in range 1-9192", mtuValue)
	}

	// Pull the existing policy
	policyBuilder, err := sriov.PullPolicy(apiClient, policyName, sriovOpNs)
	if err != nil {
		return fmt.Errorf("failed to pull SR-IOV policy %q: %w", policyName, err)
	}

	if policyBuilder == nil || policyBuilder.Object == nil {
		return fmt.Errorf("SR-IOV policy %q not found in namespace %q", policyName, sriovOpNs)
	}

	// Update the MTU in the policy definition
	policyBuilder.Object.Spec.Mtu = mtuValue
	policyBuilder.Definition.Spec.Mtu = mtuValue

	// Update the policy using eco-goinfra PolicyBuilder.Update() method
	updatedPolicy, err := policyBuilder.Update(false)
	if err != nil {
		return fmt.Errorf("failed to update SR-IOV policy %q with MTU %d: %w", policyName, mtuValue, err)
	}

	_ = updatedPolicy // Acknowledge successful update

	klog.V(90).Infof("SR-IOV policy %q successfully updated with MTU %d", policyName, mtuValue)

	return nil
}

// CreateDpdkTestPod creates a DPDK test pod with SR-IOV network.
func CreateDpdkTestPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, namespace, networkName string) (*pod.Builder, error) {
	klog.V(90).Infof("Creating DPDK test pod %q in namespace %q with network %q", name, namespace, networkName)

	// Create network annotation (DPDK pods use the network name directly)
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(
		networkName, []string{tsparams.TestPodClientIP}, tsparams.TestPodClientMAC)

	podBuilder := pod.NewBuilder(
		apiClient,
		name,
		namespace,
		config.OcpSriovTestContainer,
	).WithPrivilegedFlag().WithSecondaryNetwork(networkAnnotation).WithLabel("name", "sriov-dpdk")

	createdPod, err := podBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create DPDK test pod %q: %w", name, err)
	}

	klog.V(90).Infof("DPDK test pod %q created successfully", name)

	return createdPod, nil
}

// DeleteDpdkTestPod deletes a DPDK test pod.
func DeleteDpdkTestPod(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	klog.V(90).Infof("Deleting DPDK test pod %q from namespace %q", name, namespace)

	podBuilder, err := pod.Pull(apiClient, name, namespace)
	if err != nil || podBuilder == nil {
		klog.V(90).Infof("DPDK test pod %q does not exist in namespace %q, skipping deletion", name, namespace)

		return nil
	}

	_, err = podBuilder.DeleteAndWait(timeout)
	if err != nil {
		return fmt.Errorf("failed to delete DPDK test pod %q: %w", name, err)
	}

	klog.V(90).Infof("DPDK test pod %q successfully deleted", name)

	return nil
}
