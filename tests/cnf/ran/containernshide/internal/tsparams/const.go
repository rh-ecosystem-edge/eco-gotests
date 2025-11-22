package tsparams

import "k8s.io/klog/v2"

const (
	// LabelSuite is the label for all the tests in this suite.
	LabelSuite string = "containernshide"
	// LabelContainerNSHideTestCases is the label for a particular test case.
	LabelContainerNSHideTestCases string = "containernshide"
	// LogLevel is the verbosity of log messages in the test suite.
	LogLevel klog.Level = 90
)
