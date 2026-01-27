package tsparams

import "time"

const (
	// LabelSuite represents upgrade test suite label.
	LabelSuite = "upgrade"

	// UpgradeTestNamespace represents the namespace for upgrade tests.
	UpgradeTestNamespace = "neuron-upgrade-test"

	// TestWorkloadPodName represents the name of the test workload pod.
	TestWorkloadPodName = "neuron-test-workload"
	// TestWorkloadContainerName represents the container name.
	TestWorkloadContainerName = "test-container"

	// OperatorDeployTimeout represents the timeout for operator deployment.
	OperatorDeployTimeout = 10 * time.Minute
	// DevicePluginReadyTimeout represents the timeout for device plugin readiness.
	DevicePluginReadyTimeout = 10 * time.Minute
	// TotalUpgradeTimeout represents the total timeout for the upgrade process.
	TotalUpgradeTimeout = 45 * time.Minute
)
