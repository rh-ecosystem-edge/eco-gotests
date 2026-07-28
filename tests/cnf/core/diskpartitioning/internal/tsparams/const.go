package tsparams

import "k8s.io/klog/v2"

const (
	// LabelSuite is the label for all tests in this suite.
	LabelSuite string = "diskpartitioning"
	// LabelDiskPartitioningTestCases is the label for individual disk partitioning test cases.
	LabelDiskPartitioningTestCases string = "diskpartitioning"

	// LogLevel is the verbosity of log messages in the test suite.
	LogLevel klog.Level = 90

	// ContainersMountPoint is the expected mount point for the containers partition.
	ContainersMountPoint = "/var/lib/containers"
	// ContainersPartLabel is the partition label used for the containers partition.
	ContainersPartLabel = "containers"
	// ContainersFSType is the expected filesystem type on the containers partition.
	ContainersFSType = "xfs"
	// ContainersMountUnit is the systemd mount unit name for the containers partition.
	ContainersMountUnit = "var-lib-containers.mount"

	// WorkerNodeLabel is the label selector used to identify worker nodes.
	WorkerNodeLabel = "node-role.kubernetes.io/worker"
)
