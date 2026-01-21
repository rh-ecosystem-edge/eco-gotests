package neuronhelpers

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// WaitForClusterStability waits for all nodes to be ready.
// Note: MCP checks are skipped for Neuron tests as they only run on ROSA where MCPs don't exist.
func WaitForClusterStability(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for cluster stability (checking node readiness)")

	err := WaitForAllNodesReady(apiClient, timeout)
	if err != nil {
		return fmt.Errorf("nodes not ready: %w", err)
	}

	return nil
}

// WaitForAllNodesReady waits for all nodes to be in Ready condition.
func WaitForAllNodesReady(apiClient *clients.Settings, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, node := range nodeList {
				ready := false

				for _, condition := range node.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						ready = true

						break
					}
				}

				if !ready {
					klog.V(params.NeuronLogLevel).Infof("Node %s not ready", node.Object.Name)

					return false, nil
				}
			}

			return true, nil
		})
}

// WaitForClusterStabilityAfterDeviceConfig waits for cluster stability after DeviceConfig is applied.
func WaitForClusterStabilityAfterDeviceConfig(apiClient *clients.Settings) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for cluster stability after DeviceConfig creation")

	// Wait a bit for the operator to start reconciling
	time.Sleep(30 * time.Second)

	// Wait for nodes to be ready
	return WaitForClusterStability(apiClient, params.ClusterStabilityTimeout)
}

// WaitForNeuronNodesToBeReady waits for all Neuron-labeled nodes to be ready.
func WaitForNeuronNodesToBeReady(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for Neuron nodes to be ready")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			for _, node := range nodeList {
				ready := false

				for _, condition := range node.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						ready = true

						break
					}
				}

				if !ready {
					klog.V(params.NeuronLogLevel).Infof("Neuron node %s not ready", node.Object.Name)

					return false, nil
				}
			}

			return true, nil
		})
}

// WaitForNodeReboot waits for a node to reboot (become NotReady then Ready again).
func WaitForNodeReboot(apiClient *clients.Settings, nodeName string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for node %s to reboot", nodeName)

	// First wait for node to become NotReady (optional, as it might be fast)
	_ = wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, 2*time.Minute, false,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
			})
			if err != nil || len(nodeList) == 0 {
				return true, nil // Node not found, likely rebooting
			}

			for _, condition := range nodeList[0].Object.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionFalse {
					return true, nil
				}
			}

			return false, nil
		})

	// Then wait for node to become Ready again
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			for _, condition := range nodeList[0].Object.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					klog.V(params.NeuronLogLevel).Infof("Node %s is ready after reboot", nodeName)

					return true, nil
				}
			}

			return false, nil
		})
}
