package tsparams

import "k8s.io/klog/v2"

const (
	// LabelSuite is the label for all tests in the PTP suite.
	LabelSuite = "ptp"
	// LabelEventConsumer is the label for all tests in the PTP event consumer suite.
	LabelEventConsumer = "event-consumer"
	// LabelEventsAndMetrics is the label for all tests in the PTP events and metrics suite.
	LabelEventsAndMetrics = "events-and-metrics"
	// LabelNodeReboot is the label for all tests in the PTP node reboot suite.
	LabelNodeReboot = "node-reboot"
	// LabelInterfaces is the label for all tests in the PTP interfaces suite.
	LabelInterfaces = "interfaces"
	// LabelProcessRestart is the label for all tests in the PTP process restart suite.
	LabelProcessRestart = "process-restart"
	// LabelOC2Port is the label for all tests in the PTP OC 2 port suite.
	LabelOC2Port = "oc-two-port"
	// LabelLeapFile is the label for all tests in the PTP leap file suite.
	LabelLeapFile = "leap-file"

	// LeapConfigmapName is the name of the leap configmap.
	LeapConfigmapName = "leap-configmap"

	// CloudEventsNamespace is the namespace used for the cloud events consumer and associated resources.
	CloudEventsNamespace = "cloud-events"
)

// These parameters are for the PTP event consumer deployment.
const (
	// SidecarConsumerSA is the name of the sidecar consumer service account.
	SidecarConsumerSA = "sidecar-consumer-sa"
	// SidecarConsumerRole is the name of the sidecar consumer role.
	SidecarConsumerRole = "sidecar-consumer-role"
	// SidecarConsumerRoleBinding is the name of the sidecar consumer role binding.
	SidecarConsumerRoleBinding = "sidecar-consumer-role-binding"
	// PrometheusK8s is the name of the prometheus k8s service account and associated Role[Binding]s.
	PrometheusK8s = "prometheus-k8s"
)

// LogLevel is the glog level used for all helpers in the PTP suite. It is set so that eco-goinfra is 100, cnf/ran is
// 90, and the suite itself is 80.
const LogLevel klog.Level = 80
