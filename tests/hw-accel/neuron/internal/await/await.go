package await

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// DevicePluginDeployment waits for the device plugin DaemonSet to be ready.
func DevicePluginDeployment(
	apiClient *clients.Settings,
	namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron device plugin deployment in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing daemonsets: %v", err)

				return false, nil
			}

			for _, daemonSet := range dsList.Items {
				if daemonSet.Status.DesiredNumberScheduled > 0 &&
					daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
					klog.V(params.NeuronLogLevel).Infof(
						"Device plugin DaemonSet %s is ready: %d/%d",
						daemonSet.Name,
						daemonSet.Status.NumberReady,
						daemonSet.Status.DesiredNumberScheduled)

					return true, nil
				}
			}

			return false, nil
		})
}

// MetricsDaemonSet waits for the metrics DaemonSet to be ready.
func MetricsDaemonSet(
	apiClient *clients.Settings,
	namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron metrics DaemonSet in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing daemonsets: %v", err)

				return false, nil
			}

			for _, daemonSet := range dsList.Items {
				if daemonSet.Name == params.MetricsDaemonSetPrefix ||
					neuronparams.HasPrefix(daemonSet.Name, params.MetricsDaemonSetPrefix) {
					if daemonSet.Status.DesiredNumberScheduled > 0 &&
						daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
						klog.V(params.NeuronLogLevel).Infof(
							"Metrics DaemonSet %s is ready", daemonSet.Name)

						return true, nil
					}
				}
			}

			return false, nil
		})
}

// SchedulerDeployment waits for the custom scheduler deployment to be ready.
func SchedulerDeployment(
	apiClient *clients.Settings,
	namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron scheduler deployment in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deploy, err := deployment.Pull(apiClient, params.SchedulerDeploymentName, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Scheduler deployment not found yet: %v", err)

				return false, nil
			}

			if deploy.IsReady(10 * time.Second) {
				klog.V(params.NeuronLogLevel).Info("Scheduler deployment is ready")

				return true, nil
			}

			return false, nil
		})
}

// PodRunning waits for a specific pod to be running.
func PodRunning(
	apiClient *clients.Settings,
	name, namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to be running", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Pod %s not found: %v", name, err)

				return false, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodRunning {
				klog.V(params.NeuronLogLevel).Infof("Pod %s is running", name)

				return true, nil
			}

			klog.V(params.NeuronLogLevel).Infof("Pod %s phase: %s", name, podBuilder.Object.Status.Phase)

			return false, nil
		})
}

// PodReady waits for a specific pod to be ready (all containers pass readiness probes).
func PodReady(
	apiClient *clients.Settings,
	name, namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to be ready (readiness probe passed)", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 30*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Pod %s not found: %v", name, err)

				return false, nil
			}

			// Check if pod is running first
			if podBuilder.Object.Status.Phase != corev1.PodRunning {
				klog.V(params.NeuronLogLevel).Infof("Pod %s phase: %s (waiting for Running)",
					name, podBuilder.Object.Status.Phase)

				return false, nil
			}

		// Check if all containers are ready
		for _, cond := range podBuilder.Object.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					klog.V(params.NeuronLogLevel).Infof("Pod %s is ready", name)

					return true, nil
				}
			}

			// Log container statuses for debugging
			for _, cs := range podBuilder.Object.Status.ContainerStatuses {
				klog.V(params.NeuronLogLevel).Infof("Pod %s container %s: ready=%v, restarts=%d",
					name, cs.Name, cs.Ready, cs.RestartCount)
			}

			return false, nil
		})
}

// PodCompleted waits for a pod to complete successfully.
func PodCompleted(
	apiClient *clients.Settings,
	name, namespace string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to complete", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				return false, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodSucceeded {
				return true, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodFailed {
				return false, fmt.Errorf("pod %s failed", name)
			}

			return false, nil
		})
}

// PodsDeleted waits for all pods with given label to be deleted from a namespace.
func PodsDeleted(
	apiClient *clients.Settings,
	namespace string,
	labelSelector map[string]string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for pods to be deleted from namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, namespace, metav1.ListOptions{
				LabelSelector: labels.Set(labelSelector).String(),
			})
			if err != nil {
				return false, nil
			}

			if len(pods) == 0 {
				klog.V(params.NeuronLogLevel).Info("All pods deleted")

				return true, nil
			}

			klog.V(params.NeuronLogLevel).Infof("Still %d pods remaining", len(pods))

			return false, nil
		})
}

// NeuronNodesLabeled waits for at least one node to be labeled with Neuron label.
func NeuronNodesLabeled(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for nodes to be labeled with Neuron feature")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s",
					params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
			})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing nodes: %v", err)

				return false, nil
			}

			if len(nodeList) > 0 {
				klog.V(params.NeuronLogLevel).Infof("Found %d Neuron-labeled nodes", len(nodeList))

				return true, nil
			}

			return false, nil
		})
}

// NodeResourceAvailable waits for a node to have the Neuron resource available.
func NodeResourceAvailable(
	apiClient *clients.Settings,
	nodeName string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for Neuron resources on node %s", nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			node := nodeList[0]
			capacity := node.Object.Status.Capacity

			if quantity, ok := capacity[params.NeuronCapacityID]; ok {
				if quantity.Value() > 0 {
					klog.V(params.NeuronLogLevel).Infof("Node %s has Neuron capacity: %d",
						nodeName, quantity.Value())

					return true, nil
				}
			}

			return false, nil
		})
}

// AllNeuronNodesResourceAvailable waits for all Neuron-labeled nodes to have resources.
func AllNeuronNodesResourceAvailable(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for all Neuron nodes to have resources available")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s",
					params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			for _, node := range nodeList {
				capacity := node.Object.Status.Capacity
				if quantity, ok := capacity[params.NeuronCapacityID]; !ok || quantity.Value() == 0 {
					klog.V(params.NeuronLogLevel).Infof("Node %s does not have Neuron resources yet",
						node.Object.Name)

					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Infof(
				"All %d Neuron nodes have resources available", len(nodeList))

			return true, nil
		})
}

// DriverVersionOnNode waits for a specific driver version on a node.
func DriverVersionOnNode(
	apiClient *clients.Settings,
	nodeName, expectedVersion string,
	timeout time.Duration,
) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for driver version %s on node %s", expectedVersion, nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			// Check device plugin pods on this node
			pods, err := pod.List(apiClient, params.NeuronNamespace, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
			})
			if err != nil {
				return false, nil
			}

			for _, currentPod := range pods {
				// Check for device plugin pod
				if neuronparams.HasPrefix(currentPod.Object.Name, params.DevicePluginDaemonSetPrefix) {
					if currentPod.Object.Status.Phase == corev1.PodRunning {
						klog.V(params.NeuronLogLevel).Infof(
							"Device plugin pod running on node %s", nodeName)

						return true, nil
					}
				}
			}

			return false, nil
		})
}
