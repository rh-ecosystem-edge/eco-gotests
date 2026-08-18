package tsparams

import "time"

const (
	// LabelSuite represents the DRA test suite label.
	LabelSuite = "neuron-dra"

	// DRADeployTimeout is the timeout for DRA DaemonSet to become ready.
	DRADeployTimeout = 10 * time.Minute

	// DeviceClassTimeout is the timeout for DeviceClass operations.
	DeviceClassTimeout = 5 * time.Minute

	// DRATestNamespace is the namespace for DRA consumer test pods.
	DRATestNamespace = "neuron-dra-test"
)
