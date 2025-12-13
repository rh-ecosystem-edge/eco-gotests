// Package tsparams provides test suite parameters and constants for OCP SR-IOV tests.
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
	// LabelBasic represents basic test label for filtering.
	LabelBasic = "basic"

	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 35 * time.Minute
	// DefaultStableDuration represents the default stableDuration for most StableFor functions.
	DefaultStableDuration = 10 * time.Second
	// WaitTimeout represents default timeout for most waiting operations.
	WaitTimeout = 20 * time.Minute
	// DefaultTimeout represents default timeout for general operations.
	DefaultTimeout = 300 * time.Second
	// RetryInterval represents retry interval for the most ginkgo Eventually functions.
	RetryInterval = 3 * time.Second
	// PollingInterval represents polling interval for wait operations.
	PollingInterval = 2 * time.Second
	// NamespaceTimeout represents timeout for namespace operations.
	NamespaceTimeout = 30 * time.Second
	// PodReadyTimeout represents timeout for pod readiness checks.
	PodReadyTimeout = 300 * time.Second
	// CleanupTimeout represents timeout for cleanup operations.
	CleanupTimeout = 120 * time.Second
	// CarrierWaitTimeout represents timeout for waiting for carrier status.
	CarrierWaitTimeout = 30 * time.Second
	// MCPStableInterval represents polling interval for MCP stability checks.
	MCPStableInterval = 10 * time.Second
	// NADTimeout represents timeout for NAD creation.
	NADTimeout = 2 * time.Minute
	// PolicyApplicationTimeout represents timeout for SR-IOV policy application.
	PolicyApplicationTimeout = 35 * time.Minute
	// DebugPodCleanupTimeout represents timeout for debug pod cleanup.
	DebugPodCleanupTimeout = 60 * time.Second

	// TestPodClientMAC is the default MAC address for test client pods.
	TestPodClientMAC = "02:04:0f:f1:88:01"
	// TestPodServerMAC is the default MAC address for test server pods.
	TestPodServerMAC = "02:04:0f:f1:88:02"
	// TestPodClientIP is the default IP address for test client pods.
	TestPodClientIP = "192.168.0.1/24"
	// TestPodServerIP is the default IP address for test server pods.
	TestPodServerIP = "192.168.0.2/24"

	// DefaultTestMTU is the default MTU value for testing.
	DefaultTestMTU = 9000

	// VLAN and rate limiting test constants.

	// TestVLAN is the default VLAN ID for VLAN configuration tests.
	TestVLAN = 100
	// TestVlanQoS is the default VLAN QoS value for tests.
	TestVlanQoS = 2
	// TestMinTxRate is the default minimum TX rate (Mbps) for rate limiting tests.
	TestMinTxRate = 40
	// TestMaxTxRate is the default maximum TX rate (Mbps) for rate limiting tests.
	TestMaxTxRate = 100

	// BCM vendor constants.

	// BCMVendorID is the PCI vendor ID for Broadcom NICs.
	BCMVendorID = "14e4"
)
