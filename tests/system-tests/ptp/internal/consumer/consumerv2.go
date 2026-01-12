package consumer

import (
	"errors"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
)

// DeployV2ConsumersOnNodes deploys the cloud-event-consumer deployment and service on all nodes with PTP daemons in the
// cluster, assuming the event API version is v2. It accumulates errors during deployment and returns them all at once,
// so one deployment failing does not mean deployments fail for all nodes.
func DeployV2ConsumersOnNodes(client *clients.Settings) error {
	err := createConsumerNamespace(client)
	if err != nil {
		return fmt.Errorf("failed to deploy consumer namespace: %w", err)
	}

	nodes, err := listPtpDaemonNodes(client)
	if err != nil {
		return fmt.Errorf("failed to list nodes for consumer deployment: %w", err)
	}

	var deployErrors []error

	for _, node := range nodes {
		err = createV2ConsumerServiceOnNode(client, node.Definition.Name)
		if err != nil {
			deployErrors = append(deployErrors,
				fmt.Errorf("failed to create consumer service on node %s: %w", node.Definition.Name, err))

			continue
		}

		err = createV2ConsumerDeploymentOnNode(client, node.Definition.Name)
		if err != nil {
			deployErrors = append(deployErrors,
				fmt.Errorf("failed to create consumer deployment on node %s: %w", node.Definition.Name, err))

			continue
		}
	}

	return errors.Join(deployErrors...)
}

// CleanupV2ConsumersOnNodes deletes the cloud-event-consumer deployment and service on all nodes with PTP daemons in
// the cluster, assuming the event API version is v2. It accumulates errors during deletion and returns them all at
// once, so one deletion failing does not mean deletions fail for all nodes.
func CleanupV2ConsumersOnNodes(client *clients.Settings) error {
	nodes, err := listPtpDaemonNodes(client)
	if err != nil {
		return fmt.Errorf("failed to list nodes for consumer cleanup: %w", err)
	}

	var cleanupErrors []error

	for _, node := range nodes {
		err = deleteV2ConsumerDeploymentOnNode(client, node.Definition.Name)
		if err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("failed to delete consumer deployment on node %s: %w", node.Definition.Name, err))

			continue
		}

		err = deleteV2ConsumerServiceOnNode(client, node.Definition.Name)
		if err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("failed to delete consumer service on node %s: %w", node.Definition.Name, err))

			continue
		}
	}

	err = deleteConsumerNamespace(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete consumer namespace: %w", err))
	}

	return errors.Join(cleanupErrors...)
}

// createV2ConsumerDeploymentOnNode creates a new deployment for the cloud-event-consumer deployment with a specific
// node selected. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/main/examples/manifests/consumer.yaml.
func createV2ConsumerDeploymentOnNode(client *clients.Settings, nodeName string) error {
	v2ConsumerImage := RANConfig.PtpEventConsumerImage + RANConfig.PtpEventConsumerV2Tag

	consumerContainer, err := pod.NewContainerBuilder(
		"cloud-event-consumer", v2ConsumerImage, []string{"./cloud-event-consumer"}).
		WithPorts([]corev1.ContainerPort{{
			Name:          consumerPortName,
			ContainerPort: consumerPort,
		}}).
		WithImagePullPolicy(corev1.PullAlways).
		WithEnvVar("CONSUMER_TYPE", "PTP").
		WithEnvVar("ENABLE_STATUS_CHECK", "true").
		WithEnvVar("NODE_NAME", nodeName).
		GetContainerCfg()
	if err != nil {
		return fmt.Errorf("failed to create consumer container: %w", err)
	}

	apiAddr := fmt.Sprintf("--local-api-addr=%s.%s.svc.cluster.local:9043",
		getConsumerServiceName(nodeName), tsparams.CloudEventsNamespace)
	consumerContainer.Args = []string{
		apiAddr,
		"--api-path=/api/ocloudNotifications/v2/",
		"--http-event-publishers=ptp-event-publisher-service-NODE_NAME.openshift-ptp.svc.cluster.local:9043",
	}
	consumerContainer.SecurityContext = nil

	consumerDeployment := deployment.NewBuilder(
		client,
		getConsumerDeploymentName(nodeName),
		tsparams.CloudEventsNamespace,
		getConsumerSelectorLabels(nodeName),
		*consumerContainer).
		WithReplicas(1).
		WithAffinity(&corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchFields: []corev1.NodeSelectorRequirement{{
							Key:      "metadata.name",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{nodeName},
						}},
					}},
				},
			},
		})
	consumerDeployment.Definition.Spec.Template.ObjectMeta.Annotations = workloadManagementAnnotation

	_, err = consumerDeployment.CreateAndWaitUntilReady(createDeleteTimeout)
	if err != nil {
		return fmt.Errorf("failed to create consumer deployment: %w", err)
	}

	return nil
}

// deleteV2ConsumerDeploymentOnNode deletes the cloud-event-consumer deployment with a specific node selected. It is
// the inverse of createV2ConsumerDeploymentOnNode.
func deleteV2ConsumerDeploymentOnNode(client *clients.Settings, nodeName string) error {
	consumerDeployment, err := deployment.Pull(client, getConsumerDeploymentName(nodeName), tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = consumerDeployment.DeleteAndWait(createDeleteTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete consumer deployment: %w", err)
	}

	return nil
}

// createV2ConsumerServiceOnNode creates a new service for the cloud-event-consumer deployment with a specific node
// selected. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/main/examples/manifests/service.yaml.
func createV2ConsumerServiceOnNode(client *clients.Settings, nodeName string) error {
	_, err := service.NewBuilder(
		client,
		getConsumerServiceName(nodeName),
		tsparams.CloudEventsNamespace,
		getConsumerSelectorLabels(nodeName),
		corev1.ServicePort{Name: consumerPortName, Port: consumerPort}).
		WithAnnotation(map[string]string{"prometheus.io/scrape": "true"}).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create consumer service: %w", err)
	}

	return nil
}

// deleteV2ConsumerServiceOnNode deletes the cloud-event-consumer service with a specific node selected. It is the
// inverse of createV2ConsumerServiceOnNode.
func deleteV2ConsumerServiceOnNode(client *clients.Settings, nodeName string) error {
	consumerService, err := service.Pull(client, getConsumerServiceName(nodeName), tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = consumerService.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete consumer service: %w", err)
	}

	return nil
}
