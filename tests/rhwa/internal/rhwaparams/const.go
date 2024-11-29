package rhwaparams

import (
	"time"
)

const (
	// Label represents rhwa label that can be used for test cases selection.
	Label = "rhwa"
	// RhwaOperatorNs custom namespace of rhwa operators.
	RhwaOperatorNs = "openshift-workload-availability"
	// DefaultTimeout represents the default timeout.
	DefaultTimeout = 300 * time.Second
	// TestNamespaceName namespace where all dast test cases are performed
	TestNamespaceName = "dast-tests"

	LogLevel = 90

	TestContainerDast = "quay.io/frmoreno/eco-dast:latest"
)
