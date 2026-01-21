package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// NeuronNodesExist checks if there are nodes with Neuron label.
func NeuronNodesExist(apiClient *clients.Settings) (bool, error) {
	nodeList, err := nodes.List(apiClient, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
	})
	if err != nil {
		return false, err
	}

	return len(nodeList) > 0, nil
}

// GetNeuronNodes returns all nodes with Neuron label.
func GetNeuronNodes(apiClient *clients.Settings) ([]*nodes.Builder, error) {
	return nodes.List(apiClient, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
	})
}

// NodeHasNeuronResources checks if a node has Neuron resources in capacity.
func NodeHasNeuronResources(apiClient *clients.Settings, nodeName string) (bool, error) {
	nodeList, err := nodes.List(apiClient, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list node %s: %w", nodeName, err)
	}

	if len(nodeList) == 0 {
		return false, fmt.Errorf("node %s not found", nodeName)
	}

	node := nodeList[0]
	capacity := node.Object.Status.Capacity

	// Check for Neuron device capacity
	if quantity, ok := capacity[params.NeuronCapacityID]; ok {
		return quantity.Value() > 0, nil
	}

	return false, nil
}

// GetNeuronCapacity returns the Neuron device capacity for a node.
func GetNeuronCapacity(apiClient *clients.Settings, nodeName string) (int64, int64, error) {
	nodeList, err := nodes.List(apiClient, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
	})
	if err != nil || len(nodeList) == 0 {
		return 0, 0, fmt.Errorf("node %s not found", nodeName)
	}

	node := nodeList[0]
	capacity := node.Object.Status.Capacity

	var neuronDevices, neuronCores int64

	if quantity, ok := capacity[params.NeuronCapacityID]; ok {
		neuronDevices = quantity.Value()
	}

	if quantity, ok := capacity[params.NeuronCoreCapacityID]; ok {
		neuronCores = quantity.Value()
	}

	return neuronDevices, neuronCores, nil
}

// GetTotalNeuronCores returns the total Neuron cores available across all Neuron-labeled nodes.
func GetTotalNeuronCores(apiClient *clients.Settings) (int64, int64, error) {
	neuronNodes, err := GetNeuronNodes(apiClient)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get Neuron nodes: %w", err)
	}

	var totalDevices, totalCores int64

	for _, node := range neuronNodes {
		devices, cores, err := GetNeuronCapacity(apiClient, node.Object.Name)
		if err != nil {
			klog.V(params.NeuronLogLevel).Infof("Failed to get capacity for node %s: %v",
				node.Object.Name, err)

			continue
		}

		totalDevices += devices
		totalCores += cores
		klog.V(params.NeuronLogLevel).Infof("Node %s: %d devices, %d cores",
			node.Object.Name, devices, cores)
	}

	return totalDevices, totalCores, nil
}

// DevicePluginPodsRunning checks if device plugin pods are running on all Neuron nodes.
func DevicePluginPodsRunning(apiClient *clients.Settings) (bool, error) {
	neuronNodes, err := GetNeuronNodes(apiClient)
	if err != nil {
		return false, fmt.Errorf("failed to get Neuron nodes: %w", err)
	}

	if len(neuronNodes) == 0 {
		return false, fmt.Errorf("no Neuron-labeled nodes found")
	}

	pods, err := pod.List(apiClient, params.NeuronNamespace, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list pods in namespace %s: %w", params.NeuronNamespace, err)
	}

	// Count running device plugin pods per node
	runningPodsPerNode := make(map[string]bool)

	for _, p := range pods {
		if strings.HasPrefix(p.Object.Name, params.DevicePluginDaemonSetPrefix) ||
			strings.Contains(p.Object.Name, "device-plugin") {
			if p.Object.Status.Phase == corev1.PodRunning {
				runningPodsPerNode[p.Object.Spec.NodeName] = true
			}
		}
	}

	// Verify all Neuron nodes have a running device plugin pod
	for _, node := range neuronNodes {
		if !runningPodsPerNode[node.Object.Name] {
			klog.V(params.NeuronLogLevel).Infof("Node %s does not have a running device plugin pod",
				node.Object.Name)

			return false, nil
		}
	}

	return true, nil
}

// MetricsPodsRunning checks if metrics pods are running on all Neuron nodes.
func MetricsPodsRunning(apiClient *clients.Settings) (bool, error) {
	neuronNodes, err := GetNeuronNodes(apiClient)
	if err != nil {
		return false, fmt.Errorf("failed to get Neuron nodes: %w", err)
	}

	if len(neuronNodes) == 0 {
		return false, fmt.Errorf("no Neuron-labeled nodes found")
	}

	pods, err := pod.List(apiClient, params.NeuronNamespace, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list pods in namespace %s: %w", params.NeuronNamespace, err)
	}

	// Count running metrics pods per node
	runningPodsPerNode := make(map[string]bool)

	for _, p := range pods {
		if strings.HasPrefix(p.Object.Name, params.MetricsDaemonSetPrefix) ||
			strings.Contains(p.Object.Name, "monitor") {
			if p.Object.Status.Phase == corev1.PodRunning {
				runningPodsPerNode[p.Object.Spec.NodeName] = true
			}
		}
	}

	// Verify all Neuron nodes have a running metrics pod
	for _, node := range neuronNodes {
		if !runningPodsPerNode[node.Object.Name] {
			klog.V(params.NeuronLogLevel).Infof("Node %s does not have a running metrics pod",
				node.Object.Name)

			return false, nil
		}
	}

	return true, nil
}

// PodHealthy checks if a pod is healthy (running with all containers ready).
func PodHealthy(apiClient *clients.Settings, name, namespace string) (bool, error) {
	podBuilder, err := pod.Pull(apiClient, name, namespace)
	if err != nil {
		return false, fmt.Errorf("failed to pull pod %s in namespace %s: %w", name, namespace, err)
	}

	if podBuilder.Object.Status.Phase != corev1.PodRunning {
		return false, nil
	}

	// Check all containers are ready
	for _, containerStatus := range podBuilder.Object.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false, nil
		}
	}

	return true, nil
}

// PodRestartCount returns the total restart count for a pod.
func PodRestartCount(apiClient *clients.Settings, name, namespace string) (int32, error) {
	podBuilder, err := pod.Pull(apiClient, name, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to pull pod %s in namespace %s: %w", name, namespace, err)
	}

	var totalRestarts int32

	for _, containerStatus := range podBuilder.Object.Status.ContainerStatuses {
		totalRestarts += containerStatus.RestartCount
	}

	return totalRestarts, nil
}

// DaemonSetReady checks if a DaemonSet is fully ready.
func DaemonSetReady(apiClient *clients.Settings, name, namespace string) (bool, error) {
	dsBuilder, err := daemonset.Pull(apiClient, name, namespace)
	if err != nil {
		return false, fmt.Errorf("failed to pull daemonset %s in namespace %s: %w", name, namespace, err)
	}

	return dsBuilder.Object.Status.DesiredNumberScheduled > 0 &&
		dsBuilder.Object.Status.NumberReady == dsBuilder.Object.Status.DesiredNumberScheduled, nil
}

// PodsOnNodeWithLabel returns pods on a specific node matching a label selector.
func PodsOnNodeWithLabel(apiClient *clients.Settings, nodeName, namespace string,
	labelSelector map[string]string) ([]*pod.Builder, error) {
	allPods, err := pod.List(apiClient, namespace, metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s with labels %v: %w", namespace, labelSelector, err)
	}

	var nodePods []*pod.Builder

	for _, p := range allPods {
		if p.Object.Spec.NodeName == nodeName {
			nodePods = append(nodePods, p)
		}
	}

	return nodePods, nil
}

// NeuronWorkloadsOnNode returns count of Neuron workloads on a specific node.
func NeuronWorkloadsOnNode(apiClient *clients.Settings, nodeName, namespace string) (int, error) {
	pods, err := pod.List(apiClient, namespace, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list pods on node %s in namespace %s: %w", nodeName, namespace, err)
	}

	count := 0

	for _, p := range pods {
		// Check if pod requests Neuron resources
		for _, container := range p.Object.Spec.Containers {
			if _, ok := container.Resources.Requests[params.NeuronCapacityID]; ok {
				count++

				break
			}

			if _, ok := container.Resources.Limits[params.NeuronCapacityID]; ok {
				count++

				break
			}
		}
	}

	return count, nil
}

// ServiceMonitorExists checks if a ServiceMonitor exists.
func ServiceMonitorExists(apiClient *clients.Settings, name, namespace string) (bool, error) {
	_, err := apiClient.Resource(neuronparams.ServiceMonitorGVR).
		Namespace(namespace).
		Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
