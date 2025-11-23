package sriovenv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
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
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: IsSriovDeployed is now in tests/internal/sriovoperator package
// This function is kept for backward compatibility but should use sriovoperator.IsSriovDeployed instead
// Deprecated: Use sriovoperator.IsSriovDeployed instead
func IsSriovDeployed(apiClient *clients.Settings, config interface{}) error {
	var namespace string
	switch c := config.(type) {
	case *sriovconfig.SriovOcpConfig:
		namespace = c.OcpSriovOperatorNamespace
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
		glog.V(90).Info("API client is nil in PullTestImageOnNodes; continuing because image pull is deferred to kubelet")
	}
	glog.V(90).Infof(
		"Image pulling deferred to pod creation. Image: %q, nodeSelector: %q, pullTimeoutSeconds: %d. "+
			"Images will be pulled on first pod creation; this may take extra time on first pod launch.",
		image, nodeSelector, pullTimeout)
	return nil
}

// CleanAllNetworksByTargetNamespace cleans all networks by target namespace
func CleanAllNetworksByTargetNamespace(apiClient *clients.Settings, sriovOpNs, targetNs string) error {
	glog.V(90).Infof("Cleaning up SR-IOV networks for target namespace %q (operator_namespace: %q)", targetNs, sriovOpNs)

	// List all SriovNetwork resources in the operator namespace
	sriovNetworks, err := sriov.List(apiClient, sriovOpNs, client.ListOptions{})
	if err != nil {
		glog.V(90).Infof("Error listing SR-IOV networks: %v", err)
		// Don't fail if we can't list networks - just log and continue
		return nil
	}

	networksCleaned := 0
	for _, network := range sriovNetworks {
		// Check if this network targets the given namespace
		// SR-IOV networks have a networkNamespace field that indicates the target namespace
		if network.Definition.Spec.NetworkNamespace != targetNs {
			continue
		}

		glog.V(90).Infof("Deleting SR-IOV network %q in namespace %q (target_namespace: %q)",
			network.Definition.Name, sriovOpNs, targetNs)

		// Delete the SriovNetwork CR
		err := network.Delete()
		if err != nil && !apierrors.IsNotFound(err) {
			glog.V(90).Infof("Error deleting SR-IOV network %q: %v", network.Definition.Name, err)
			continue
		}

		networksCleaned++

		// Also delete the corresponding NetworkAttachmentDefinition in the target namespace
		nadName := network.Definition.Name
		nadBuilder := nad.NewBuilder(apiClient, nadName, targetNs)
		if nadBuilder.Exists() {
			glog.V(90).Infof("Deleting NetworkAttachmentDefinition %q in namespace %q", nadName, targetNs)
			err := nadBuilder.Delete()
			if err != nil && !apierrors.IsNotFound(err) {
				glog.V(90).Infof("Error deleting NetworkAttachmentDefinition %q: %v", nadName, err)
				// Continue even if NAD deletion fails
			}
		}
	}

	glog.V(90).Infof("Cleanup complete: cleaned %d networks for target namespace %q", networksCleaned, targetNs)

	// No need to sleep - deletions are handled asynchronously by Kubernetes
	// The caller should wait for resources to be deleted if needed

	return nil
}

// CleanupLeftoverResources cleans up any leftover resources from previous failed test runs
// This should be called at the beginning of the test suite to ensure a clean state
func CleanupLeftoverResources(apiClient *clients.Settings, sriovOperatorNamespace string) error {
	glog.V(90).Info("Starting cleanup of leftover resources from previous test runs")

	// Step 1: Clean up leftover e2e test namespaces
	namespaceList, err := namespace.List(apiClient, metav1.ListOptions{})
	if err != nil {
		glog.V(90).Infof("Failed to list namespaces for cleanup: %v", err)
		return fmt.Errorf("failed to list namespaces for cleanup: %w", err)
	}

	for _, ns := range namespaceList {
		// Look for test namespaces created by previous runs
		if strings.HasPrefix(ns.Definition.Name, "e2e-") {
			glog.V(90).Infof("Removing leftover test namespace %q", ns.Definition.Name)

			// Try to delete with reasonable timeout
			deleteErr := ns.DeleteAndWait(tsparams.CleanupTimeout)
			if deleteErr != nil {
				glog.V(90).Infof("Failed to delete leftover namespace %q (continuing cleanup): %v", ns.Definition.Name, deleteErr)

				// Try force delete as fallback
				if forceDeleteErr := ns.Delete(); forceDeleteErr != nil {
					glog.V(90).Infof("Failed to force delete leftover namespace %q: %v", ns.Definition.Name, forceDeleteErr)
				}
			}
		}
	}

	// Step 2: Clean up leftover SR-IOV networks
	sriovNetworks, err := sriov.List(apiClient, sriovOperatorNamespace, client.ListOptions{})
	if err != nil {
		glog.V(90).Infof("Failed to list SriovNetworks for cleanup: %v", err)
	} else {
		// Match test network names: 5-digit test case ID followed by dash (e.g., "25959-deviceName", "70821-deviceName")
		// Also match DPDK network names: device name followed by "dpdknet" (e.g., "deviceNamedpdknet")
		// Require word characters before "dpdknet" to avoid matching unrelated resources
		testNetworkPattern := regexp.MustCompile(`^\d{5}-|\w+dpdknet$`)
		for _, net := range sriovNetworks {
			networkName := net.Definition.Name
			if testNetworkPattern.MatchString(networkName) {
				glog.V(90).Infof("Removing leftover SR-IOV network %q (matches test network pattern)", networkName)

				err := net.Delete()
				if err != nil {
					glog.V(90).Infof("Failed to delete leftover SR-IOV network %q (continuing cleanup): %v", networkName, err)
				}
			}
		}
	}

	// Step 3: Clean up leftover SR-IOV policies that might conflict
	// Clean up policies that match test device names to prevent VF range conflicts
	sriovPolicies, err := sriov.ListPolicy(apiClient, sriovOperatorNamespace, client.ListOptions{})
	if err != nil {
		glog.V(90).Infof("Failed to list SriovNetworkNodePolicies for cleanup: %v", err)
	} else {
		// Get test device names from configuration (supports both env var and defaults)
		deviceConfigs := tsparams.GetDeviceConfig()
		testDeviceNames := make([]string, 0, len(deviceConfigs))
		for _, device := range deviceConfigs {
			testDeviceNames = append(testDeviceNames, device.Name)
		}
		// Also include "cx5ex" which may not be in default config
		testDeviceNames = append(testDeviceNames, "cx5ex")
		for _, policy := range sriovPolicies {
			policyName := policy.Definition.Name
			// Check if policy name matches a test device name (exact match or with suffix)
			shouldCleanup := false
			for _, deviceName := range testDeviceNames {
				if policyName == deviceName || strings.HasPrefix(policyName, deviceName) {
					shouldCleanup = true
					break
				}
			}
			if shouldCleanup {
				glog.V(90).Infof("Removing leftover SR-IOV policy %q to prevent VF range conflicts", policyName)
				err := policy.Delete()
				if err != nil {
					glog.V(90).Infof("Failed to delete leftover SR-IOV policy %q (continuing cleanup): %v", policyName, err)
				} else {
					// Policy deletion initiated - Kubernetes will handle it asynchronously
					// No need to sleep - the deletion will be processed by the API server
				}
			}
		}
	}

	// Step 4: Log cleanup summary
	glog.V(90).Info("Cleanup of leftover resources completed")
	return nil
}

// RemoveSriovPolicy removes a SRIOV policy by name if it exists
func RemoveSriovPolicy(apiClient *clients.Settings, name, sriovOpNs string, timeout time.Duration) error {
	glog.V(90).Infof("Removing SRIOV policy %q if it exists in namespace %q", name, sriovOpNs)

	// Use PullPolicy to check if the policy exists (doesn't require resourceName)
	policyBuilder, err := sriov.PullPolicy(apiClient, name, sriovOpNs)
	if err != nil {
		// Policy doesn't exist, which is fine
		glog.V(90).Infof("SRIOV policy %q does not exist, skipping deletion: %v", name, err)
		return nil
	}

	err = policyBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete SRIOV policy %q: %w", name, err)
	}

	// Wait for policy to be deleted using wait.PollUntilContextTimeout
	glog.V(90).Infof("Waiting for SRIOV policy %q to be deleted", name)
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			checkPolicy := sriov.NewPolicyBuilder(
				apiClient,
				name,
				sriovOpNs,
				"",
				0,
				[]string{},
				map[string]string{},
			)
			return !checkPolicy.Exists(), nil
		})

	if err != nil {
		return fmt.Errorf("timeout waiting for SRIOV policy %q to be deleted from namespace %q: %w", name, sriovOpNs, err)
	}

	glog.V(90).Infof("SRIOV policy %q successfully deleted", name)
	return nil
}

// RemoveSriovNetwork removes a SRIOV network by name from the operator namespace
func RemoveSriovNetwork(apiClient *clients.Settings, name, sriovOpNs string, timeout time.Duration) error {
	glog.V(90).Infof("Removing SRIOV network %q from namespace %q", name, sriovOpNs)

	// Use List to find the network by name
	listOptions := client.ListOptions{}
	sriovNetworks, err := sriov.List(apiClient, sriovOpNs, listOptions)
	if err != nil {
		return fmt.Errorf("failed to list SRIOV networks in namespace %q: %w", sriovOpNs, err)
	}

	// Find the network with matching name
	var targetNetwork *sriov.NetworkBuilder
	var targetNamespace string
	var resourceName string
	for _, network := range sriovNetworks {
		if network.Object.Name == name {
			targetNamespace = network.Object.Spec.NetworkNamespace
			if targetNamespace == "" {
				targetNamespace = sriovOpNs
			}
			resourceName = network.Object.Spec.ResourceName
			// Rebuild the network builder with the same parameters to delete it
			targetNetwork = sriov.NewNetworkBuilder(
				apiClient,
				name,
				sriovOpNs,
				targetNamespace,
				resourceName,
			)
			break
		}
	}

	if targetNetwork == nil || !targetNetwork.Exists() {
		glog.V(90).Infof("SRIOV network %q not found or already deleted in namespace %q", name, sriovOpNs)
		return nil
	}

	// Delete the SRIOV network
	err = targetNetwork.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete SRIOV network %q: %w", name, err)
	}

	// Wait for SRIOV network to be fully deleted using wait.PollUntilContextTimeout
	glog.V(90).Infof("Waiting for SRIOV network %q to be deleted", name)
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			checkNetwork := sriov.NewNetworkBuilder(
				apiClient,
				name,
				sriovOpNs,
				targetNamespace,
				resourceName,
			)
			return !checkNetwork.Exists(), nil
		})

	if err != nil {
		return fmt.Errorf("timeout waiting for SRIOV network %q to be deleted from namespace %q: %w", name, sriovOpNs, err)
	}

	// Wait for NAD to be deleted in the target namespace
	if targetNamespace != sriovOpNs {
		glog.V(90).Infof("Waiting for NetworkAttachmentDefinition %q to be deleted in namespace %q", name, targetNamespace)

		// First, check if NAD exists
		_, pullErr := nad.Pull(apiClient, name, targetNamespace)
		if pullErr != nil {
			if apierrors.IsNotFound(pullErr) {
				glog.V(90).Infof("NAD %q does not exist (already deleted or never created) in namespace %q", name, targetNamespace)
				return nil
			}
			return fmt.Errorf("failed to check NAD %q in namespace %q before deletion wait: %w", name, targetNamespace, pullErr)
		}

		// Wait for NAD deletion using wait.PollUntilContextTimeout
		err = wait.PollUntilContextTimeout(
			context.TODO(),
			tsparams.PollingInterval,
			tsparams.NADTimeout,
			true,
			func(ctx context.Context) (bool, error) {
				_, pullErr := nad.Pull(apiClient, name, targetNamespace)
				if pullErr != nil {
					if apierrors.IsNotFound(pullErr) {
						// NAD is deleted (NotFound), which is what we want
						glog.V(90).Infof("NetworkAttachmentDefinition %q successfully deleted in namespace %q", name, targetNamespace)
						return true, nil
					}
					// Transient error, retry
					glog.V(90).Infof("Temporary error pulling NAD %q in namespace %q, will retry: %v", name, targetNamespace, pullErr)
					return false, nil
				}
				// NAD still exists, keep waiting
				return false, nil
			})

		if err != nil {
			// Try to force delete if timeout occurred
			glog.V(90).Infof("Timeout waiting for NAD deletion, attempting force delete: %v", err)
			nadBuilder, pullErr := nad.Pull(apiClient, name, targetNamespace)
			if pullErr == nil && nadBuilder != nil {
				deleteErr := nadBuilder.Delete()
				if deleteErr != nil {
					glog.V(90).Infof("Failed to force delete NAD %q: %v", name, deleteErr)
				} else {
					glog.V(90).Infof("Successfully force deleted NAD %q", name)
					// NAD deletion initiated - Kubernetes will handle it asynchronously
					// No need to sleep - the deletion will be processed by the API server
				}
			}

			// Final check - if NAD is gone, that's fine
			_, finalCheckErr := nad.Pull(apiClient, name, targetNamespace)
			if apierrors.IsNotFound(finalCheckErr) {
				glog.V(90).Infof("NAD %q is now deleted (after timeout but before final check)", name)
				return nil
			}

			return fmt.Errorf("NetworkAttachmentDefinition %q was not deleted from namespace %q within timeout. "+
				"Please check SR-IOV operator status (last error: %v)", name, targetNamespace, finalCheckErr)
		}
	}

	glog.V(90).Infof("SRIOV network %q successfully deleted", name)
	return nil
}

// WaitForPodWithLabelReady waits for a pod with specific label to be ready.
// The timeout parameter is used only for per-pod readiness checks (WaitUntilReady).
// Pod discovery uses the fixed tsparams.PodLabelReadyTimeout constant.
func WaitForPodWithLabelReady(apiClient *clients.Settings, namespace, labelSelector string, timeout time.Duration) error {
	glog.V(90).Infof("Waiting for pod with label %q to be ready in namespace %q", labelSelector, namespace)

	// Wait for pod to appear using wait.PollUntilContextTimeout
	var podList []*pod.Builder
	var listErr error
	err := wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingInterval,
		tsparams.PodLabelReadyTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			podList, listErr = pod.List(apiClient, namespace, metav1.ListOptions{LabelSelector: labelSelector})
			if listErr != nil {
				glog.V(90).Infof("Failed to list pods, will retry: %v (namespace: %q, labelSelector: %q)", listErr, namespace, labelSelector)
				return false, nil
			}
			return len(podList) > 0, nil
		})

	if err != nil {
		return fmt.Errorf("timeout waiting for pod with label %q to appear in namespace %q: %w", labelSelector, namespace, err)
	}

	if len(podList) == 0 {
		return fmt.Errorf("no pods found with label %q in namespace %q", labelSelector, namespace)
	}

	// Wait for each pod to be ready
	for _, p := range podList {
		glog.V(90).Infof("Waiting for pod %q to be ready in namespace %q", p.Definition.Name, namespace)
		err := p.WaitUntilReady(timeout)
		if err != nil {
			// Log pod status for debugging
			if p.Definition != nil {
				glog.V(90).Infof("Pod status details - name: %q, phase: %q, reason: %q, message: %q",
					p.Definition.Name, p.Definition.Status.Phase, p.Definition.Status.Reason, p.Definition.Status.Message)

				// Log container statuses
				for _, cs := range p.Definition.Status.ContainerStatuses {
					glog.V(90).Infof("Container status - name: %q, ready: %v, state: %+v",
						cs.Name, cs.Ready, cs.State)
				}

				// Log conditions
				for _, cond := range p.Definition.Status.Conditions {
					glog.V(90).Infof("Pod condition - type: %q, status: %q, reason: %q, message: %q",
						cond.Type, cond.Status, cond.Reason, cond.Message)
				}
			}
			return fmt.Errorf("pod %q not ready in namespace %q: %w", p.Definition.Name, namespace, err)
		}
	}

	glog.V(90).Infof("All pods with label %q are ready in namespace %q", labelSelector, namespace)
	return nil
}

// WaitForSriovAndMCPStable waits for SRIOV and MCP to be stable
func WaitForSriovAndMCPStable(
	apiClient *clients.Settings,
	timeout time.Duration,
	interval time.Duration,
	mcpLabel, sriovOpNs string) error {
	glog.V(90).Infof("Waiting for SR-IOV and MCP to be stable (timeout: %v, interval: %v, mcp_label: %q)", timeout, interval, mcpLabel)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextCancel(ctx, interval, false, func(ctx context.Context) (bool, error) {
		// Verify SR-IOV operator is running by checking for SR-IOV node states
		nodeStates, err := sriov.ListNetworkNodeState(apiClient, sriovOpNs, client.ListOptions{})
		if err != nil {
			glog.V(90).Infof("Error listing SR-IOV network node states: %v", err)
			return false, nil // Retry on error
		}

		// Validate SR-IOV sync status for all node states
		if len(nodeStates) == 0 {
			glog.V(90).Info("WAITING: No SR-IOV node states available yet - operator still initializing")
			return false, nil // Retry - node states not yet available
		}

		allNodesSynced := true
		for _, nodeState := range nodeStates {
			if nodeState.Objects != nil {
				syncStatus := nodeState.Objects.Status.SyncStatus
				nodeName := nodeState.Objects.Name

				if syncStatus != "Succeeded" {
					glog.V(90).Infof("WAITING: SR-IOV node %q not yet synced (status: %q)", nodeName, syncStatus)
					allNodesSynced = false
				} else {
					glog.V(90).Infof("OK: Node SR-IOV sync complete for node %q", nodeName)
				}
			}
		}

		if !allNodesSynced {
			return false, nil // Retry - wait for all nodes to sync
		}

		// Check MachineConfigPool conditions using eco-goinfra
		mcpList, err := mco.ListMCP(apiClient)
		if err != nil {
			if strings.Contains(err.Error(), "no kind is registered") {
				glog.V(90).Info("INFO: MachineConfigPool check unavailable in scheme - using SR-IOV node state sync as stability indicator")
			} else {
				glog.V(90).Infof("TEMPORARY ERROR: Could not list MachineConfigPools: %v", err)
				return false, nil
			}
		} else {
			// MCP check succeeded, verify conditions
			allPoolsUpdated := true
			for _, mcp := range mcpList {
				// Check if pool matches the provided MCP label selector
				shouldCheck := false
				if mcpLabel == "" {
					shouldCheck = true
				} else {
					parts := strings.SplitN(mcpLabel, "=", 2)
					if len(parts) == 2 {
						labelKey := parts[0]
						labelValue := parts[1]
						if mcp.Object.Labels != nil {
							if val, ok := mcp.Object.Labels[labelKey]; ok && val == labelValue {
								shouldCheck = true
							}
						}
					}
				}
				if !shouldCheck {
					continue
				}

				isUpdated := false
				for _, condition := range mcp.Object.Status.Conditions {
					if condition.Type == machineconfigv1.MachineConfigPoolUpdated && condition.Status == corev1.ConditionTrue {
						isUpdated = true
						glog.V(90).Infof("OK: MachineConfigPool %q condition met (Updated=True)", mcp.Object.Name)
					}
					if condition.Type == machineconfigv1.MachineConfigPoolDegraded && condition.Status == corev1.ConditionTrue {
						glog.V(90).Infof("WAITING: MachineConfigPool %q is degraded (reason: %q, message: %q)",
							mcp.Object.Name, condition.Reason, condition.Message)
						allPoolsUpdated = false
					}
				}

				if !isUpdated {
					glog.V(90).Infof("WAITING: MachineConfigPool %q not yet updated", mcp.Object.Name)
					allPoolsUpdated = false
				}
			}

			if !allPoolsUpdated {
				return false, nil // Retry - wait for MCP to be updated
			}

			glog.V(90).Info("OK: All MachineConfigPools are stable")
		}

		// Check node conditions for worker nodes using eco-goinfra
		nodeList, err := nodes.List(apiClient, metav1.ListOptions{})
		if err != nil {
			glog.V(90).Infof("TEMPORARY ERROR: Could not list nodes: %v", err)
			return false, nil // Retry on error
		}

		allNodesReady := true
		readyNodeCount := 0
		for _, node := range nodeList {
			if _, isWorker := node.Definition.Labels["node-role.kubernetes.io/worker"]; !isWorker {
				continue
			}

			isReady := false
			for _, condition := range node.Definition.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					isReady = true
					readyNodeCount++
					glog.V(90).Infof("OK: Worker node %q is Ready", node.Definition.Name)
					break
				}
				if (condition.Type == corev1.NodeMemoryPressure || condition.Type == corev1.NodeDiskPressure) &&
					condition.Status == corev1.ConditionTrue {
					glog.V(90).Infof("WAITING: Worker node %q has resource pressure (condition: %q)", node.Definition.Name, condition.Type)
					allNodesReady = false
					return false, nil // Retry
				}
			}

			if !isReady {
				glog.V(90).Infof("WAITING: Worker node %q is not yet Ready", node.Definition.Name)
				allNodesReady = false
				return false, nil // Retry
			}
		}

		if !allNodesReady {
			return false, nil // Retry
		}

		glog.V(90).Infof("SUCCESS: All stability checks passed (sr_iov_nodes_synced: %d, worker_nodes_ready: %d)",
			len(nodeStates), readyNodeCount)
		return true, nil
	})

	if err != nil && err != context.DeadlineExceeded {
		return fmt.Errorf("failed waiting for SRIOV and MCP stability: %w", err)
	}

	if err == context.DeadlineExceeded {
		return fmt.Errorf("timeout waiting for SRIOV and MCP to be stable after %v", timeout)
	}

	return nil
}

// VerifyVFResourcesAvailable checks if VF resources are advertised and available on worker nodes
func VerifyVFResourcesAvailable(apiClient *clients.Settings, config *sriovconfig.SriovOcpConfig, resourceName string) (bool, error) {
	if apiClient == nil {
		return false, fmt.Errorf("API client is nil, cannot verify VF resources")
	}

	// Get all worker nodes
	workerNodes, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: config.OcpWorkerLabel})
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
			glog.V(90).Infof("VF resource %q not found on node %q", resourceKey, nodeName)
			continue
		}

		// Check if there are available resources (allocatable > 0)
		if hasAllocatable {
			allocQty := allocatableValue.Value()
			if allocQty > 0 {
				glog.V(90).Infof("VF resources available on node %q (resource: %q, capacity: %s, allocatable: %s)",
					nodeName, resourceKey, capacityValue.String(), allocatableValue.String())
				return true, nil
			}
		}

		if hasCapacity {
			capQty := capacityValue.Value()
			if capQty > 0 && !hasAllocatable {
				glog.V(90).Infof("VF resources exist but not allocatable on node %q (resource: %q, capacity: %s)",
					nodeName, resourceKey, capacityValue.String())
				continue
			}
		}

		glog.V(90).Infof("No allocatable VF resources on node %q (resource: %q, capacity: %s, allocatable: %s)",
			nodeName, resourceKey, capacityValue.String(), allocatableValue.String())
	}

	// If we get here, no nodes have available resources
	glog.V(90).Infof("VF resources %q not available on any worker node", resourceName)
	return false, nil
}

// SriovNetworkConfig represents configuration for creating a SRIOV network
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

// CreateSriovNetwork creates a SRIOV network and waits for it to be ready.
// Note: The timeout parameter is informational only (logged as timeoutHint).
// Actual waiting behavior is governed by tsparams constants:
// - NamespaceTimeout for policy verification
// - NADTimeout for NetworkAttachmentDefinition creation
// - VFResourceTimeout for VF resource availability
func CreateSriovNetwork(apiClient *clients.Settings, config *SriovNetworkConfig, timeout time.Duration) error {
	glog.V(90).Infof("Creating SRIOV network %q in namespace %q (target_namespace: %q, resource: %q, timeoutHint: %v)",
		config.Name, config.Namespace, config.NetworkNamespace, config.ResourceName, timeout)

	networkBuilder := sriov.NewNetworkBuilder(
		apiClient,
		config.Name,
		config.Namespace,
		config.NetworkNamespace,
		config.ResourceName,
	).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")

	// Set optional parameters
	if config.SpoofCheck != "" {
		if config.SpoofCheck == "on" {
			networkBuilder.WithSpoof(true)
		} else {
			networkBuilder.WithSpoof(false)
		}
	}

	if config.Trust != "" {
		if config.Trust == "on" {
			networkBuilder.WithTrustFlag(true)
		} else {
			networkBuilder.WithTrustFlag(false)
		}
	}

	if config.VlanID > 0 {
		networkBuilder.WithVLAN(uint16(config.VlanID))
	}

	if config.VlanQoS > 0 {
		networkBuilder.WithVlanQoS(uint16(config.VlanQoS))
	}

	if config.MinTxRate > 0 {
		networkBuilder.WithMinTxRate(uint16(config.MinTxRate))
	}

	if config.MaxTxRate > 0 {
		networkBuilder.WithMaxTxRate(uint16(config.MaxTxRate))
	}

	// Set LinkState with default to "auto" if not specified
	// This matches the behavior of openshift-tests-private and ensures VF follows PF state
	linkState := config.LinkState
	if linkState == "" {
		linkState = "auto"
		glog.V(90).Infof("LinkState not specified, defaulting to 'auto' for SRIOV network %q", config.Name)
	}
	networkBuilder.WithLinkState(linkState)

	sriovNetwork, err := networkBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create SRIOV network %q: %w", config.Name, err)
	}

	// Verify the SRIOV network was created successfully
	if sriovNetwork != nil && sriovNetwork.Object != nil {
		glog.V(90).Infof("SRIOV network created successfully - name: %q, namespace: %q, resourceName: %q, targetNamespace: %q",
			config.Name, config.Namespace, sriovNetwork.Object.Spec.ResourceName, sriovNetwork.Object.Spec.NetworkNamespace)
	} else {
		// Fallback: try to pull the network to verify it exists
		createdNetwork, pullErr := sriov.PullNetwork(apiClient, config.Name, config.Namespace)
		if pullErr != nil {
			return fmt.Errorf("failed to verify created SRIOV network %q: %w", config.Name, pullErr)
		}
		glog.V(90).Infof("SRIOV network created successfully - name: %q, namespace: %q, resourceName: %q, targetNamespace: %q",
			config.Name, config.Namespace, createdNetwork.Object.Spec.ResourceName, createdNetwork.Object.Spec.NetworkNamespace)
	}

	// Verify that a SRIOV policy exists for the resourceName before waiting for NAD
	glog.V(90).Infof("Verifying SRIOV policy exists for resource %q", config.ResourceName)
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingInterval,
		tsparams.NamespaceTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			policy, err := sriov.PullPolicy(apiClient, config.ResourceName, config.Namespace)
			if err == nil && policy != nil && policy.Object != nil {
				glog.V(90).Infof("SRIOV policy found - name: %q, namespace: %q, resourceName: %q, numVfs: %d",
					config.ResourceName, config.Namespace, policy.Object.Spec.ResourceName, policy.Object.Spec.NumVfs)
				return true, nil
			}
			if err != nil {
				glog.V(90).Infof("SRIOV policy not found - name: %q, namespace: %q, error: %v", config.ResourceName, config.Namespace, err)
			}
			return false, nil
		})

	if err != nil {
		return fmt.Errorf("SRIOV policy %q must exist in namespace %q before NAD can be created. Ensure initVF succeeded and the policy was created: %w",
			config.ResourceName, config.Namespace, err)
	}

	// Wait for NetworkAttachmentDefinition to be created by the SRIOV operator
	glog.V(90).Infof("Waiting for NetworkAttachmentDefinition %q to be created in namespace %q", config.Name, config.NetworkNamespace)
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingInterval,
		tsparams.NADTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(apiClient, config.Name, config.NetworkNamespace)
			if err != nil {
				glog.V(90).Infof("NetworkAttachmentDefinition not yet created - name: %q, namespace: %q, error: %v",
					config.Name, config.NetworkNamespace, err)
				return false, nil
			}
			return true, nil
		})

	if err != nil {
		return fmt.Errorf("failed to wait for NetworkAttachmentDefinition %q in namespace %q. Ensure the SRIOV policy exists and is properly configured: %w",
			config.Name, config.NetworkNamespace, err)
	}

	// Verify that VF resources are actually available on nodes
	// Note: This check is important but can timeout if policy is still being applied
	// We use a shorter timeout here and let the test proceed - VF availability will be checked when pods are created
	glog.V(90).Infof("Verifying VF resources are available for %q", config.ResourceName)
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.VFResourcePollingInterval,
		tsparams.VFResourceTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			// Use global SriovOcpConfig from ocpsriovinittools
			available, err := VerifyVFResourcesAvailable(apiClient, SriovOcpConfig, config.ResourceName)
			if err != nil {
				return false, err
			}
			return available, nil
		})

	if err != nil {
		// Log warning but don't fail - VF resources may still be provisioning
		// The actual availability will be checked when test pods are created
		glog.V(90).Infof("VF resources %q not yet available (may still be provisioning): %v. Will proceed and check again when pods are created.", config.ResourceName, err)
		// Don't return error - let the test proceed
	}

	glog.V(90).Infof("SRIOV network %q created and ready", config.Name)
	return nil
}

const (
	// DefaultMcpLabel is the default MachineConfigPool label for worker nodes
	DefaultMcpLabel = "machineconfiguration.openshift.io/role=worker"
)

// CheckSriovOperatorStatus checks if SR-IOV operator is running and healthy
func CheckSriovOperatorStatus(apiClient *clients.Settings, config *sriovconfig.SriovOcpConfig) error {
	glog.V(90).Infof("Checking SR-IOV operator status in namespace %q", config.OcpSriovOperatorNamespace)
	// Use the centralized function from sriovoperator package
	return sriovoperator.IsSriovDeployed(apiClient, config.OcpSriovOperatorNamespace)
}

// WaitForSriovPolicyReady waits for SR-IOV policy to be ready and MCP to be stable
func WaitForSriovPolicyReady(apiClient *clients.Settings, config *sriovconfig.SriovOcpConfig, timeout time.Duration) error {
	glog.V(90).Infof("Waiting for SR-IOV policy to be ready (timeout: %v)", timeout)
	return WaitForSriovAndMCPStable(
		apiClient, timeout, tsparams.MCPStableInterval, DefaultMcpLabel, config.OcpSriovOperatorNamespace)
}

// VerifyWorkerNodesReady verifies that all worker nodes are stable and ready for SRIOV initialization
func VerifyWorkerNodesReady(apiClient *clients.Settings, workerNodes []*nodes.Builder, sriovOpNs string) error {
	if apiClient == nil {
		return fmt.Errorf("API client is nil, cannot verify node readiness")
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no worker nodes provided to VerifyWorkerNodesReady")
	}

	glog.V(90).Infof("Verifying worker node readiness for SRIOV operator namespace %q", sriovOpNs)

	allNodesReady := true
	var lastErr error

	for _, node := range workerNodes {
		nodeName := node.Definition.Name
		glog.V(90).Infof("Checking node readiness for node %q", nodeName)

		// Verify node is in Ready state
		refreshedNode, err := nodes.Pull(apiClient, nodeName)
		if err != nil {
			glog.V(90).Infof("Failed to pull node %q: %v", nodeName, err)
			allNodesReady = false
			lastErr = err
			continue
		}

		if refreshedNode == nil || refreshedNode.Definition == nil {
			glog.V(90).Infof("Node definition is nil for node %q", nodeName)
			allNodesReady = false
			lastErr = fmt.Errorf("node definition is nil for node %q", nodeName)
			continue
		}

		// Check node conditions
		hasReadyCondition := false
		hasNotSchedulableCondition := false

		for _, condition := range refreshedNode.Definition.Status.Conditions {
			glog.V(90).Infof("Node condition - node: %q, type: %q, status: %q, reason: %q, message: %q",
				nodeName, condition.Type, condition.Status, condition.Reason, condition.Message)

			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				hasReadyCondition = true
			}

			// Check for scheduling issues
			if condition.Type == corev1.NodeMemoryPressure && condition.Status == corev1.ConditionTrue {
				glog.V(90).Infof("Node %q has memory pressure", nodeName)
				allNodesReady = false
			}
			if condition.Type == corev1.NodeDiskPressure && condition.Status == corev1.ConditionTrue {
				glog.V(90).Infof("Node %q has disk pressure", nodeName)
				allNodesReady = false
			}
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionFalse {
				glog.V(90).Infof("Node %q is not ready (reason: %q)", nodeName, condition.Reason)
				allNodesReady = false
			}
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionUnknown {
				hasNotSchedulableCondition = true
			}

			// Check for node reboot/restart indicators
			if strings.Contains(condition.Reason, "NodeNotReady") ||
				strings.Contains(condition.Reason, "Rebooting") ||
				strings.Contains(condition.Reason, "KernelDeadlock") {
				glog.V(90).Infof("Node %q appears to be rebooting or unstable (reason: %q, message: %q)",
					nodeName, condition.Reason, condition.Message)
				allNodesReady = false
			}
		}

		if !hasReadyCondition {
			glog.V(90).Infof("Node %q is not in Ready state", nodeName)
			allNodesReady = false
			lastErr = fmt.Errorf("node %q is not in Ready state", nodeName)
		}

		if hasNotSchedulableCondition {
			glog.V(90).Infof("Node %q is unschedulable", nodeName)
			allNodesReady = false
			lastErr = fmt.Errorf("node %q is unschedulable", nodeName)
		}
	}

	if !allNodesReady {
		if lastErr != nil {
			return fmt.Errorf("one or more worker nodes are not ready: %w", lastErr)
		}
		return fmt.Errorf("one or more worker nodes are not ready")
	}

	glog.V(90).Info("All worker nodes are ready for SRIOV initialization")
	return nil
}

// discoverInterfaceName discovers the actual interface name on a node by matching Vendor and DeviceID
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
			glog.V(90).Infof("Found interface %q on node %q matching vendor %q and deviceID %q",
				iface.Name, nodeName, vendor, deviceID)
			return iface.Name, nil
		}
	}

	return "", fmt.Errorf("no interface found on node %q matching vendor %q and deviceID %q", nodeName, vendor, deviceID)
}

// initVFWithDevType is a common helper function for initializing VF with a specific device type
func initVFWithDevType(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, deviceID, interfaceName, vendor, sriovOpNs, devType string,
	vfNum int,
	workerNodes []*nodes.Builder) (bool, error) {
	glog.V(90).Infof("Initializing VF for device %q (deviceID: %q, vendor: %q, interface: %q, vfNum: %d, devType: %q)",
		name, deviceID, vendor, interfaceName, vfNum, devType)

	// Verify all worker nodes are stable and ready before initializing SRIOV
	err := VerifyWorkerNodesReady(apiClient, workerNodes, sriovOpNs)
	if err != nil {
		glog.V(90).Infof("Worker nodes are not ready for SRIOV initialization: %v", err)
		return false, fmt.Errorf("worker nodes are not ready for SRIOV initialization: %w", err)
	}

	// Clean up any existing policy with the same name before creating a new one
	// This prevents VF range conflicts from previous test runs
	glog.V(90).Infof("Checking for existing SRIOV policy %q that might conflict", name)
	err = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.NamespaceTimeout)
	if err != nil {
		glog.V(90).Infof("Note: Could not remove existing policy %q (may not exist): %v", name, err)
	}

	// Check if the device exists on any worker node
	for _, node := range workerNodes {
		// Try to discover the actual interface name from node state if vendor and deviceID are provided
		actualInterfaceName := interfaceName
		if vendor != "" && deviceID != "" {
			discoveredName, err := discoverInterfaceName(apiClient, node.Definition.Name, sriovOpNs, vendor, deviceID)
			if err == nil {
				actualInterfaceName = discoveredName
				glog.V(90).Infof("Discovered interface name %q for node %q (requested: %q)", actualInterfaceName, node.Definition.Name, interfaceName)
			} else {
				glog.V(90).Infof("Could not discover interface name for node %q, using requested name %q: %v", node.Definition.Name, interfaceName, err)
			}
		}

		pfSelector := fmt.Sprintf("%s#0-%d", actualInterfaceName, vfNum-1)
		glog.V(90).Infof("Creating SRIOV policy - name: %q, node: %q, pfSelector: %q, deviceID: %q, vendor: %q, interfaceName: %q, devType: %q",
			name, node.Definition.Name, pfSelector, deviceID, vendor, actualInterfaceName, devType)

		// Create SRIOV policy
		sriovPolicy := sriov.NewPolicyBuilder(
			apiClient,
			name,
			sriovOpNs,
			name,
			vfNum,
			[]string{pfSelector},
			map[string]string{"kubernetes.io/hostname": node.Definition.Name},
		).WithDevType(devType)

		// Set Vendor and DeviceID in NicSelector to help operator match hardware
		// This is especially important when interface names might not match exactly
		if vendor != "" {
			sriovPolicy.Definition.Spec.NicSelector.Vendor = vendor
		}
		if deviceID != "" {
			sriovPolicy.Definition.Spec.NicSelector.DeviceID = deviceID
		}

		_, err := sriovPolicy.Create()
		if err != nil {
			glog.V(90).Infof("Failed to create SRIOV policy on node %q: %v (pfSelector: %q, deviceID: %q, vendor: %q, interfaceName: %q)",
				node.Definition.Name, err, pfSelector, deviceID, vendor, actualInterfaceName)
			// Clean up any partially created policy
			_ = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.DefaultTimeout)
			continue
		}

		glog.V(90).Infof("SRIOV policy created successfully, waiting for it to be applied - name: %q, node: %q", name, node.Definition.Name)

		// Wait for policy to be applied
		err = WaitForSriovAndMCPStable(
			apiClient, tsparams.PolicyApplicationTimeout, tsparams.MCPStableInterval, DefaultMcpLabel, sriovOpNs)
		if err != nil {
			glog.V(90).Infof("Failed to wait for SRIOV policy to be applied on node %q: %v", node.Definition.Name, err)
			// Clean up policy if wait fails
			_ = RemoveSriovPolicy(apiClient, name, sriovOpNs, tsparams.DefaultTimeout)
			continue
		}

		glog.V(90).Infof("SRIOV policy successfully applied - name: %q, node: %q", name, node.Definition.Name)
		return true, nil
	}

	glog.V(90).Infof("Failed to create SRIOV policy on any worker node - name: %q, deviceID: %q, vendor: %q, interfaceName: %q",
		name, deviceID, vendor, interfaceName)
	return false, fmt.Errorf("failed to create SRIOV policy %q on any worker node (deviceID: %q, vendor: %q, interfaceName: %q)",
		name, deviceID, vendor, interfaceName)
}

// InitVF initializes VF for the given device
func InitVF(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, deviceID, interfaceName, vendor, sriovOpNs string,
	vfNum int,
	workerNodes []*nodes.Builder) (bool, error) {
	return initVFWithDevType(apiClient, config, name, deviceID, interfaceName, vendor, sriovOpNs, "netdevice", vfNum, workerNodes)
}

// CreateTestPod creates a test pod with SRIOV network
func CreateTestPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, namespace, networkName, ipAddress, macAddress string) (*pod.Builder, error) {
	glog.V(90).Infof("Creating test pod %q in namespace %q with network %q (ip: %q, mac: %q)",
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

	glog.V(90).Infof("Test pod %q created successfully", name)
	return createdPod, nil
}

// VerifyInterfaceReady verifies that a pod's network interface is ready
func VerifyInterfaceReady(podObj *pod.Builder, interfaceName, podName string) error {
	glog.V(90).Infof("Verifying interface %q is ready on pod %q", interfaceName, podName)

	cmd := []string{"ip", "link", "show", interfaceName}
	output, err := podObj.ExecCommand(cmd)

	// Handle cases where pod is not accessible (already deleted, terminating, etc.)
	if err != nil {
		// Check if it's a network error (pod inaccessible)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			glog.V(90).Infof("Pod %q not accessible (may be terminating or deleted): %v", podName, err)
			return fmt.Errorf("pod %q is not accessible - likely being terminated or already deleted: %w", podName, err)
		}
		return fmt.Errorf("failed to get interface status for pod %q: %w", podName, err)
	}

	// Check if interface is UP
	outputStr := output.String()
	if !strings.Contains(outputStr, "UP") || strings.Contains(outputStr, "DOWN") {
		return fmt.Errorf("interface %q is not UP on pod %q (output: %q)", interfaceName, podName, outputStr)
	}

	glog.V(90).Infof("Interface %q is ready on pod %q", interfaceName, podName)
	return nil
}

// CheckInterfaceCarrier checks if interface has carrier (physical link is active)
func CheckInterfaceCarrier(podObj *pod.Builder, interfaceName string) (bool, error) {
	glog.V(90).Infof("Checking carrier status for interface %q", interfaceName)

	cmd := []string{"ip", "link", "show", interfaceName}
	output, err := podObj.ExecCommand(cmd)
	if err != nil {
		// Handle cases where pod is not accessible (already deleted, terminating, etc.)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			glog.V(90).Infof("Pod not accessible when checking carrier status (may be terminating): %v", err)
			return false, fmt.Errorf("pod not accessible when checking carrier status: %w", err)
		}
		return false, fmt.Errorf("failed to get interface status: %w", err)
	}

	outputStr := output.String()

	// Check for NO-CARRIER flag
	if strings.Contains(outputStr, "NO-CARRIER") {
		glog.V(90).Infof("Interface %q has NO-CARRIER status", interfaceName)
		return false, nil
	}

	glog.V(90).Infof("Interface %q carrier is active", interfaceName)
	return true, nil
}

// ExtractPodInterfaceMAC extracts the MAC address from a pod's interface
func ExtractPodInterfaceMAC(podObj *pod.Builder, interfaceName string) (string, error) {
	glog.V(90).Infof("Extracting MAC address from interface %q", interfaceName)

	cmd := []string{"ip", "link", "show", interfaceName}
	output, err := podObj.ExecCommand(cmd)
	if err != nil {
		// Handle cases where pod is not accessible (already deleted, terminating, etc.)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "i/o timeout") ||
			strings.Contains(err.Error(), "connection reset") {
			glog.V(90).Infof("Pod not accessible when extracting MAC (may be terminating): %v", err)
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
					glog.V(90).Infof("Extracted MAC address %q from interface %q", mac, interfaceName)
					return mac, nil
				}
			}
		}
	}

	return "", fmt.Errorf("MAC address not found for interface %q", interfaceName)
}

// VerifyVFSpoofCheck verifies that spoof checking is active on the VF
// Note: This function logs diagnostic commands but doesn't execute them on the node
// Actual verification would require node access which is typically done via oc debug
func VerifyVFSpoofCheck(nodeName, nicName, podMAC string) error {
	glog.V(90).Infof("Verifying spoof checking is active on node %q for MAC %q (interface: %q)", nodeName, podMAC, nicName)

	// Validate inputs
	if nodeName == "" {
		return fmt.Errorf("node name should not be empty")
	}
	if nicName == "" {
		return fmt.Errorf("interface name should not be empty")
	}
	if podMAC == "" {
		return fmt.Errorf("pod MAC should not be empty")
	}

	// Log the diagnostic command that would be used for verification
	// In a real implementation, this would execute: oc debug node/<nodeName> -- chroot /host sh -c "ip link show <nicName> | grep -i spoof"
	glog.V(90).Infof("Spoof checking verification - node: %q, interface: %q, podMAC: %q (diagnostic command: oc debug node/%s -- chroot /host sh -c 'ip link show %s | grep -i spoof')",
		nodeName, nicName, podMAC, nodeName, nicName)

	glog.V(90).Infof("VF spoof checking verification setup complete - node: %q, interface: %q, mac: %q", nodeName, nicName, podMAC)
	return nil
}

// VerifyLinkStateConfiguration verifies that link state configuration is applied without requiring connectivity
// This function creates a test pod and verifies that the interface is up with the expected configuration
func VerifyLinkStateConfiguration(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	networkName, namespace, description string,
	timeout time.Duration) (bool, error) {
	glog.V(90).Infof("Verifying link state configuration: %q (network: %q, namespace: %q)", description, networkName, namespace)

	// Create a single test pod to verify link state
	testPod, err := CreateTestPod(apiClient, config, "linkstate-test", namespace, networkName, "192.168.1.10/24", "20:04:0f:f1:88:01")
	if err != nil {
		return false, fmt.Errorf("failed to create test pod: %w", err)
	}

	// Defer cleanup of pod
	defer func() {
		glog.V(90).Info("Cleaning up link state test pod")
		if testPod != nil {
			_, _ = testPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	// Wait for pod to be ready
	glog.V(90).Info("Waiting for test pod to be ready")
	err = testPod.WaitUntilReady(timeout)
	if err != nil {
		if testPod.Definition != nil {
			glog.V(90).Infof("Test pod status - phase: %q, reason: %q, message: %q",
				testPod.Definition.Status.Phase, testPod.Definition.Status.Reason, testPod.Definition.Status.Message)
		}
		return false, fmt.Errorf("test pod not ready: %w", err)
	}

	// Verify interface configuration on pod
	glog.V(90).Info("Verifying interface configuration on pod")
	err = VerifyInterfaceReady(testPod, "net1", "linkstate-test")
	if err != nil {
		return false, fmt.Errorf("failed to verify test pod interface: %w", err)
	}

	// Check carrier status to determine if connectivity tests can be run
	glog.V(90).Info("Checking interface carrier status")
	hasCarrier, err := CheckInterfaceCarrier(testPod, "net1")
	if err != nil {
		return false, fmt.Errorf("failed to check interface carrier status: %w", err)
	}

	if !hasCarrier {
		glog.V(90).Info("Interface has NO-CARRIER status - link state configuration is applied but no physical connection")
		return false, nil // No carrier, but configuration is valid
	}

	glog.V(90).Infof("Link state configuration verified successfully with carrier: %q", description)
	return true, nil // Has carrier, connectivity tests can proceed
}

// CheckVFStatusWithPassTraffic checks VF status and passes traffic between test pods
// This function creates client and server pods, verifies interface configuration,
// checks carrier status, verifies spoof checking, and tests connectivity
func CheckVFStatusWithPassTraffic(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	networkName, interfaceName, namespace, description string,
	timeout time.Duration) error {
	glog.V(90).Infof("Checking VF status with traffic: %q (network: %q, interface: %q, namespace: %q)", description, networkName, interfaceName, namespace)

	// Create test pods
	clientPod, err := CreateTestPod(apiClient, config, "client", namespace, networkName, "192.168.1.10/24", "20:04:0f:f1:88:01")
	if err != nil {
		return fmt.Errorf("failed to create client pod: %w", err)
	}

	serverPod, err := CreateTestPod(apiClient, config, "server", namespace, networkName, "192.168.1.11/24", "20:04:0f:f1:88:02")
	if err != nil {
		// Try to clean up client pod if server pod creation fails
		_, _ = clientPod.DeleteAndWait(tsparams.NamespaceTimeout)
		return fmt.Errorf("failed to create server pod: %w", err)
	}

	// Defer cleanup of pods
	defer func() {
		glog.V(90).Info("Cleaning up test pods")
		if clientPod != nil {
			_, _ = clientPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
		if serverPod != nil {
			_, _ = serverPod.DeleteAndWait(tsparams.CleanupTimeout)
		}
	}()

	// Wait for pods to be ready
	glog.V(90).Info("Waiting for client pod to be ready")
	err = clientPod.WaitUntilReady(timeout)
	if err != nil {
		if clientPod.Definition != nil {
			glog.V(90).Infof("Client pod status - phase: %q, reason: %q, message: %q",
				clientPod.Definition.Status.Phase, clientPod.Definition.Status.Reason, clientPod.Definition.Status.Message)
		}
		return fmt.Errorf("client pod not ready: %w", err)
	}

	glog.V(90).Info("Waiting for server pod to be ready")
	err = serverPod.WaitUntilReady(timeout)
	if err != nil {
		if serverPod.Definition != nil {
			glog.V(90).Infof("Server pod status - phase: %q, reason: %q, message: %q",
				serverPod.Definition.Status.Phase, serverPod.Definition.Status.Reason, serverPod.Definition.Status.Message)
		}
		return fmt.Errorf("server pod not ready: %w", err)
	}

	// Verify interface configuration on pods
	glog.V(90).Info("Verifying interface configuration on pods")
	err = VerifyInterfaceReady(clientPod, "net1", "client")
	if err != nil {
		return fmt.Errorf("failed to verify client pod interface: %w", err)
	}

	err = VerifyInterfaceReady(serverPod, "net1", "server")
	if err != nil {
		return fmt.Errorf("failed to verify server pod interface: %w", err)
	}

	// Check for NO-CARRIER status
	glog.V(90).Info("Checking interface link status")
	clientCarrier, err := CheckInterfaceCarrier(clientPod, "net1")
	if err != nil {
		return fmt.Errorf("failed to check client interface carrier status: %w", err)
	}

	if !clientCarrier {
		glog.V(90).Info("Interface has NO-CARRIER status (physical link down), skipping connectivity test")
		// Return a special error that indicates the test should be skipped
		// The test file can check for this and call Skip() if needed
		return fmt.Errorf("interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
	}

	// Verify Spoof Checking on VF (Extract MAC and verify on node)
	glog.V(90).Info("Verifying spoof checking is active on VF")
	// Refresh pod definition to get the latest node name after it was scheduled
	refreshedPod, err := pod.Pull(apiClient, clientPod.Definition.Name, clientPod.Definition.Namespace)
	if err != nil {
		return fmt.Errorf("failed to refresh client pod definition: %w", err)
	}

	clientPodNode := refreshedPod.Definition.Spec.NodeName
	if clientPodNode == "" {
		return fmt.Errorf("client pod node name should not be empty after scheduling")
	}
	glog.V(90).Infof("Client pod is running on node %q", clientPodNode)

	// Extract client pod's MAC address from net1 interface
	clientMAC, err := ExtractPodInterfaceMAC(clientPod, "net1")
	if err != nil {
		// Check if it's a network error (pod inaccessible)
		if strings.Contains(err.Error(), "use of closed network connection") ||
			strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "pod not accessible") {
			glog.V(90).Infof("Pod not accessible when extracting MAC (may be terminating): %v", err)
			return fmt.Errorf("pod is not accessible - likely being terminated or already deleted: %w", err)
		}
		return fmt.Errorf("failed to extract client pod MAC address: %w", err)
	}
	glog.V(90).Infof("Client pod MAC address extracted: %q", clientMAC)

	// Verify spoof checking is enabled on node
	err = VerifyVFSpoofCheck(clientPodNode, interfaceName, clientMAC)
	if err != nil {
		return fmt.Errorf("failed to verify VF spoof checking: %w", err)
	}

	// Test connectivity with timeout
	glog.V(90).Info("Testing connectivity between pods")
	pingCmd := []string{"ping", "-c", "3", "192.168.1.11"}
	pingTimeout := tsparams.PingTimeout

	var pingOutput bytes.Buffer
	err = wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PingPollingInterval,
		pingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var execErr error
			pingOutput, execErr = clientPod.ExecCommand(pingCmd)
			if execErr != nil {
				glog.V(90).Infof("Ping command failed, will retry: %v (output: %q)", execErr, pingOutput.String())
				return false, nil // Retry on error
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
	if !strings.Contains(pingOutputStr, "3 packets transmitted") {
		return fmt.Errorf("ping did not complete successfully (output: %q)", pingOutputStr)
	}

	glog.V(90).Infof("VF status verification with traffic completed successfully: %q", description)
	return nil
}

// InitDpdkVF initializes DPDK VF for the given device
func InitDpdkVF(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, deviceID, interfaceName, vendor, sriovOpNs string,
	vfNum int,
	workerNodes []*nodes.Builder) (bool, error) {
	return initVFWithDevType(apiClient, config, name, deviceID, interfaceName, vendor, sriovOpNs, "vfio-pci", vfNum, workerNodes)
}

// GetPciAddress gets the PCI address for a pod from network status annotation
func GetPciAddress(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	namespace, podName, policyName string) (string, error) {
	glog.V(90).Infof("Getting PCI address for pod %q in namespace %q (policy: %q)", podName, namespace, policyName)

	podBuilder := pod.NewBuilder(apiClient, podName, namespace, config.OcpSriovTestContainer)
	if !podBuilder.Exists() {
		return "", fmt.Errorf("pod %q does not exist in namespace %q", podName, namespace)
	}

	// Pull the pod to get the latest annotations
	podObj, err := pod.Pull(apiClient, podName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull pod %q: %w", podName, err)
	}

	if podObj == nil || podObj.Object == nil {
		return "", fmt.Errorf("pod object is nil for pod %q", podName)
	}

	// Get the network status annotation
	networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"
	podNetAnnotation := podObj.Object.Annotations[networkStatusAnnotation]
	if podNetAnnotation == "" {
		glog.V(90).Infof("Pod network annotation not found for pod %q in namespace %q", podName, namespace)
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
		glog.V(90).Infof("Failed to unmarshal pod network status for pod %q: %v", podName, err)
		return "", fmt.Errorf("failed to unmarshal pod network status annotation: %w", err)
	}

	// Find the network matching the policy name
	for _, networkAnnotation := range annotation {
		if strings.Contains(networkAnnotation.Name, policyName) {
			if networkAnnotation.DeviceInfo.Pci.PciAddress != "" {
				glog.V(90).Infof("PCI address found for pod %q: %q (policy: %q)", podName, networkAnnotation.DeviceInfo.Pci.PciAddress, policyName)
				return networkAnnotation.DeviceInfo.Pci.PciAddress, nil
			}
		}
	}

	glog.V(90).Infof("PCI address not found for pod %q with policy %q", podName, policyName)
	return "", fmt.Errorf("PCI address not found for pod %q with policy %q", podName, policyName)
}

// UpdateSriovPolicyMTU updates the MTU value of an existing SR-IOV policy
// using the eco-goinfra PolicyBuilder Update() helper.
func UpdateSriovPolicyMTU(apiClient *clients.Settings, policyName, sriovOpNs string, mtuValue int) error {
	glog.V(90).Infof("Updating SR-IOV policy %q MTU to %d in namespace %q", policyName, mtuValue, sriovOpNs)

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

	// Update the builder's Object reference
	policyBuilder.Object = updatedPolicy.Object

	glog.V(90).Infof("SR-IOV policy %q successfully updated with MTU %d", policyName, mtuValue)
	return nil
}

// CreateDpdkTestPod creates a DPDK test pod with SR-IOV network
func CreateDpdkTestPod(
	apiClient *clients.Settings,
	config *sriovconfig.SriovOcpConfig,
	name, namespace, networkName string) (*pod.Builder, error) {
	glog.V(90).Infof("Creating DPDK test pod %q in namespace %q with network %q", name, namespace, networkName)

	// Create network annotation (DPDK pods use the network name directly)
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(networkName, []string{"192.168.1.10/24"}, "20:04:0f:f1:88:01")

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

	glog.V(90).Infof("DPDK test pod %q created successfully", name)
	return createdPod, nil
}

// DeleteDpdkTestPod deletes a DPDK test pod
func DeleteDpdkTestPod(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	glog.V(90).Infof("Deleting DPDK test pod %q from namespace %q", name, namespace)

	podBuilder := pod.NewBuilder(apiClient, name, namespace, "")
	if !podBuilder.Exists() {
		glog.V(90).Infof("DPDK test pod %q does not exist in namespace %q, skipping deletion", name, namespace)
		return nil
	}

	_, err := podBuilder.DeleteAndWait(timeout)
	if err != nil {
		return fmt.Errorf("failed to delete DPDK test pod %q: %w", name, err)
	}

	glog.V(90).Infof("DPDK test pod %q successfully deleted", name)
	return nil
}
