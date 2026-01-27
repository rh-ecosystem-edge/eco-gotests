package tsparams

var (
	// TestWorkloadLabels represents the labels for test workload pods.
	TestWorkloadLabels = map[string]string{
		"app":        "neuron-test-workload",
		"test-suite": "neuron-upgrade",
	}
)
