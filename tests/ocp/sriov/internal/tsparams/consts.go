package tsparams

import "time"

const (
	// TestNamespaceName is the sriov namespace where all test cases are performed.
	TestNamespaceName = "sriov-tests"
	// LabelSuite represents sriov label that can be used for test cases selection.
	LabelSuite = "ocpsriov"
	// LabelBasic represents basic test label for filtering.
	LabelBasic = "basic"

	// WaitTimeout is the maximum wait time for long-running operations.
	WaitTimeout = 20 * time.Minute
	// DefaultTimeout is the default timeout for general operations.
	DefaultTimeout = 300 * time.Second
	// RetryInterval is the interval between retries for operations.
	RetryInterval = 30 * time.Second
	// NamespaceTimeout is the timeout for namespace operations.
	NamespaceTimeout = 30 * time.Second
	// PodReadyTimeout is the timeout for waiting for pods to be ready.
	PodReadyTimeout = 300 * time.Second
	// CleanupTimeout is the timeout for cleanup operations.
	CleanupTimeout = 120 * time.Second

	// NADTimeout is the timeout for NetworkAttachmentDefinition operations.
	NADTimeout = 3 * time.Minute
	// PodLabelReadyTimeout is the timeout for waiting for pod with label to be ready.
	PodLabelReadyTimeout = 60 * time.Second
	// PingTimeout is the timeout for ping connectivity tests.
	PingTimeout = 2 * time.Minute
	// VFResourceTimeout is the timeout for VF resource availability check.
	VFResourceTimeout = 2 * time.Minute
	// PolicyApplicationTimeout is the timeout for SR-IOV policy application (includes MCP update).
	PolicyApplicationTimeout = 20 * time.Minute
	// InterfaceVerifyTimeout is the timeout for interface verification retries.
	InterfaceVerifyTimeout = 30 * time.Second
	// PollingInterval is the standard polling interval for wait operations.
	PollingInterval = 2 * time.Second
	// MCPStableInterval is the polling interval for MachineConfigPool stability checks.
	MCPStableInterval = 30 * time.Second

	// VFResourcePollingInterval is the longer interval for VF resource checks (heavier operation).
	VFResourcePollingInterval = PollingInterval * 3
	// PingPollingInterval is the longer interval for ping operations.
	PingPollingInterval = PollingInterval * 3

	// TestPodClientIP is the default IP address for test client pods.
	TestPodClientIP = "192.168.1.10/24"
	// TestPodServerIP is the default IP address for test server pods.
	TestPodServerIP = "192.168.1.11/24"

	// TestPodClientMAC is the default MAC address for test client pods.
	TestPodClientMAC = "20:04:0f:f1:88:01"
	// TestPodServerMAC is the default MAC address for test server pods.
	TestPodServerMAC = "20:04:0f:f1:88:02"
)

var (
	// Labels list for suite-level test selection
	// NOTE: Only LabelSuite is included here. Individual test files add their own specific labels.
	// For example, basic.go adds LabelBasic, so filtering by "basic" will only run those 9 tests.
	Labels = []string{LabelSuite}
)
