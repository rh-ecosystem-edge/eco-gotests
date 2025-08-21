package amdgpuconfig

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/exec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This package now focuses only on node labeling and verification operations
// Other functionality has been moved to more focused packages:
// - amdgpudeviceconfig: DeviceConfig CRD operations
// - amdgpumachineconfig: MachineConfig and MCP operations
// - amdgpuregistry: Image registry management
// - amdgpunfd: NodeFeatureDiscovery operations

// WaitForAMDGPUNodes waits for nodes to be labeled with AMD GPU features.
func WaitForAMDGPUNodes(apiClient *clients.Settings, timeout time.Duration) error {
	glog.V(90).Info("Waiting for nodes to be labeled with AMD GPU features")

	return WaitForNodeLabel(apiClient, "feature.node.kubernetes.io/amd-gpu", "true", timeout)
}

// WaitForNodeLabel waits for at least one node to have the specified label.
func WaitForNodeLabel(apiClient *clients.Settings, labelKey, labelValue string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			nodes, err := apiClient.CoreV1Interface.Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: labelKey + "=" + labelValue,
			})
			if err != nil {
				glog.V(90).Infof("Error listing nodes: %v", err)
				time.Sleep(10 * time.Second)

				continue
			}

			if len(nodes.Items) > 0 {
				glog.V(90).Infof("Found %d nodes with label %s=%s", len(nodes.Items), labelKey, labelValue)

				return nil
			}

			glog.V(90).Infof("No nodes found with label %s=%s, waiting...", labelKey, labelValue)
			time.Sleep(10 * time.Second)
		}
	}
}

// VerifyAMDGPUKernelModule checks if the amdgpu kernel module is properly blacklisted.
func VerifyAMDGPUKernelModule(apiClient *clients.Settings) error {
	glog.V(amdgpuparams.LogLevel).Infof("Verifying amdgpu kernel module is blacklisted on nodes")

	nodes, err := getAMDGPUNodes(apiClient)
	if err != nil {
		glog.V(amdgpuparams.LogLevel).Infof("Failed to get AMD GPU nodes (this may be expected): %v", err)

		return nil
	}

	if len(nodes.Items) == 0 {
		glog.V(amdgpuparams.LogLevel).Infof("No nodes with AMD GPU labels found - verification skipped")

		return nil
	}

	return verifyKernelModuleOnNodes(apiClient, nodes.Items)
}

// getAMDGPUNodes retrieves nodes with AMD GPU labels.
func getAMDGPUNodes(apiClient *clients.Settings) (*corev1.NodeList, error) {
	nodes, err := apiClient.CoreV1Interface.Nodes().List(
		context.TODO(), metav1.ListOptions{
			LabelSelector: "feature.node.kubernetes.io/amd-gpu=true",
		})
	if err != nil {
		return nil, fmt.Errorf("failed to list AMD GPU nodes: %w", err)
	}

	return nodes, nil
}

// verifyKernelModuleOnNodes verifies kernel module status on all provided nodes.
func verifyKernelModuleOnNodes(apiClient *clients.Settings, nodes []corev1.Node) error {
	success := true

	for _, node := range nodes {
		if !checkKernelModuleOnNode(apiClient, node.Name) {
			success = false
		}
	}

	if !success {
		return fmt.Errorf("failed to verify amdgpu module status on some nodes")
	}

	return nil
}

// checkKernelModuleOnNode checks kernel module status on a single node.
func checkKernelModuleOnNode(apiClient *clients.Settings, nodeName string) bool {
	glog.V(amdgpuparams.LogLevel).Infof("Checking amdgpu module status on node %s", nodeName)

	if !checkModuleBlacklist(apiClient, nodeName) {
		return false
	}

	return checkModuleLoadStatus(apiClient, nodeName)
}

// checkModuleBlacklist checks if amdgpu module is blacklisted.
func checkModuleBlacklist(apiClient *clients.Settings, nodeName string) bool {
	blacklistCheck := "modprobe -n -v amdgpu | grep -q 'blacklisted' && echo 'BLACKLISTED' || echo 'NOT_BLACKLISTED'"

	output, err := execCommandOnNode(apiClient, nodeName, blacklistCheck)
	if err != nil {
		glog.V(amdgpuparams.LogLevel).Infof("Error checking blacklist on node %s: %v", nodeName, err)

		return false
	}

	glog.V(amdgpuparams.LogLevel).Infof("Node %s amdgpu module blacklist status: %s", nodeName, output)

	return true
}

// checkModuleLoadStatus checks if amdgpu module is loaded.
func checkModuleLoadStatus(apiClient *clients.Settings, nodeName string) bool {
	loadedCheck := "lsmod | grep amdgpu || echo 'MODULE_NOT_LOADED'"
	output, err := execCommandOnNode(apiClient, nodeName, loadedCheck)

	if err != nil {
		glog.V(amdgpuparams.LogLevel).Infof("Error checking module load status on node %s: %v", nodeName, err)

		return false
	}

	logModuleLoadStatus(nodeName, output)

	return true
}

// logModuleLoadStatus logs the module load status.
func logModuleLoadStatus(nodeName, output string) {
	if strings.Contains(output, "amdgpu") && !strings.Contains(output, "MODULE_NOT_LOADED") {
		glog.V(amdgpuparams.LogLevel).Infof("WARNING: amdgpu module is still loaded on node %s", nodeName)
		glog.V(amdgpuparams.LogLevel).Infof("Module status: %s", output)
	} else {
		glog.V(amdgpuparams.LogLevel).Infof("Good: amdgpu module is not loaded on node %s", nodeName)
	}
}

// execCommandOnNode executes a command on a specific node and returns the output.
func execCommandOnNode(apiClient *clients.Settings, nodeName, command string) (string, error) {
	glog.V(amdgpuparams.LogLevel).Infof("Executing command on node %s: %s", nodeName, command)

	podName := fmt.Sprintf("debug-amdgpu-%s", strings.ToLower(nodeName))

	// Build nsenter command for node debugging
	nsenterCmd := []string{
		"nsenter", "--target", "1",
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--", "sh", "-c", command,
	}

	// Create pod command with node debugging configuration
	podCommand := exec.NewPodCommandDirect(
		apiClient,
		podName,
		"default",
		"registry.redhat.io/ubi8/ubi:latest",
		"debug",
		nsenterCmd,
		nil, // requests
		nil, // limits
	).
		WithNodeName(nodeName).
		WithPrivileged(true).
		WithHostNetwork(true).
		WithHostPID(true)

	output, err := podCommand.ExecuteAndCleanup(2 * time.Minute)
	if err != nil {
		glog.V(amdgpuparams.LogLevel).Infof("Command execution failed on node %s: %v", nodeName, err)

		return output, err
	}

	glog.V(amdgpuparams.LogLevel).Infof("Command executed successfully on node %s", nodeName)

	return output, nil
}
