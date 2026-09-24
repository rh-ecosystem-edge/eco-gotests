package tsparams

import "time"

const (
	// LabelSuite represents cni label that can be used for test cases selection.
	LabelSuite = "cni"
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// PodStartTimeout represents the timeout for pod creation and startup.
	PodStartTimeout = 2 * time.Minute
	// LabelSriovVRFDualTestCases represents vrf-sriov-dual test suite label.
	LabelSriovVRFDualTestCases = "vrf-sriov-dual"
	// LabelSriovVRFDhcpTestCases represents vrf-sriov-dhcp test suite label.
	LabelSriovVRFDhcpTestCases = "vrf-sriov-dhcp"
	// LabelSriovVRFTestCases represents vrf-sriov test suite label.
	LabelSriovVRFTestCases = "vrf-sriov"
)
