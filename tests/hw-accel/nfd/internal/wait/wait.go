package wait

import (
	"context"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// CheckLabel check that feature labels exist on worker nodes.
func CheckLabel(apiClient *clients.Settings,
	timeout time.Duration,
	label string) (bool, error) {
	klog.V(nfdparams.LogLevel).
		Infof("Waiting for feature labels containing '%s' on worker nodes (timeout: %v)",
			label,
			timeout)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nodes, e := apiClient.CoreV1Interface.Nodes().List(
				context.TODO(), metav1.ListOptions{})
			if e != nil {
				return false, e
			}

			workernodes := filterNodesByLabel(nodes, "worker")
			if len(workernodes) == 0 {
				klog.V(nfdparams.LogLevel).Infof("No worker nodes found")

				return false, nil
			}

			nodesWithLabels := 0
			totalWorkerNodes := len(workernodes)

			for _, node := range workernodes {
				featureLabelCount, sampleLabels := countFeatureLabels(node.Labels, label)
				hasFeatureLabels := featureLabelCount > 0

				if hasFeatureLabels {
					nodesWithLabels++

					klog.V(nfdparams.LogLevel).Infof("Node %s has %d feature labels (examples: %v)",
						node.Name, featureLabelCount, sampleLabels)
				} else {
					klog.V(nfdparams.LogLevel).Infof("Node %s does not have feature labels yet", node.Name)
				}
			}

			klog.V(nfdparams.LogLevel).
				Infof("Feature label progress: %d/%d worker nodes have labels",
					nodesWithLabels,
					totalWorkerNodes)

			if nodesWithLabels == totalWorkerNodes {
				klog.V(nfdparams.LogLevel).Infof("SUCCESS: All %d worker nodes have feature labels", totalWorkerNodes)

				return true, nil
			}

			return false, nil
		})
	if err != nil {
		klog.V(nfdparams.LogLevel).Infof("Feature label wait failed: %v", err)

		return false, err
	}

	return true, nil
}

// ForNodeReadiness check that all nodes in namespace are Ready.
func ForNodeReadiness(apiClient *clients.Settings,
	timeout time.Duration,
	nodeSelector map[string]string) (bool, error) {
	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nodes, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(nodeSelector).String()})
			if err != nil {
				return false, err
			}

			checkReadiness := true

			for _, node := range nodes {
				isNodeReady, err := node.IsReady()
				if err != nil {
					return false, err
				}

				checkReadiness = checkReadiness && isNodeReady
			}

			return checkReadiness, nil
		})
	if err != nil {
		return false, err
	}

	return true, nil
}

// ForPodsRunning check that all pods in namespace are in running state.
func ForPodsRunning(apiClient *clients.Settings, timeout time.Duration, nsname string) (bool, error) {
	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := get.PodStatus(apiClient, hwaccelparams.NFDNamespace)
			if err != nil {
				return false, err
			}

			for _, pod := range pods {
				if pod.State != string(corev1.PodRunning) {
					klog.V(nfdparams.LogLevel).Infof("pod %s is in %s state", pod.Name, pod.State)

					return false, nil
				}
			}

			klog.V(nfdparams.LogLevel).Info("all pods are in running status")

			return true, nil
		})
	if err != nil {
		return false, err
	}

	return true, nil
}

func filterNodesByLabel(nodes *corev1.NodeList, keyword string) []corev1.Node {
	var filteredNodes []corev1.Node

	for _, node := range nodes.Items {
		for nodeLabel := range node.Labels {
			if strings.Contains(nodeLabel, keyword) {
				filteredNodes = append(filteredNodes, node)

				break
			}
		}
	}

	return filteredNodes
}

// countFeatureLabels counts how many labels contain the specified substring and returns sample labels for debugging.
func countFeatureLabels(labels map[string]string, labelSubstring string) (int, []string) {
	count := 0
	sampleLabels := []string{}

	for labelKey := range labels {
		if strings.Contains(labelKey, labelSubstring) {
			count++

			if len(sampleLabels) < 3 {
				sampleLabels = append(sampleLabels, labelKey)
			}
		}
	}

	return count, sampleLabels
}
