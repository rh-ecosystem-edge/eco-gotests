package tsparams

import "time"

const (
	// LabelSuite represents cni label that can be used for test cases selection.
	LabelSuite = "cni"
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// LabelVrfTestCases represents vrf test cases label.
	LabelVrfTestCases = "vrf"
)
