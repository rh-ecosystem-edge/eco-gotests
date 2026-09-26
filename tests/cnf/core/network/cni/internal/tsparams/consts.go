package tsparams

import "time"

const (
	// LabelSuite represents cni label that can be used for test cases selection.
	LabelSuite = "cni"
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// PodStartTimeout represents the timeout for pod creation and startup.
	PodStartTimeout = 2 * time.Minute
	// IPPoolEmptyTimeout represents the timeout waiting for whereabouts IPPool allocations to drain.
	IPPoolEmptyTimeout = 30 * time.Second
	// IPPoolEmptyPollingInterval represents the poll interval for whereabouts IPPool drain checks.
	IPPoolEmptyPollingInterval = 3 * time.Second
	// LabelSriovVRFDualTestCases represents vrf-sriov-dual test suite label.
	LabelSriovVRFDualTestCases = "vrf-sriov-dual"
	// LabelSriovVRFDhcpTestCases represents vrf-sriov-dhcp test suite label.
	LabelSriovVRFDhcpTestCases = "vrf-sriov-dhcp"
	// LabelSriovVRFTestCases represents vrf-sriov test suite label.
	LabelSriovVRFTestCases = "vrf-sriov"
	// LabelNadVRFTestCases represents vrf-nad test suite label.
	LabelNadVRFTestCases = "vrf-nad"
	// LabelNadVRFDualTestCases represents vrf-nad-dual test suite label.
	LabelNadVRFDualTestCases = "vrf-nad-dual"
	// LabelNadVRFDhcpTestCases represents vrf-nad-dhcp test suite label.
	LabelNadVRFDhcpTestCases = "vrf-nad-dhcp"
	// LabelNadVRFWhereaboutsTestCases represents vrf-nad-whereabouts test suite label.
	LabelNadVRFWhereaboutsTestCases = "vrf-nad-whereabouts"
)
