package tsparams

import "time"

const (
	// LabelSuite represents metrics test suite label.
	LabelSuite = "metrics"

	// MetricsTestNamespace represents the namespace for metrics tests.
	MetricsTestNamespace = "neuron-metrics-test"

	// ServiceMonitorReadyTimeout represents the timeout for ServiceMonitor to be ready.
	ServiceMonitorReadyTimeout = 5 * time.Minute
	// OperatorDeployTimeout represents the timeout for operator deployment.
	OperatorDeployTimeout = 10 * time.Minute
	// DevicePluginReadyTimeout represents the timeout for device plugin readiness.
	DevicePluginReadyTimeout = 10 * time.Minute
	// MetricScrapeTimeout is how long to poll for a metric to appear in Prometheus after ServiceMonitor creation.
	MetricScrapeTimeout = 5 * time.Minute
	// MetricScrapeInterval is how often to retry when polling for metric availability.
	MetricScrapeInterval = 30 * time.Second

	// MetricsWorkloadPodName represents the name of the helper workload pod.
	MetricsWorkloadPodName = "neuron-metrics-workload"
	// MetricsWorkloadContainerName represents the container name for the helper workload.
	MetricsWorkloadContainerName = "metrics-helper"
	// WorkloadStartupTimeout is how long to wait for the helper workload pod to become Running.
	WorkloadStartupTimeout = 5 * time.Minute
)
