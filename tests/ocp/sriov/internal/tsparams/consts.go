package tsparams

import "time"

const (
	// TestNamespaceName sriov namespace where all test cases are performed.
	TestNamespaceName = "sriov-tests"
	// LabelSuite represents sriov label that can be used for test cases selection.
	LabelSuite = "ocpsriov"
	// LabelBasic represents basic test label for filtering
	LabelBasic = "basic"
	
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
