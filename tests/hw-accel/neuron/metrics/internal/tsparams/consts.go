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
)
