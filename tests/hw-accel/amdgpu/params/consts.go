package params

import "time"

const (
	// Label represents gpu that can be used for test cases selection.
	Label = "amd-gpu"
	// AMDGPUCapacityID - ID string for AMD GPU capacity.
	AMDGPUCapacityID = "amd.com/gpu"
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
	// ClusterStabilityTimeout - The timeout for waiting for cluster stability.
	// In SNO environments, MachineConfig changes can trigger reboots taking 30+ minutes.
	ClusterStabilityTimeout = 60 * time.Minute
	// DefaultTimeout - The default timeout in minutes.
	DefaultTimeout = 30 * time.Minute
	// DefaultSleepInterval - The default sleep time interval between checks.
	DefaultSleepInterval = 10 * time.Second
	// MaxNodeLabellerPodsPerNode - Maximum Node Labeller Pods on each AMD GPU worker node.
	MaxNodeLabellerPodsPerNode = 1

	// NFDNamespace represents NFD operator namespace (re-export for convenience).
	NFDNamespace = "openshift-nfd"

	// DefaultDeviceConfigName represents the default DeviceConfig CR name.
	DefaultDeviceConfigName = "amd-gpu-device-config"

	// DefaultMachineConfigName represents the default MachineConfig name for blacklisting.
	DefaultMachineConfigName = "amdgpu-module-blacklist"

	// SNOPodRunningTimeout - Extended timeout for SNO environments where node may reboot.
	// SNO reboot + driver loading can take 30+ minutes.
	SNOPodRunningTimeout = 60 * time.Minute

	// SNOClusterStabilityTimeout - Extended timeout for SNO cluster stability after reboot.
	// MachineConfig changes in SNO can take 30+ minutes for full reboot cycle.
	SNOClusterStabilityTimeout = 60 * time.Minute

	// ConnectionRetryInterval - Interval between retries when connection is lost.
	ConnectionRetryInterval = 30 * time.Second

	// MaxConnectionRetries - Maximum number of retries on connection failure.
	// With 30s interval, this allows for 30 minutes of retries.
	MaxConnectionRetries = 60
)
