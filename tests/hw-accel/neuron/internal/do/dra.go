package do

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	"k8s.io/klog/v2"
)

const defaultTestImage = "registry.access.redhat.com/ubi9/ubi-minimal:latest"

// NewDRAConsumerPod creates a pod builder configured with a ResourceClaim
// referencing the given ResourceClaimTemplate name.
func NewDRAConsumerPod(
	apiClient *clients.Settings, name, namespace, claimTemplateName string) *pod.Builder {
	return pod.NewBuilder(apiClient, name, namespace, defaultTestImage).
		WithCommand([]string{"sleep", "infinity"}).
		WithResourceClaim("neuron-device", claimTemplateName)
}

// NewSleepPod creates a simple pod builder with a sleep infinity command.
func NewSleepPod(apiClient *clients.Settings, name, namespace string) *pod.Builder {
	return pod.NewBuilder(apiClient, name, namespace, defaultTestImage).
		WithCommand([]string{"sleep", "infinity"})
}

// CreateDRAConsumerPodAndWait creates a DRA consumer pod and waits for it to reach Running.
func CreateDRAConsumerPodAndWait(
	apiClient *clients.Settings, name, namespace, claimTemplateName string,
	timeout time.Duration) error {
	consumerPod := NewDRAConsumerPod(apiClient, name, namespace, claimTemplateName)

	_, err := consumerPod.Create()
	if err != nil {
		return fmt.Errorf("failed to create DRA consumer pod %s: %w", name, err)
	}

	return await.PodRunning(apiClient, name, namespace, timeout)
}

// DeletePodsIfExist deletes the named pods from a namespace, ignoring not-found errors.
func DeletePodsIfExist(apiClient *clients.Settings, namespace string, podNames []string) error {
	for _, podName := range podNames {
		existingPod, pullErr := pod.Pull(apiClient, podName, namespace)
		if pullErr != nil {
			if strings.Contains(pullErr.Error(), "does not exist") {
				continue
			}

			return fmt.Errorf("failed to look up pod %s: %w", podName, pullErr)
		}

		_, delErr := existingPod.DeleteAndWait(2 * time.Minute)
		if delErr != nil {
			return fmt.Errorf("failed to delete pod %s: %w", podName, delErr)
		}

		klog.V(params.NeuronLogLevel).Infof("Deleted pod %s", podName)
	}

	return nil
}

// ExhaustDRADevicesOnNode creates DRA consumer pods on a specific node to exhaust
// all available devices, then waits for all pods to reach Running state.
func ExhaustDRADevicesOnNode(
	apiClient *clients.Settings, namespace, claimTemplateName, targetNode string,
	deviceCount int, timeout time.Duration) error {
	nodeSelector := map[string]string{"kubernetes.io/hostname": targetNode}

	klog.V(params.NeuronLogLevel).Infof(
		"Creating %d exhaust pods on node %s", deviceCount, targetNode)

	for i := 0; i < deviceCount; i++ {
		name := fmt.Sprintf("exhaust-pod-%d", i)

		exhaustPod := NewDRAConsumerPod(apiClient, name, namespace, claimTemplateName).
			WithNodeSelector(nodeSelector)

		_, err := exhaustPod.Create()
		if err != nil {
			return fmt.Errorf("failed to create exhaust pod %s: %w", name, err)
		}
	}

	for i := 0; i < deviceCount; i++ {
		name := fmt.Sprintf("exhaust-pod-%d", i)

		err := await.PodRunning(apiClient, name, namespace, timeout)
		if err != nil {
			return fmt.Errorf("exhaust pod %s not running: %w", name, err)
		}
	}

	klog.V(params.NeuronLogLevel).Infof("All %d exhaust pods running on %s", deviceCount, targetNode)

	return nil
}
