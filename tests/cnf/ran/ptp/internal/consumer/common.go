// Package consumer provides helpers for deploying the cloud-event-consumer service on a cluster with potentially
// multiple nodes. We want to have a deployment with a single replica per node corresponding to each linuxptp-daemon
// deployment. However, a DaemonSet is not suitable because each a separate service is needed for each node.
package consumer

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
// selector label. It returns an error if the pod list is empty or contains more than one pod.
func GetConsumerPodforNode(client *clients.Settings, nodeName string) (*pod.Builder, error) {
	podList, err := pod.List(client, tsparams.CloudEventsNamespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(getConsumerSelectorLabels(nodeName)).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list consumer pods: %w", err)
	}

	if len(podList) != 1 {
		return nil, fmt.Errorf("expected 1 consumer pod on node %s, got %d", nodeName, len(podList))
	}

	return podList[0], nil
}

// ListConsumerPods lists all the consumer pods in the cluster. It returns an error if there are no consumer pods found.
func ListConsumerPods(client *clients.Settings) ([]*pod.Builder, error) {
	podList, err := pod.List(client, tsparams.CloudEventsNamespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{consumerLabel: ""}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list consumer pods: %w", err)
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf("no consumer pods found in the cluster")
	}

	return podList, nil
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
