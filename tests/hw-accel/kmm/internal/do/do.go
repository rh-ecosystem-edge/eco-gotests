package do

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// Reboot reboots a node using the existing kmm-test-helper pod.
func Reboot(apiClient *clients.Settings, nodeName, namespace string) error {
	klog.V(kmmparams.KmmLogLevel).Infof("Initiating reboot on node %s", nodeName)

	node, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	originalBootID := node.Object.Status.NodeInfo.BootID
	klog.V(kmmparams.KmmLogLevel).Infof("Node %s current boot ID: %s", nodeName, originalBootID)

	helperPod, err := await.ReadyHelperPod(apiClient, namespace, nodeName, time.Minute)
	if err != nil {
		return fmt.Errorf("failed to find ready helper pod on node %s: %w", nodeName, err)
	}

	klog.V(kmmparams.KmmLogLevel).Infof("Found helper pod %s on node %s", helperPod.Object.Name, nodeName)

	klog.V(kmmparams.KmmLogLevel).Infof("Executing 'chroot /host reboot' on node %s", nodeName)

	rebootCmd := []string{"chroot", "/host", "reboot"}
	_, _ = helperPod.ExecCommand(rebootCmd, "test")

	klog.V(kmmparams.KmmLogLevel).Infof("Waiting for node %s boot ID to change", nodeName)

	err = waitForBootIDChange(apiClient, nodeName, originalBootID, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("reboot verification failed: %w", err)
	}

	klog.V(kmmparams.KmmLogLevel).Infof("Waiting for node %s to become Ready", nodeName)

	err = waitForNodeReady(apiClient, nodeName, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("node did not become Ready after reboot: %w", err)
	}

	klog.V(kmmparams.KmmLogLevel).Infof("Node %s successfully rebooted and Ready", nodeName)

	return nil
}

// waitForBootIDChange waits for the node's boot ID to change from the original value.
func waitForBootIDChange(apiClient *clients.Settings, nodeName, originalBootID string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			node, err := nodes.Pull(apiClient, nodeName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("Node %s unreachable during reboot: %v", nodeName, err)

				return false, nil
			}

			newBootID := node.Object.Status.NodeInfo.BootID
			if newBootID != originalBootID {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Node %s boot ID changed: %s -> %s (reboot confirmed)",
					nodeName, originalBootID, newBootID)

				return true, nil
			}

			return false, nil
		})
}

// waitForNodeReady waits for the node to become Ready.
func waitForNodeReady(apiClient *clients.Settings, nodeName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			node, err := nodes.Pull(apiClient, nodeName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("Node %s still unreachable: %v", nodeName, err)

				return false, nil
			}

			for _, condition := range node.Object.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					klog.V(kmmparams.KmmLogLevel).Infof("Node %s is Ready", nodeName)

					return true, nil
				}
			}

			return false, nil
		})
}
