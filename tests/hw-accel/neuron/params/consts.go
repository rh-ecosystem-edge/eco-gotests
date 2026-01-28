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
	NeuronNamespace = "ai-operator-on-aws"
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

	// DevicePluginDaemonSetPrefix represents the prefix for the device plugin DaemonSet name.
	DevicePluginDaemonSetPrefix = "neuron-device-plugin"

	// SchedulerDeploymentName represents the name of the custom scheduler deployment.
	SchedulerDeploymentName = "neuron-scheduler"
)

// DeviceIDs contains all supported Neuron device IDs.
var DeviceIDs = []string{
	"7064", "7065", "7066", "7067",
	"7164",
	"7264",
	"7364",
}
