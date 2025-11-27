package tsparams

import "time"

const (
	// TestNamespaceName sriov namespace where all test cases are performed.
	TestNamespaceName = "sriov-tests"
	// LabelSuite represents sriov label that can be used for test cases selection.
	LabelSuite = "ocpsriov"
	// LabelOcpSriovReinstallation represents an SR-IOV operator reinstallation label
	// that can be used for test cases selection.
	LabelOcpSriovReinstallation = "sriovreinstall"
	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 35 * time.Minute
	// DefaultStableDuration represents the default stableDuration for most StableFor functions.
	DefaultStableDuration = 10 * time.Second
	// RetryInterval represents retry interval for the most ginkgo Eventually functions.
	RetryInterval = 3 * time.Second
)
