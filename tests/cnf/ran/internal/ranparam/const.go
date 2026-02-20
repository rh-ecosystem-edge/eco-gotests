package ranparam

import (
	"time"

	"k8s.io/klog/v2"
)

// UnreachableIPv4Address is the IPv4 address that is unreachable. It is in the 192.0.2.0/24 subnet, which is reserved
// for documentation and example code.
const UnreachableIPv4Address = "192.0.2.1"

const (
	// Label represents the label for the ran test cases.
	Label = "ran"
	// LabelNoContainer is the label for RAN test cases that should not be executed in a container.
	LabelNoContainer = "no-container"

	// AcmOperatorNamespace ACM's namespace.
	AcmOperatorNamespace = "rhacm"

	// MceOperatorNamespace is the namespace for the MCE operator.
	MceOperatorNamespace = "multicluster-engine"

	// TalmOperatorHubNamespace TALM namespace.
	TalmOperatorHubNamespace = "topology-aware-lifecycle-manager"
	// TalmContainerName is the name of the container in the talm pod.
	TalmContainerName = "manager"

	// OpenshiftOperatorNamespace is the namespace where operators are.
	OpenshiftOperatorNamespace = "openshift-operators"
	// OpenshiftGitOpsNamespace is the namespace for the GitOps operator.
	OpenshiftGitOpsNamespace = "openshift-gitops"
	// OpenshiftGitopsRepoServer ocp git repo server.
	OpenshiftGitopsRepoServer = "openshift-gitops-repo-server"

	// OCloudOperatorNamespace is the namespace for the O-Cloud operator.
	OCloudOperatorNamespace = "oran-o2ims"

	// PtpContainerName is the name of the container in the PTP daemon pod.
	PtpContainerName = "linuxptp-daemon-container"
	// PtpDaemonsetLabelSelector is the label selector to find the PTP daemon pod.
	PtpDaemonsetLabelSelector = "app=linuxptp-daemon"
	// PtpOperatorNamespace is the namespace for the PTP operator.
	PtpOperatorNamespace = "openshift-ptp"
	// LinuxPtpDaemonsetName is the name of the Linux PTP daemon daemonset.
	LinuxPtpDaemonsetName = "linuxptp-daemon"
	// PtpServiceMonitorName is the name of the PTP operator's ServiceMonitor.
	PtpServiceMonitorName = "monitor-ptp"

	// CloudEventProxyContainerName is the sidecar in the linuxptp-daemon pod that emits events.
	CloudEventProxyContainerName = "cloud-event-proxy"
	// LogLevel is the verbosity for ran/internal packages.
	LogLevel klog.Level = 80

	// RetryInterval retry interval for node exec commands.
	RetryInterval = 10 * time.Second
	// RetryCount retry count for node exec commands.
	RetryCount = 3
)

// Querier package constants.
const (
	// ThanosQuerierRouteName is the name of the Thanos querier route.
	ThanosQuerierRouteName = "thanos-querier"
	// OpenshiftMonitoringNamespace is the namespace for the OpenShift Monitoring.
	OpenshiftMonitoringNamespace = "openshift-monitoring"
	// OpenshiftMonitoringViewRole is the role for the OpenShift Monitoring.
	OpenshiftMonitoringViewRole = "cluster-monitoring-view"
	// QuerierServiceAccountName is the name of the querier service account that gets created by the querier
	// package.
	QuerierServiceAccountName = "ran-querier"
	// QuerierCRBName is the name of the querier cluster role binding that gets created by the querier package to
	// bind the querier service account to the cluster monitoring view role.
	QuerierCRBName = "ran-querier-crb"
)

// Params for the alerter package. These are used for getting a token for the ACM Observability Alertmanager instance.
const (
	// ACMObservabilityNamespace is the namespace for the ACM Observability component.
	ACMObservabilityNamespace = "open-cluster-management-observability"
	// ACMObservabilityAMRouteName is the name of the route for the ACM Observability Alertmanager instance.
	ACMObservabilityAMRouteName = "alertmanager"
	// ACMObservabilityAMSecretName is the name of the secret for the ACM Observability Alertmanager instance which
	// contains the token for accessing the Alertmanager API.
	ACMObservabilityAMSecretName = "observability-alertmanager-accessor"
)

// Params for the default openshift ingress router CA secret. Used by rancluster for getting the CA pool for the default
// ingress router.
const (
	// IngressDefaultRouterCASecret is the name of the secret in the [OpenshiftIngressNamespace] namespace for the
	// default openshift ingress router CA.
	IngressDefaultRouterCASecret = "router-certs-default"
	// IngressDefaultRouterCAKey is the key in the [IngressDefaultRouterCASecret] secret for the default openshift
	// ingress router CA.
	IngressDefaultRouterCAKey = "tls.crt"
	// OpenshiftIngressNamespace is the namespace for the openshift ingress router.
	OpenshiftIngressNamespace string = "openshift-ingress"
)

// HubOperatorName represets the possible operator names that may have associated versions on the hub cluster.
type HubOperatorName string

const (
	// ACM is the name of the advanced cluster management operator.
	ACM HubOperatorName = "advanced-cluster-management"
	// TALM is the name of the topology aware lifecycle manager operator.
	TALM HubOperatorName = "topology-aware-lifecycle-manager"
	// GitOps is the name of the GitOps operator.
	GitOps HubOperatorName = "openshift-gitops-operator"
	// MCE is the name of the multicluster engine operator.
	MCE HubOperatorName = "multicluster-engine"
)

// SpokeOperatorName represents the possible operator names that may have associated versions on a spoke cluster.
type SpokeOperatorName string

const (
	// PTP is the name of the PTP operator.
	PTP SpokeOperatorName = "ptp-operator"
)

// ClusterType represents spoke cluster type.
type ClusterType string

const (
	// SNOCluster represents spoke cluster type as single-node openshift (SNO) cluster.
	SNOCluster ClusterType = "SNO"
	// HighlyAvailableCluster represents spoke cluster type as multi-node openshift (MNO) cluster.
	HighlyAvailableCluster ClusterType = "HighlyAvailable"
)
