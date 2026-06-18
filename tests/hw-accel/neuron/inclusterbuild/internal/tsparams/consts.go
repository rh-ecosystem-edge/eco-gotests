package tsparams

import "time"

const (
	// LabelSuite represents in-cluster build test suite label.
	LabelSuite = "inclusterbuild"

	// InClusterBuildTestNamespace represents the namespace for in-cluster build tests.
	InClusterBuildTestNamespace = "neuron-inclusterbuild-test"

	// BuildConfigMapTimeout represents the timeout for the Dockerfile ConfigMap to be created.
	BuildConfigMapTimeout = 5 * time.Minute
	// DevicePluginReadyTimeout represents the timeout for device plugin readiness.
	DevicePluginReadyTimeout = 15 * time.Minute
	// OperatorDeployTimeout represents the timeout for operator deployment.
	OperatorDeployTimeout = 10 * time.Minute
)
