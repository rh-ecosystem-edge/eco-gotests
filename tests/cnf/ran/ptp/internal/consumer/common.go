// Package consumer provides helpers for deploying the cloud-event-consumer service on a cluster with potentially
// multiple nodes. We want to have a deployment with a single replica per node corresponding to each linuxptp-daemon
// deployment. However, a DaemonSet is not suitable because each a separate service is needed for each node.
package consumer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	consumerPortName    = "sub-port"
	consumerPort        = 9043
	createDeleteTimeout = 5 * time.Minute
	consumerLabel       = "cloud-events-consumer"
)

var workloadManagementAnnotation = map[string]string{
	"target.workload.openshift.io/management": `{"effect": "PreferredDuringScheduling"}`,
}

// GetConsumerPodforNode returns the cloud-event-consumer pod for a specific node. It lists pods using the node-specific
// selector label, ignoring pods that are terminating or in a terminal phase. It returns an error if the active pod
// list is empty or contains more than one pod.
func GetConsumerPodforNode(client *clients.Settings, nodeName string) (*pod.Builder, error) {
	return activeConsumerPodOnNode(client, nodeName)
}

// WaitForActiveConsumerPodOnNode polls until a single running consumer pod exists on the node. After events such as a
// node reboot, stale terminating pods may remain while the replacement pod is still starting.
func WaitForActiveConsumerPodOnNode(
	client *clients.Settings, nodeName string, timeout time.Duration) (*pod.Builder, error) {
	var activePod *pod.Builder

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			consumerPod, activeCount, _, err := lookupActiveConsumerPodOnNode(client, nodeName)
			if err != nil {
				return false, err
			}

			if activeCount == 1 {
				activePod = consumerPod

				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("timed out waiting for active consumer pod on node %s: %w", nodeName, err)
	}

	return activePod, nil
}

func activeConsumerPodOnNode(client *clients.Settings, nodeName string) (*pod.Builder, error) {
	consumerPod, activeCount, totalCount, err := lookupActiveConsumerPodOnNode(client, nodeName)
	if err != nil {
		return nil, err
	}

	if activeCount != 1 {
		return nil, fmt.Errorf("expected 1 active consumer pod on node %s, got %d (%d pods total including terminating)",
			nodeName, activeCount, totalCount)
	}

	return consumerPod, nil
}

// lookupActiveConsumerPodOnNode returns the single active consumer pod when exactly one is running. activeCount is zero
// while terminating pods are being replaced and the new pod is still starting. An error is returned when more than one
// running pod exists.
func lookupActiveConsumerPodOnNode(client *clients.Settings, nodeName string) (*pod.Builder, int, int, error) {
	podList, err := listConsumerPodsForNode(client, nodeName)
	if err != nil {
		return nil, 0, 0, err
	}

	activePods := filterActiveConsumerPods(podList)

	activeCount := len(activePods)
	if activeCount > 1 {
		return nil, activeCount, len(podList), fmt.Errorf(
			"expected 1 active consumer pod on node %s, got %d (%d pods total including terminating)",
			nodeName, activeCount, len(podList))
	}

	if activeCount == 0 {
		return nil, 0, len(podList), nil
	}

	return activePods[0], 1, len(podList), nil
}

// ListConsumerPods lists all active consumer pods in the cluster. Terminating pods are excluded. It returns an error if
// there are no active consumer pods found.
func ListConsumerPods(client *clients.Settings) ([]*pod.Builder, error) {
	podList, err := pod.List(client, tsparams.CloudEventsNamespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{consumerLabel: ""}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list consumer pods: %w", err)
	}

	activePods := filterActiveConsumerPods(podList)
	if len(activePods) == 0 {
		return nil, fmt.Errorf("no active consumer pods found in the cluster")
	}

	return activePods, nil
}

func listConsumerPodsForNode(client *clients.Settings, nodeName string) ([]*pod.Builder, error) {
	podList, err := pod.List(client, tsparams.CloudEventsNamespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(getConsumerSelectorLabels(nodeName)).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list consumer pods: %w", err)
	}

	return podList, nil
}

// filterActiveConsumerPods returns pods that are not terminating and are still eligible to receive events (running).
func filterActiveConsumerPods(podList []*pod.Builder) []*pod.Builder {
	var activePods []*pod.Builder

	for _, consumerPod := range podList {
		if consumerPod == nil || consumerPod.Object == nil {
			continue
		}

		if consumerPod.Object.DeletionTimestamp != nil {
			continue
		}

		if consumerPod.Object.Status.Phase != corev1.PodRunning {
			continue
		}

		activePods = append(activePods, consumerPod)
	}

	return activePods
}

// waitForConsumerPodsRemovedOnNode waits until no consumer pods remain for the given node. Deployment deletion does
// not wait for pods to terminate; callers must wait before redeploying to avoid duplicate pods during rollout.
func waitForConsumerPodsRemovedOnNode(client *clients.Settings, nodeName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			podList, err := listConsumerPodsForNode(client, nodeName)
			if err != nil {
				return false, err
			}

			return len(podList) == 0, nil
		})
}

// AreEventsEnabled checks if events are enabled in the PTP operator config. Events are considered enabled if and only
// if PtpOperatorConfig.Spec.EventConfig is non-nil and PtpOperatorConfig.Spec.EventConfig.EnableEventPublisher is true.
func AreEventsEnabled(client *clients.Settings) (bool, error) {
	ptpOperatorConfig, err := ptp.PullPtpOperatorConfig(client)
	if err != nil {
		return false, fmt.Errorf("failed to pull PTP operator config: %w", err)
	}

	if ptpOperatorConfig.Definition == nil {
		return false, fmt.Errorf("PTP operator config definition is nil")
	}

	if ptpOperatorConfig.Definition.Spec.EventConfig == nil {
		return false, nil
	}

	return ptpOperatorConfig.Definition.Spec.EventConfig.EnableEventPublisher, nil
}

// createConsumerNamespace creates the cloud-events namespace with the necessary labels and annotations. It uses the
// definition from https://github.com/redhat-cne/cloud-event-proxy/blob/main/examples/manifests/namespace.yaml.
func createConsumerNamespace(client *clients.Settings) error {
	consumerNamespace := namespace.NewBuilder(client, tsparams.CloudEventsNamespace).
		WithMultipleLabels(map[string]string{
			"security.openshift.io/scc.podSecurityLabelSync": "false",
			"pod-security.kubernetes.io/audit":               "privileged",
			"pod-security.kubernetes.io/enforce":             "privileged",
			"pod-security.kubernetes.io/warn":                "privileged",
			"name":                                           tsparams.CloudEventsNamespace,
			"openshift.io/cluster-monitoring":                "true",
		})
	consumerNamespace.Definition.SetAnnotations(map[string]string{
		"workload.openshift.io/allowed": "management",
	})

	_, err := consumerNamespace.Create()
	if err != nil {
		return fmt.Errorf("failed to create cloud events namespace: %w", err)
	}

	return nil
}

// deleteConsumerNamespace deletes the cloud-events namespace. It is the inverse of createConsumerNamespace.
func deleteConsumerNamespace(client *clients.Settings) error {
	consumerNamespace, err := namespace.Pull(client, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	return consumerNamespace.DeleteAndWait(5 * time.Minute)
}

// getConsumerDeploymentName returns the name of the cloud-event-consumer deployment for a specific node. Similar to how
// the container uses NODE_NAME, it splits the node name by the dot and takes the first part.
func getConsumerDeploymentName(nodeName string) string {
	return fmt.Sprintf("cloud-event-consumer-%s", strings.Split(nodeName, ".")[0])
}

// getConsumerSelectorLabels returns the labels used to select the cloud-event-consumer deployment for a specific node.
// This allows for services to be tied to the deployment specifically for that node. It additionally adds the label
// "cloud-events-consumer" to the labels to allow for easy selection of all consumer pods.
func getConsumerSelectorLabels(nodeName string) map[string]string {
	return map[string]string{
		"app":         getConsumerDeploymentName(nodeName),
		consumerLabel: "",
	}
}

// getConsumerServiceName returns the name of the cloud-event-consumer service for a specific node. Similar to how
// the container uses NODE_NAME, it splits the node name by the dot and takes the first part.
func getConsumerServiceName(nodeName string) string {
	return fmt.Sprintf("consumer-events-service-%s", strings.Split(nodeName, ".")[0])
}

func getConsumerServingCertsSecretName(nodeName string) string {
	return fmt.Sprintf("consumer-events-serving-certs-%s", strings.Split(nodeName, ".")[0])
}

func getConsumerServicesAnnotations(nodeName string) map[string]string {
	return map[string]string{
		"prometheus.io/scrape":                                "true",
		"service.alpha.openshift.io/serving-cert-secret-name": getConsumerServingCertsSecretName(nodeName),
	}
}

func getSidecarServiceMonitorName(nodeName string) string {
	return fmt.Sprintf("consumer-sidecar-service-monitor-%s", strings.Split(nodeName, ".")[0])
}

// getConsumerEventsSubscriptionServiceName returns the name of the consumer-events-subscription-service for a specific
// node. Similar to how the container uses NODE_NAME, it splits the node name by the dot and takes the first part.
func getConsumerEventsSubscriptionServiceName(nodeName string) string {
	return fmt.Sprintf("consumer-events-subscription-service-%s", strings.Split(nodeName, ".")[0])
}

// getConsumerSidecarServiceName returns the name of the consumer-sidecar-service for a specific node. Similar to how
// the container uses NODE_NAME, it splits the node name by the dot and takes the first part.
func getConsumerSidecarServiceName(nodeName string) string {
	return fmt.Sprintf("consumer-sidecar-service-%s", strings.Split(nodeName, ".")[0])
}

// listPtpDaemonNodes lists the nodes that the PTP daemon is deployed on. It uses the PtpOperatorConfig
// spec.daemonNodeSelector.
func listPtpDaemonNodes(client *clients.Settings) ([]*nodes.Builder, error) {
	ptpOperatorConfig, err := ptp.PullPtpOperatorConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PtpOperatorConfig: %w", err)
	}

	// Selector is a set of labels where matching nodes will have the PTP daemon deployed. A nil or empty selector
	// will match all nodes.
	selector := ptpOperatorConfig.Definition.Spec.DaemonNodeSelector

	nodeList, err := nodes.List(client, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes matching PtpOperatorConfig spec.daemonNodeSelector: %w", err)
	}

	if len(nodeList) == 0 {
		return nil, fmt.Errorf("no nodes match PtpOperatorConfig spec.daemonNodeSelector %v", selector)
	}

	return nodeList, nil
}
