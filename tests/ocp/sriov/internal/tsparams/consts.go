package tsparams

import "time"

const (
	// Labels
	LabelSuite = "sriov"
	LabelBasic = "basic"
	
	// Test namespace
	TestNamespaceName = "sriov-basic-test"
	
	// Timeouts
	WaitTimeout = 20 * time.Minute
	DefaultTimeout = 300 * time.Second
	RetryInterval = 30 * time.Second
	NamespaceTimeout = 30 * time.Second
	PodReadyTimeout = 300 * time.Second
	CleanupTimeout = 120 * time.Second
)

var (
	// Labels list for test selection
	Labels = []string{LabelSuite, LabelBasic}
)

