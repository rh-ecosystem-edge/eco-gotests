package amdgpuparams

const (
	// AMDGPUTestNamespace represents test case namespace name.
	AMDGPUTestNamespace = "amd-gpu-test"

	// AMDGPUOperatorNamespace represents AMD GPU operator namespace.
	AMDGPUOperatorNamespace = "openshift-amd-gpu"

	// NFDNamespace represents NFD operator namespace (re-export for convenience).
	NFDNamespace = "openshift-nfd"

	// LogLevel represents the default log level for AMD GPU tests.
	LogLevel = 90

	// DefaultDeviceConfigName represents the default DeviceConfig CR name.
	DefaultDeviceConfigName = "amd-gpu-device-config"

	// DefaultMachineConfigName represents the default MachineConfig name for blacklisting.
	DefaultMachineConfigName = "amdgpu-module-blacklist"

	// DefaultDriverVersion represents the default AMD GPU driver version.
	DefaultDriverVersion = "6.4.3"

	// AMDPCIVendorID represents the AMD GPU PCI vendor ID.
	AMDPCIVendorID = "1002"

	// DefaultOperatorTimeout represents the default timeout for operator operations.
	DefaultOperatorTimeout = "15m"
	// KMMOperatorTimeout represents the timeout for KMM operator operations.
	KMMOperatorTimeout = "10m"
	// AMDGPUOperatorTimeout represents the timeout for AMD GPU operator operations.
	AMDGPUOperatorTimeout = "10m"
)

var (
	// AMDRXDeviceIDs contains common AMD GPU PCI device IDs for RX series.
	AMDRXDeviceIDs = []string{
		"15d8", // RX 580
		"67df", // RX 480/580
		"6fdf", // RX 5500 XT
		"731f", // RX 6600/6600 XT
		"73ef", // RX 6650 XT
		"73ff", // RX 6600M
		"7340", // RX 7900 XTX
		"744c", // RX 7900 XT
	}
)
