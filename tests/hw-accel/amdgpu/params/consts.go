package params

const (
	// Label represents gpu that can be used for test cases selection.
	Label = "amd-gpu"
	// AMDGPULogLevel - Log Level for AMD GPU Tests.
	AMDGPULogLevel = 90
	// AMDGPUNamespace - Namespace for the AMD GPU Operator.
	AMDGPUNamespace = "openshift-amd-gpu"
	// AMDNFDLabelKey - The key of the label added by NFD.
	AMDNFDLabelKey = "feature.node.kubernetes.io/amd-gpu"
	// AMDNFDLabelValue - The value of the label added by NFD.
	AMDNFDLabelValue = "true"
	// DeviceConfigName - The name of the DeviceConfig CR.
	DeviceConfigName = "amd-gpu-device-config"
	// LabelSuite represents 'AMD GPU Basic' label that can be used for test cases selection.
	LabelSuite = "amd-gpu-basic"

	// DefaultTimeout - The default timeout in seconds.
	DefaultTimeout = 20
	// DefaultSleepInterval - The default sleep time interval between checks.
	DefaultSleepInterval = 1
	// MaxNodeLabellerPodsPerNode - Maximum Node Labeller Pods on each AMD GPU worker node.
	MaxNodeLabellerPodsPerNode = 1

	// NFDNamespace represents NFD operator namespace (re-export for convenience).
	NFDNamespace = "openshift-nfd"

	// DefaultDeviceConfigName represents the default DeviceConfig CR name.
	DefaultDeviceConfigName = "amd-gpu-device-config"

	// DefaultMachineConfigName represents the default MachineConfig name for blacklisting.
	DefaultMachineConfigName = "amdgpu-module-blacklist"

	// DefaultDriverVersion represents the default AMD GPU driver version.
	DefaultDriverVersion = "6.4.3"
)
