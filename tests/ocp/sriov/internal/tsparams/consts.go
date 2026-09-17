// Package tsparams provides test suite parameters and constants for OCP SR-IOV tests.
package tsparams

import "time"

const (
	// TestNamespaceName sriov namespace where all test cases are performed.
	TestNamespaceName = "sriov-tests"
	// TestNamespaceName1 sriov namespace where all test cases are performed.
	TestNamespaceName1 = "sriov-tests-1"
	// TestNamespaceName2 sriov namespace where all test cases are performed.
	TestNamespaceName2 = "sriov-tests-2"
	// LabelSuite represents sriov label that can be used for test cases selection.
	LabelSuite = "ocpsriov"
	// LabelOcpSriovReinstallation represents an SR-IOV operator reinstallation label
	// that can be used for test cases selection.
	LabelOcpSriovReinstallation = "sriovreinstall"
	// LabelBasic represents basic test label for filtering.
	LabelBasic = "basic"
	// LabelExposeMTUTestCases represents Expose MTU label that can be used for test cases selection.
	LabelExposeMTUTestCases = "exposemtu"
	// LabelSriovMetricsTestCases represents Sriov Metrics Exporter label that can be used for test cases selection.
	LabelSriovMetricsTestCases = "sriovmetrics"
	// LabelSriovHWEnabled represents sriov HW Enabled tests that can be used for test cases selection.
	LabelSriovHWEnabled = "sriov-hw-enabled"
	// LabelSriovNetAppNsTestCases represents sriov network application namespace label that can be used
	// for test cases selection.
	LabelSriovNetAppNsTestCases = "sriovnet-app-ns"
	// LabelRdmaMetricsAPITestCases represents Rdma Metrics label that can be used for test cases selection.
	LabelRdmaMetricsAPITestCases = "rdmametricsapi"
	// LabelParallelDrainingTestCases represents parallel draining label that can be used for test cases selection.
	LabelParallelDrainingTestCases = "paralleldraining"
	// LabelQinQTestCases represents QinQ label that can be used for test cases selection.
	LabelQinQTestCases = "qinq"
	// LabelExternallyManagedTestCases represents ExternallyManaged label that can be used for test cases selection.
	LabelExternallyManagedTestCases = "externallymanaged"
	// LabelSriovIPv4 represents SR-IOV IPv4 connectivity tests that can be used for test cases selection.
	LabelSriovIPv4 = "sriov-ipv4"
	// SCTPMachineConfigName is the MachineConfig that loads the SCTP kernel module on workers.
	SCTPMachineConfigName = "load-sctp-module"
	// SriovResourceNameExManagedTrue is the SR-IOV policy and network name for ExternallyManaged tests.
	SriovResourceNameExManagedTrue = "extmanaged"

	// Cluster IP family values returned by sriovocpenv.GetClusterIPFamily.

	// IPV4Family represents IP version 4 protocol.
	IPV4Family = "IPv4"
	// IPV6Family represents IP version 6 protocol.
	IPV6Family = "IPv6"
	// DualIPFamily represents dual-stack (IPv4 and IPv6) IP protocol.
	DualIPFamily = "dual"

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
	NADTimeout = 1 * time.Minute
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

	// Net1Interface is the name of the first secondary network interface attached to pods.
	Net1Interface = "net1"

	// ClientPodMTU500 is the name of the client pod for MTU 500 tests.
	ClientPodMTU500 = "client-mtu500"
	// ServerPodMTU500 is the name of the server pod for MTU 500 tests.
	ServerPodMTU500 = "server-mtu500"
	// ClientPodMTU9000 is the name of the client pod for MTU 9000 tests.
	ClientPodMTU9000 = "client-mtu9000"
	// ServerPodMTU9000 is the name of the server pod for MTU 9000 tests.
	ServerPodMTU9000 = "server-mtu9000"
	// ClientPodWhereabouts is the name of the client pod for whereabouts IPAM tests.
	ClientPodWhereabouts = "client-whereabouts"
	// ServerPodWhereabouts is the name of the server pod for whereabouts IPAM tests.
	ServerPodWhereabouts = "server-whereabouts"
	// ClientPodVlanMTU500 is the name of the client pod for VLAN MTU 500 tests.
	ClientPodVlanMTU500 = "client-vlan-mtu500"
	// ServerPodVlanMTU500 is the name of the server pod for VLAN MTU 500 tests.
	ServerPodVlanMTU500 = "server-vlan-mtu500"
	// ClientPodVlanMTU9000 is the name of the client pod for VLAN MTU 9000 tests.
	ClientPodVlanMTU9000 = "client-vlan-mtu9000"
	// ServerPodVlanMTU9000 is the name of the server pod for VLAN MTU 9000 tests.
	ServerPodVlanMTU9000 = "server-vlan-mtu9000"

	// MulticastIPv4Group is the default IPv4 multicast group address.
	MulticastIPv4Group = "239.100.0.250"
	// MulticastIPv4MAC is the Ethernet multicast MAC for 239.100.0.250.
	MulticastIPv4MAC = "01:00:5e:64:00:fa"
	// MulticastIPv4GroupLargeMTU is the IPv4 multicast group for MTU > 1500.
	MulticastIPv4GroupLargeMTU = "239.100.100.250"
	// MulticastIPv4MACLargeMTU is the Ethernet multicast MAC for 239.100.100.250.
	MulticastIPv4MACLargeMTU = "01:00:5e:64:64:fa"
	// MulticastIPv6Group is the IPv6 site-local multicast group address.
	MulticastIPv6Group = "ff05:5::5"
	// MulticastIPv6MAC is the Ethernet multicast MAC for ff05:5::5.
	MulticastIPv6MAC = "33:33:00:00:00:05"

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
	// MlxVendorID is the Mellanox SR-IOV Vendor ID.
	MlxVendorID = "15b3"

	// TestResourceLabelKey is the label key used to identify test-created resources.
	TestResourceLabelKey = "eco-gotests.openshift.io/test"
	// TestResourceLabelValue is the label value for test-created resources.
	TestResourceLabelValue = "ocp-sriov"
)
