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
	WaitTimeout      = 20 * time.Minute
	DefaultTimeout   = 300 * time.Second
	RetryInterval    = 30 * time.Second
	NamespaceTimeout = 30 * time.Second
	PodReadyTimeout  = 300 * time.Second
	CleanupTimeout   = 120 * time.Second
	// Specific operation timeouts
	NADTimeout               = 3 * time.Minute  // Timeout for NetworkAttachmentDefinition operations
	PodLabelReadyTimeout     = 60 * time.Second // Timeout for waiting for pod with label to be ready
	PingTimeout              = 2 * time.Minute  // Timeout for ping connectivity tests
	VFResourceTimeout        = 2 * time.Minute  // Timeout for VF resource availability check
	PolicyApplicationTimeout = 20 * time.Minute // Timeout for SR-IOV policy application (includes MCP update)
	PollingInterval          = 2 * time.Second  // Standard polling interval for wait operations
	MCPStableInterval        = 30 * time.Second // Polling interval for MachineConfigPool stability checks
	// Polling intervals for heavier operations (3x standard interval)
	VFResourcePollingInterval = PollingInterval * 3 // Longer interval for VF resource checks (heavier operation)
	PingPollingInterval       = PollingInterval * 3 // Longer interval for ping operations

	// Test pod network configuration
	// Default IP addresses for test pods (client and server)
	TestPodClientIP = "192.168.1.10/24"
	TestPodServerIP = "192.168.1.11/24"
	// Default MAC addresses for test pods
	TestPodClientMAC = "20:04:0f:f1:88:01"
	TestPodServerMAC = "20:04:0f:f1:88:02"
)

var (
	// Labels list for suite-level test selection
	// NOTE: Only LabelSuite is included here. Individual test files add their own specific labels.
	// For example, basic.go adds LabelBasic, so filtering by "basic" will only run those 9 tests.
	Labels = []string{LabelSuite}
)
