package params

import "time"

const (
	// Label represents neuron that can be used for test cases selection.
	Label = "neuron"
	// NeuronCapacityID - ID string for Neuron device capacity.
	NeuronCapacityID = "aws.amazon.com/neuron"
	// NeuronCoreCapacityID - ID string for NeuronCore capacity.
	NeuronCoreCapacityID = "aws.amazon.com/neuroncore"
	// NeuronLogLevel - Log Level for Neuron Tests.
	NeuronLogLevel = 90
	// NeuronNamespace - Namespace for the AWS Neuron Operator.
	NeuronNamespace = "aws-neuron-operator"
	// NeuronNFDLabelKey - The key of the label added by NFD.
	NeuronNFDLabelKey = "feature.node.kubernetes.io/aws-neuron"
	// NeuronNFDLabelValue - The value of the label added by NFD.
	NeuronNFDLabelValue = "true"
	// DeviceConfigName - The name of the DeviceConfig CR.
	DeviceConfigName = "neuron"
	// LabelSuite represents 'Neuron Basic' label that can be used for test cases selection.
	LabelSuite = "neuron-basic"
	// ClusterStabilityTimeout - The timeout for waiting for cluster stability.
	ClusterStabilityTimeout = 15 * time.Minute
	// DefaultTimeout - The default timeout in minutes.
	DefaultTimeout = 5 * time.Minute
	// DefaultSleepInterval - The default sleep time interval between checks.
	DefaultSleepInterval = 5 * time.Second

	// NFDNamespace represents NFD operator namespace (re-export for convenience).
	NFDNamespace = "openshift-nfd"

	// DefaultDeviceConfigName represents the default DeviceConfig CR name.
	DefaultDeviceConfigName = "neuron"

	// PCIVendorID represents the AWS Neuron PCI vendor ID.
	PCIVendorID = "1d0f"

	// MetricsDaemonSetPrefix represents the prefix for the metrics DaemonSet name.
	MetricsDaemonSetPrefix = "neuron-node-metrics"

	// InClusterBuildLabel represents the label for in-cluster build test cases.
	InClusterBuildLabel = "in-cluster-build"

	// BuildConfigMapPrefix represents the prefix for the Dockerfile ConfigMap created by the operator.
	BuildConfigMapPrefix = "dockerfile-"

	// DevicePluginDaemonSetPrefix represents the prefix for the device plugin DaemonSet name.
	DevicePluginDaemonSetPrefix = "neuron-device-plugin"

	// SchedulerDeploymentName represents the name of the custom scheduler deployment.
	SchedulerDeploymentName = "neuron-scheduler"

	// DRALabel represents the label for DRA test cases.
	DRALabel = "dra"

	// DRADaemonSetLabelKey is the KMM label key for DRA-role DaemonSets.
	DRADaemonSetLabelKey = "kmm.node.kubernetes.io/role"

	// DRADaemonSetLabelValue is the KMM label value for DRA-role DaemonSets.
	DRADaemonSetLabelValue = "dra"

	// DRADriverName is the default DRA driver name for Neuron.
	DRADriverName = "neuron.aws.com"

	// DRADefaultDeviceClassName is the default DeviceClass name for Neuron DRA.
	DRADefaultDeviceClassName = "neuron.aws.com"

	// DRAServiceAccountName is the service account for the DRA driver pods.
	DRAServiceAccountName = "awslabs-gpu-operator-dra-driver"

	// DRADaemonSetPrefix is the prefix for DRA DaemonSet names created by KMM.
	DRADaemonSetPrefix = "neuron-dra-"

	// SchedulerExtensionDeploymentPrefix is the prefix for the scheduler extension deployment.
	SchedulerExtensionDeploymentPrefix = "neuron-scheduler-extension"
)

// DeviceIDs contains all supported Neuron device IDs.
var DeviceIDs = []string{
	"7064", "7065", "7066", "7067",
	"7164",
	"7264",
	"7364",
}
