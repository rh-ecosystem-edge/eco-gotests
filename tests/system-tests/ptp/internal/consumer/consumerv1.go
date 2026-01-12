package consumer

import (
	"errors"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/monitoring"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DeployV1ConsumersOnNodes deploys the cloud-event-consumer deployment and service on all nodes with PTP daemons in the
// cluster, assuming the event API version is v1.
//
// It works by first deploying the cluster-level resources required for the consumer using [deployV1ConsumerOnCluster].
// Next, it lists the nodes with PTP daemons and deploys the consumer on each node using [deployV1ConsumerOnNode].
func DeployV1ConsumersOnNodes(client *clients.Settings) error {
	err := deployV1ConsumerOnCluster(client)
	if err != nil {
		return err
	}

	images, err := getEventImagesFromPtpDaemon(client)
	if err != nil {
		return fmt.Errorf("failed to get event images: %w", err)
	}

	nodes, err := listPtpDaemonNodes(client)
	if err != nil {
		return fmt.Errorf("failed to list nodes for consumer deployment: %w", err)
	}

	var deployErrors []error

	for _, node := range nodes {
		// errors.Join ignores nil errors so we can append regardless of whether err is nil.
		deployErrors = append(deployErrors, deployV1ConsumerOnNode(client, node.Definition.Name, images))
	}

	return errors.Join(deployErrors...)
}

// CleanupV1ConsumersOnNodes deletes the cloud-event-consumer deployment and service on all nodes with PTP daemons in
// the cluster, assuming the event API version is v1.
//
// It works by first listing the nodes with PTP daemons and cleaning up the consumer on each node using
// [cleanupV1ConsumerOnNode]. Next, it cleans up the cluster-level resources created by [deployV1ConsumerOnCluster]
// using [cleanupV1ConsumerOnCluster].
func CleanupV1ConsumersOnNodes(client *clients.Settings) error {
	nodes, err := listPtpDaemonNodes(client)
	if err != nil {
		return fmt.Errorf("failed to list nodes for consumer cleanup: %w", err)
	}

	var cleanupErrors []error

	for _, node := range nodes {
		cleanupErrors = append(cleanupErrors, cleanupV1ConsumerOnNode(client, node.Definition.Name))
	}

	cleanupErrors = append(cleanupErrors, cleanupV1ConsumerOnCluster(client)...)

	return errors.Join(cleanupErrors...)
}

// deployV1ConsumerOnCluster creates all cluster-level resources required for V1 consumers. In creates the following
// resources in order:
//
//  1. namespace using [createConsumerNamespace].
//  2. sidecar service account using [createV1SidecarConsumerServiceAccount].
//  3. sidecar cluster role using [createV1SidecarConsumerClusterRole].
//  4. sidecar cluster role binding using [createV1SidecarConsumerClusterRoleBinding].
//  5. prometheus role using [createV1PrometheusRole].
//  6. prometheus role binding using [createV1PrometheusRoleBinding].
//
// If any of these fail, the function returns an error immediately.
func deployV1ConsumerOnCluster(client *clients.Settings) error {
	err := createConsumerNamespace(client)
	if err != nil {
		return fmt.Errorf("failed to deploy consumer namespace: %w", err)
	}

	err = createV1SidecarConsumerServiceAccount(client)
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	err = createV1SidecarConsumerClusterRole(client)
	if err != nil {
		return fmt.Errorf("failed to create cluster role: %w", err)
	}

	err = createV1SidecarConsumerClusterRoleBinding(client)
	if err != nil {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	err = createV1PrometheusRole(client)
	if err != nil {
		return fmt.Errorf("failed to create prometheus role: %w", err)
	}

	err = createV1PrometheusRoleBinding(client)
	if err != nil {
		return fmt.Errorf("failed to create prometheus role binding: %w", err)
	}

	return nil
}

// cleanupV1ConsumerOnCluster removes all cluster-level resources created by deployV1ConsumerOnCluster. It deletes the
// following resources in order:
//
//  1. prometheus role binding using [deleteV1PrometheusRoleBinding].
//  2. prometheus role using [deleteV1PrometheusRole].
//  3. sidecar cluster role binding using [deleteV1SidecarConsumerClusterRoleBinding].
//  4. sidecar cluster role using [deleteV1SidecarConsumerClusterRole].
//  5. sidecar service account using [deleteV1SidecarConsumerServiceAccount].
//  6. consumer namespace using [deleteConsumerNamespace].
//
// If any of these fail, the function accumulates the errors and returns them all at once.
func cleanupV1ConsumerOnCluster(client *clients.Settings) []error {
	var cleanupErrors []error

	err := deleteV1PrometheusRoleBinding(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete prometheus role binding: %w", err))
	}

	err = deleteV1PrometheusRole(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete prometheus role: %w", err))
	}

	err = deleteV1SidecarConsumerClusterRoleBinding(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete cluster role binding: %w", err))
	}

	err = deleteV1SidecarConsumerClusterRole(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete cluster role: %w", err))
	}

	err = deleteV1SidecarConsumerServiceAccount(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete service account: %w", err))
	}

	err = deleteConsumerNamespace(client)
	if err != nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("failed to delete consumer namespace: %w", err))
	}

	return cleanupErrors
}

// deployV1ConsumerOnNode deploys all the resources required for a consumer on a specific node with a PTP daemon. It
// creates the following resources in order:
//
//  1. consumer service using [createV1ConsumerServiceOnNode].
//  2. sidecar service using [createV1SidecarServiceOnNode].
//  3. service monitor using [createV1ServiceMonitorOnNode].
//  4. consumer deployment using [createV1ConsumerDeploymentOnNode].
//
// If any of these fail, the function returns an error immediately.
func deployV1ConsumerOnNode(client *clients.Settings, nodeName string, images *eventImages) error {
	err := createV1ConsumerServiceOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to create consumer service on node %s: %w", nodeName, err)
	}

	err = createV1SidecarServiceOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to create sidecar service on node %s: %w", nodeName, err)
	}

	err = createV1ServiceMonitorOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to create service monitor on node %s: %w", nodeName, err)
	}

	err = createV1ConsumerDeploymentOnNode(client, nodeName, images)
	if err != nil {
		return fmt.Errorf("failed to create consumer deployment on node %s: %w", nodeName, err)
	}

	return nil
}

// cleanupV1ConsumerOnNode removes all the resources created by deployV1ConsumerOnNode on a specific node with a PTP
// daemon. It deletes the following resources in order:
//
//  1. consumer deployment using [deleteV1ConsumerDeploymentOnNode].
//  2. service monitor using [deleteV1ServiceMonitorOnNode].
//  3. sidecar service using [deleteV1SidecarServiceOnNode].
//  4. consumer service using [deleteV1ConsumerServiceOnNode].
//
// If any of these fail, the function accumulates the errors and returns them all at once.
func cleanupV1ConsumerOnNode(client *clients.Settings, nodeName string) error {
	err := deleteV1ConsumerDeploymentOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete consumer deployment on node %s: %w", nodeName, err)
	}

	err = deleteV1ServiceMonitorOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete service monitor on node %s: %w", nodeName, err)
	}

	err = deleteV1SidecarServiceOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete sidecar service on node %s: %w", nodeName, err)
	}

	err = deleteV1ConsumerServiceOnNode(client, nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete consumer service on node %s: %w", nodeName, err)
	}

	return nil
}

// createV1ConsumerDeploymentOnNode creates a new deployment for the cloud-event-consumer deployment with a specific
// node selected. When creating, it waits for [createDeleteTimeout] until the deployment is ready. It uses the
// definition from https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/consumer.yaml.
func createV1ConsumerDeploymentOnNode(client *clients.Settings, nodeName string, images *eventImages) error {
	consumerContainer, err := defineConsumerContainer(nodeName)
	if err != nil {
		return err
	}

	sidecarContainer, err := defineSidecarContainer(nodeName, images.cloudEventProxy)
	if err != nil {
		return err
	}

	rbacProxyContainer, err := defineRBACProxyContainer(images.kubeRBACProxy)
	if err != nil {
		return err
	}

	consumerDeployment := deployment.NewBuilder(
		client,
		getConsumerDeploymentName(nodeName),
		tsparams.CloudEventsNamespace,
		getConsumerSelectorLabels(nodeName),
		*consumerContainer,
	).
		WithAdditionalContainerSpecs([]corev1.Container{
			*sidecarContainer,
			*rbacProxyContainer,
		}).
		WithReplicas(1).
		WithServiceAccountName(tsparams.SidecarConsumerSA).
		WithVolume(corev1.Volume{
			Name: "pubsubstore",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}).
		WithVolume(corev1.Volume{
			Name: "sidecar-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: getConsumerServingCertsSecretName(nodeName),
				},
			},
		}).
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

// deleteV1ConsumerDeploymentOnNode deletes the cloud-event-consumer deployment with a specific node selected. It is the
// inverse of [createV1ConsumerDeploymentOnNode] and also waits for [createDeleteTimeout] until the deployment is gone.
func deleteV1ConsumerDeploymentOnNode(client *clients.Settings, nodeName string) error {
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

// createV1ConsumerServiceOnNode creates a new service for the cloud-event-consumer subscription service on a specific
// node. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/service.yaml.
func createV1ConsumerServiceOnNode(client *clients.Settings, nodeName string) error {
	selectorLabels := getConsumerSelectorLabels(nodeName)

	_, err := service.NewBuilder(
		client,
		getConsumerEventsSubscriptionServiceName(nodeName),
		tsparams.CloudEventsNamespace,
		selectorLabels,
		corev1.ServicePort{Name: consumerPortName, Port: consumerPort},
	).
		WithLabels(selectorLabels).
		WithAnnotation(getConsumerServicesAnnotations(nodeName)).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create consumer service: %w", err)
	}

	return nil
}

// deleteV1ConsumerServiceOnNode deletes the cloud-event-consumer subscription service on a specific node. It is the
// inverse of [createV1ConsumerServiceOnNode].
func deleteV1ConsumerServiceOnNode(client *clients.Settings, nodeName string) error {
	serviceName := getConsumerEventsSubscriptionServiceName(nodeName)

	consumerService, err := service.Pull(client, serviceName, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = consumerService.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete consumer service: %w", err)
	}

	return nil
}

// createV1SidecarServiceOnNode creates a new sidecar service on a specific node. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/service.yaml.
func createV1SidecarServiceOnNode(client *clients.Settings, nodeName string) error {
	selectorLabels := getConsumerSelectorLabels(nodeName)

	_, err := service.NewBuilder(
		client,
		getConsumerSidecarServiceName(nodeName),
		tsparams.CloudEventsNamespace,
		selectorLabels,
		corev1.ServicePort{
			Name:       "metrics",
			Port:       8443,
			TargetPort: intstr.FromString("https"),
		},
	).
		WithLabels(selectorLabels).
		WithAnnotation(getConsumerServicesAnnotations(nodeName)).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create metrics service: %w", err)
	}

	return nil
}

// deleteV1SidecarServiceOnNode deletes the cloud-event-consumer metrics service on a specific node. It is the inverse
// of [createV1SidecarServiceOnNode].
func deleteV1SidecarServiceOnNode(client *clients.Settings, nodeName string) error {
	serviceName := getConsumerSidecarServiceName(nodeName)

	metricsService, err := service.Pull(client, serviceName, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = metricsService.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete metrics service: %w", err)
	}

	return nil
}

// createV1ServiceMonitorOnNode creates a new ServiceMonitor for the cloud-event-consumer metrics on a specific node. It
// is based on the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/service.yaml.
func createV1ServiceMonitorOnNode(client *clients.Settings, nodeName string) error {
	serverName := fmt.Sprintf("%s.%s.svc", getConsumerSidecarServiceName(nodeName), tsparams.CloudEventsNamespace)

	_, err := monitoring.NewBuilder(client, getSidecarServiceMonitorName(nodeName), tsparams.CloudEventsNamespace).
		WithLabels(map[string]string{"k8s-app": getSidecarServiceMonitorName(nodeName)}).
		WithSelector(getConsumerSelectorLabels(nodeName)).
		WithNamespaceSelector([]string{tsparams.CloudEventsNamespace}).
		WithEndpoints([]monitoringv1.Endpoint{{
			Interval:        "30s",
			Port:            "metrics",
			BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
			Scheme:          "https",
			TLSConfig: &monitoringv1.TLSConfig{
				CAFile: "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
				SafeTLSConfig: monitoringv1.SafeTLSConfig{
					ServerName: &serverName,
				},
			},
		}}).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create service monitor: %w", err)
	}

	return nil
}

// deleteV1ServiceMonitorOnNode deletes the ServiceMonitor for the cloud-event-consumer metrics on a specific node. It
// is the inverse of [createV1ServiceMonitorOnNode].
func deleteV1ServiceMonitorOnNode(client *clients.Settings, nodeName string) error {
	serviceMonitor, err := monitoring.Pull(client, getSidecarServiceMonitorName(nodeName), tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	_, err = serviceMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service monitor: %w", err)
	}

	return nil
}

// createV1SidecarConsumerServiceAccount creates a ServiceAccount for the sidecar consumer. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/service-account.yaml.
func createV1SidecarConsumerServiceAccount(client *clients.Settings) error {
	_, err := serviceaccount.NewBuilder(client, tsparams.SidecarConsumerSA, tsparams.CloudEventsNamespace).Create()
	if err != nil {
		return fmt.Errorf("failed to create sidecar consumer service account: %w", err)
	}

	return nil
}

// deleteV1SidecarConsumerServiceAccount deletes the ServiceAccount for the sidecar consumer. It is the inverse of
// [createV1SidecarConsumerServiceAccount].
func deleteV1SidecarConsumerServiceAccount(client *clients.Settings) error {
	serviceAccountBuilder, err := serviceaccount.Pull(client, tsparams.SidecarConsumerSA, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = serviceAccountBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete sidecar consumer service account: %w", err)
	}

	return nil
}

// createV1SidecarConsumerClusterRole creates a ClusterRole for the sidecar consumer. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/roles.yaml.
func createV1SidecarConsumerClusterRole(client *clients.Settings) error {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{"authentication.k8s.io"},
			Resources: []string{"tokenreviews"},
			Verbs:     []string{"create"},
		},
		{
			APIGroups: []string{"authorization.k8s.io"},
			Resources: []string{"subjectaccessreviews"},
			Verbs:     []string{"create"},
		},
	}

	_, err := rbac.NewClusterRoleBuilder(client, tsparams.SidecarConsumerRole, rules[0]).WithRules(rules[1:]).Create()
	if err != nil {
		return fmt.Errorf("failed to create sidecar consumer cluster role: %w", err)
	}

	return nil
}

// deleteV1SidecarConsumerClusterRole deletes the ClusterRole for the sidecar consumer. It is the inverse of
// [createV1SidecarConsumerClusterRole].
func deleteV1SidecarConsumerClusterRole(client *clients.Settings) error {
	cr, err := rbac.PullClusterRole(client, tsparams.SidecarConsumerRole)
	if err != nil {
		return nil
	}

	err = cr.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete sidecar consumer cluster role: %w", err)
	}

	return nil
}

// createV1SidecarConsumerClusterRoleBinding creates a ClusterRoleBinding for the sidecar consumer. It uses the
// definition from https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/roles.yaml.
func createV1SidecarConsumerClusterRoleBinding(client *clients.Settings) error {
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      tsparams.SidecarConsumerSA,
		Namespace: tsparams.CloudEventsNamespace,
	}

	_, err := rbac.NewClusterRoleBindingBuilder(
		client, tsparams.SidecarConsumerRoleBinding, tsparams.SidecarConsumerRole, subject).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create sidecar consumer cluster role binding: %w", err)
	}

	return nil
}

// deleteV1SidecarConsumerClusterRoleBinding deletes the ClusterRoleBinding for the sidecar consumer. It is the inverse
// of [createV1SidecarConsumerClusterRoleBinding].
func deleteV1SidecarConsumerClusterRoleBinding(client *clients.Settings) error {
	crb, err := rbac.PullClusterRoleBinding(client, tsparams.SidecarConsumerRoleBinding)
	if err != nil {
		return nil
	}

	err = crb.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete sidecar consumer cluster role binding: %w", err)
	}

	return nil
}

// createV1PrometheusRole creates a Role for Prometheus. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/roles.yaml.
func createV1PrometheusRole(client *clients.Settings) error {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"services", "endpoints", "pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"monitoring.coreos.com"},
			Resources: []string{"servicemonitors"},
			Verbs:     []string{"get", "create"},
		},
	}

	_, err := rbac.NewRoleBuilder(client, tsparams.PrometheusK8s, tsparams.CloudEventsNamespace, rules[0]).
		WithRules(rules[1:]).Create()
	if err != nil {
		return fmt.Errorf("failed to create prometheus role: %w", err)
	}

	return nil
}

// deleteV1PrometheusRole deletes the Role for Prometheus. It is the inverse of [createV1PrometheusRole].
func deleteV1PrometheusRole(client *clients.Settings) error {
	roleBuilder, err := rbac.PullRole(client, tsparams.PrometheusK8s, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = roleBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete prometheus role: %w", err)
	}

	return nil
}

// createV1PrometheusRoleBinding creates a RoleBinding for Prometheus. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/roles.yaml.
func createV1PrometheusRoleBinding(client *clients.Settings) error {
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      tsparams.PrometheusK8s,
		Namespace: ranparam.OpenshiftMonitoringNamespace,
	}

	_, err := rbac.NewRoleBindingBuilder(
		client, tsparams.PrometheusK8s, tsparams.CloudEventsNamespace, tsparams.PrometheusK8s, subject).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create prometheus role binding: %w", err)
	}

	return nil
}

// deleteV1PrometheusRoleBinding deletes the RoleBinding for Prometheus. It is the inverse of
// [createV1PrometheusRoleBinding].
func deleteV1PrometheusRoleBinding(client *clients.Settings) error {
	roleBindingBuilder, err := rbac.PullRoleBinding(client, tsparams.PrometheusK8s, tsparams.CloudEventsNamespace)
	if err != nil {
		return nil
	}

	err = roleBindingBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete prometheus role binding: %w", err)
	}

	return nil
}

// defineConsumerContainer defines the cloud-event-consumer container. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/consumer.yaml.
func defineConsumerContainer(nodeName string) (*corev1.Container, error) {
	v1ConsumerImage := RANConfig.PtpEventConsumerImage + RANConfig.PtpEventConsumerV1Tag

	container, err := pod.NewContainerBuilder(
		"cloud-event-consumer", v1ConsumerImage, []string{"./cloud-event-consumer"}).
		WithImagePullPolicy(corev1.PullAlways).
		WithEnvVar("NODE_NAME", nodeName).
		WithEnvVar("CONSUMER_TYPE", "PTP").
		WithEnvVar("ENABLE_STATUS_CHECK", "true").
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to define the consumer container: %w", err)
	}

	container.SecurityContext = nil
	container.Args = []string{
		"--local-api-addr=127.0.0.1:9089",
		"--api-path=/api/ocloudNotifications/v1/",
		"--api-addr=127.0.0.1:8089",
		"--http-event-publishers=ptp-event-publisher-service-NODE_NAME.openshift-ptp.svc.cluster.local:9043",
	}

	return container, nil
}

// defineSidecarContainer creates the cloud-event-sidecar container. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/consumer.yaml.
func defineSidecarContainer(nodeName string, cloudEventProxyImage string) (*corev1.Container, error) {
	container, err := pod.NewContainerBuilder(
		"cloud-event-sidecar", cloudEventProxyImage, []string{"./cloud-event-sidecar"}).
		WithImagePullPolicy(corev1.PullAlways).
		WithEnvVar("NODE_NAME", nodeName).
		WithPorts([]corev1.ContainerPort{{
			Name:          "metrics-port",
			ContainerPort: 9091,
		}}).
		WithVolumeMount(corev1.VolumeMount{
			Name:      "pubsubstore",
			MountPath: "/store",
		}).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to define the sidecar container: %w", err)
	}

	transportHost := fmt.Sprintf("--transport-host=%s.%s.svc.cluster.local:9043",
		getConsumerServiceName(nodeName), tsparams.CloudEventsNamespace)
	container.Args = []string{
		"--metrics-addr=127.0.0.1:9091",
		"--store-path=/store",
		transportHost,
		"--http-event-publishers=ptp-event-publisher-service-NODE_NAME.openshift-ptp.svc.cluster.local:9043",
		"--api-port=8089",
	}
	container.SecurityContext = nil

	return container, nil
}

// defineRBACProxyContainer creates the kube-rbac-proxy container. It uses the definition from
// https://github.com/redhat-cne/cloud-event-proxy/blob/release-4.18/examples/manifests/consumer.yaml.
func defineRBACProxyContainer(kubeRBACProxyImage string) (*corev1.Container, error) {
	container, err := pod.NewContainerBuilder(
		"kube-rbac-proxy", kubeRBACProxyImage, []string{"/usr/local/bin/kube-rbac-proxy"}).
		WithPorts([]corev1.ContainerPort{{
			Name:          "https",
			ContainerPort: 8443,
		}}).
		WithCustomResourcesRequests(corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("20Mi"),
		}).
		WithVolumeMount(corev1.VolumeMount{
			Name:      "sidecar-certs",
			MountPath: "/etc/metrics",
		}).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create rbac-proxy container: %w", err)
	}

	container.SecurityContext = nil
	container.Args = []string{
		"--logtostderr",
		"--secure-listen-address=:8443",
		"--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256," +
			"TLS_RSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
		"--upstream=http://127.0.0.1:9091/",
		"--tls-private-key-file=/etc/metrics/tls.key",
		"--tls-cert-file=/etc/metrics/tls.crt",
	}

	return container, nil
}

// eventImages is a struct to hold the images taken from the linuxptp-daemon daemonset.
type eventImages struct {
	cloudEventProxy string
	kubeRBACProxy   string
}

// getEventImagesFromPtpDaemon retrieves the images used by the cloud-event-proxy and kube-rbac-proxy containers in the
// linuxptp-daemon daemonset. It returns an error if the daemonset is not found or if any of the images is not found.
func getEventImagesFromPtpDaemon(client *clients.Settings) (*eventImages, error) {
	daemonsetBuilder, err := daemonset.Pull(client, ranparam.LinuxPtpDaemonsetName, ranparam.PtpOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull linuxptp-daemon daemonset: %w", err)
	}

	if daemonsetBuilder.Definition == nil {
		return nil, fmt.Errorf("linuxptp-daemon daemonset definition is nil")
	}

	var images eventImages

	for _, container := range daemonsetBuilder.Definition.Spec.Template.Spec.Containers {
		switch container.Name {
		case "cloud-event-proxy":
			images.cloudEventProxy = container.Image
		case "kube-rbac-proxy":
			images.kubeRBACProxy = container.Image
		}
	}

	if images.cloudEventProxy == "" {
		return nil, fmt.Errorf("cloud-event-proxy image not found in linuxptp-daemon")
	}

	if images.kubeRBACProxy == "" {
		return nil, fmt.Errorf("kube-rbac-proxy image not found in linuxptp-daemon")
	}

	return &images, nil
}
