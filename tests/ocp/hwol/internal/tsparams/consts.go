// Package tsparams provides test suite parameters and constants for OCP HWOL tests.
package tsparams

import "time"

const (
	// TestNamespaceName is the namespace where HWOL test cases are performed.
	TestNamespaceName = "hwol-tests"
	// LabelSuite represents the HWOL suite label for test selection.
	LabelSuite = "ocphwol"
	// LabelSetup represents the setup/smoke test label for filtering.
	LabelSetup = "setup"
	// LabelSwitchdev represents switchdev / OVS bridge coverage.
	LabelSwitchdev = "switchdev"
	// LabelOvsNetwork represents OVS CNI attach coverage.
	LabelOvsNetwork = "ovs-network"
	// LabelSriovNetwork represents sriov CNI attach coverage.
	LabelSriovNetwork = "sriov-network"
	// LabelOffload represents iperf traffic + OVS type=offloaded flow coverage.
	LabelOffload = "offload"

	// ResourceNamePrefix is the OpenShift device-plugin resource name prefix.
	ResourceNamePrefix = "openshift.io/"

	// IperfDuration is the client-side iperf3 test duration for offload checks.
	IperfDuration = 15 * time.Second
	// MinVFNumForOffload is VF0 reserved plus two workload VFs for server/client pods.
	MinVFNumForOffload = 3

	// DefaultTimeout represents default timeout for general operations.
	DefaultTimeout = 300 * time.Second
	// WaitTimeout represents default timeout for most waiting operations.
	WaitTimeout = 20 * time.Minute
	// MCOWaitTimeout represents MCP / SR-IOV drain and apply wait budget.
	MCOWaitTimeout = 45 * time.Minute
	// CleanupWaitTimeout is the AfterAll budget for leaving switchdev.
	// Leaving switchdev can hang on busy eswitch; fail fast instead of burning MCOWaitTimeout.
	CleanupWaitTimeout = 15 * time.Minute
	// DefaultStableDuration is how long MCP must remain stable after update.
	DefaultStableDuration = 1 * time.Minute
	// RetryInterval represents retry interval for ginkgo Eventually functions.
	RetryInterval = 3 * time.Second

	// MlxVendorID is the Mellanox/NVIDIA PCI vendor ID.
	MlxVendorID = "15b3"

	// SwitchdevMode is the eSwitchMode value for hardware offload.
	SwitchdevMode = "switchdev"
	// ManageSoftwareBridgesGate is the SriovOperatorConfig feature gate for OVS bridge management.
	ManageSoftwareBridgesGate = "manageSoftwareBridges"

	// PoolConfigName is the SriovNetworkPoolConfig created by switchdev tests.
	PoolConfigName = "hwol-ovs-offload"
	// OvsOffloadMachineConfigPrefix begins the MachineConfig name rendered by the SR-IOV Operator.
	OvsOffloadMachineConfigPrefix = "00-"
	// OvsOffloadMachineConfigSuffix ends the MachineConfig name rendered by the SR-IOV Operator.
	OvsOffloadMachineConfigSuffix = "-ovs-hw-offload"
	// PolicyName is the SriovNetworkNodePolicy created by switchdev tests.
	PolicyName = "hwol-switchdev"
	// ResourceName is the SR-IOV resource name advertised for HWOL VFs.
	ResourceName = "hwolresource"
	// SriovNetworkName is the SriovNetwork created alongside the switchdev policy.
	SriovNetworkName = "hwol-sriovnet"
	// OvsNetworkName is the OVSNetwork created by ovs-network tests.
	OvsNetworkName = "hwol-ovsnet"

	// TestResourceLabelKey is the label key used to identify test-created resources.
	TestResourceLabelKey = "eco-gotests.openshift.io/test"
	// TestResourceLabelValue is the label value for test-created resources.
	TestResourceLabelValue = "ocp-hwol"
)
