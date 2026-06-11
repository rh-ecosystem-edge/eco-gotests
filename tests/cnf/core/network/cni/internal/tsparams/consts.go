package tsparams

import "time"

const (
	// LabelSuite represents cni label that can be used for test cases selection.
	LabelSuite = "cni"
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// RetryInterval represents the default polling interval for Eventually functions.
	RetryInterval = 5 * time.Second
	// PodWaitingTime represents the default timeout for pod lifecycle operations.
	PodWaitingTime = 2 * time.Minute
	// NADWaitTimeout represents the timeout for waiting until a NAD is available.
	NADWaitTimeout = 60 * time.Second
)
