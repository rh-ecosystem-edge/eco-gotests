package neuronconfig

import (
	"os"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	"k8s.io/klog/v2"
)

// NeuronConfig holds the configuration for Neuron tests.
type NeuronConfig struct {
	// VLLMImage is the vLLM container image with Neuron support.
	VLLMImage string
	// ModelName is the model to load for inference tests.
	ModelName string
	// HuggingFaceToken is the token for downloading models from Hugging Face.
	HuggingFaceToken string
	// DriversImage is the Neuron kernel module driver image.
	DriversImage string
	// DriverVersion is the Neuron driver version (required for DeviceConfig creation).
	DriverVersion string
	// DevicePluginImage is the Neuron device plugin image.
	DevicePluginImage string
	// SchedulerImage is the custom kube-scheduler image for Neuron.
	SchedulerImage string
	// SchedulerExtensionImage is the Neuron scheduler extension image.
	SchedulerExtensionImage string
	// NodeMetricsImage is the Neuron node metrics exporter image.
	NodeMetricsImage string
	// CatalogSource is the catalog source for the operator.
	CatalogSource string
	// CatalogSourceNamespace is the namespace for the catalog source.
	CatalogSourceNamespace string
	// SubscriptionName is the name of the operator subscription.
	SubscriptionName string
	// UpgradeTargetVersion is the target driver version for upgrade tests.
	UpgradeTargetVersion string
	// UpgradeTargetDriversImage is the target drivers image for upgrade tests.
	UpgradeTargetDriversImage string
	// ImageRepoSecretName is the name of the secret for pulling images.
	ImageRepoSecretName string
	// InstanceType is the AWS instance type for scaling tests (e.g., inf2.xlarge).
	InstanceType string
	// StorageClassName is the storage class for model PVC (default: gp3-csi).
	StorageClassName string
	// KServeModelName is the HuggingFace model for KServe inference tests.
	KServeModelName string
	// KServeVLLMImage is the vLLM Neuron image for the KServe ServingRuntime.
	KServeVLLMImage string
	// KServeNamespace is the namespace where KServe resources are deployed.
	KServeNamespace string
	// KServeTensorParallelSize is the tensor parallel size for KServe vLLM.
	KServeTensorParallelSize string
}

// NewNeuronConfig creates a new NeuronConfig from environment variables.
func NewNeuronConfig() *NeuronConfig {
	config := &NeuronConfig{
		VLLMImage:                 os.Getenv("ECO_HWACCEL_NEURON_VLLM_IMAGE"),
		ModelName:                 os.Getenv("ECO_HWACCEL_NEURON_MODEL_NAME"),
		HuggingFaceToken:          os.Getenv("ECO_HWACCEL_NEURON_HF_TOKEN"),
		DriversImage:              os.Getenv("ECO_HWACCEL_NEURON_DRIVERS_IMAGE"),
		DriverVersion:             os.Getenv("ECO_HWACCEL_NEURON_DRIVER_VERSION"),
		DevicePluginImage:         os.Getenv("ECO_HWACCEL_NEURON_DEVICE_PLUGIN_IMAGE"),
		SchedulerImage:            os.Getenv("ECO_HWACCEL_NEURON_SCHEDULER_IMAGE"),
		SchedulerExtensionImage:   os.Getenv("ECO_HWACCEL_NEURON_SCHEDULER_EXTENSION_IMAGE"),
		NodeMetricsImage:          os.Getenv("ECO_HWACCEL_NEURON_NODE_METRICS_IMAGE"),
		CatalogSource:             os.Getenv("ECO_HWACCEL_NEURON_CATALOG_SOURCE"),
		CatalogSourceNamespace:    os.Getenv("ECO_HWACCEL_NEURON_CATALOG_SOURCE_NAMESPACE"),
		SubscriptionName:          os.Getenv("ECO_HWACCEL_NEURON_SUBSCRIPTION_NAME"),
		UpgradeTargetVersion:      os.Getenv("ECO_HWACCEL_NEURON_UPGRADE_TARGET_VERSION"),
		UpgradeTargetDriversImage: os.Getenv("ECO_HWACCEL_NEURON_UPGRADE_TARGET_DRIVERS_IMAGE"),
		ImageRepoSecretName:       os.Getenv("ECO_HWACCEL_NEURON_IMAGE_REPO_SECRET"),
		InstanceType:              os.Getenv("ECO_HWACCEL_NEURON_INSTANCE_TYPE"),
		StorageClassName:          os.Getenv("ECO_HWACCEL_NEURON_STORAGE_CLASS"),
		KServeModelName:           os.Getenv("ECO_HWACCEL_NEURON_KSERVE_MODEL_NAME"),
		KServeVLLMImage:           os.Getenv("ECO_HWACCEL_NEURON_KSERVE_VLLM_IMAGE"),
		KServeNamespace:           os.Getenv("ECO_HWACCEL_NEURON_KSERVE_NAMESPACE"),
		KServeTensorParallelSize:  os.Getenv("ECO_HWACCEL_NEURON_KSERVE_TENSOR_PARALLEL_SIZE"),
	}

	// Set defaults
	if config.CatalogSourceNamespace == "" {
		config.CatalogSourceNamespace = "openshift-marketplace"
	}

	if config.SubscriptionName == "" {
		config.SubscriptionName = "aws-neuron-operator"
	}

	if config.ModelName == "" {
		config.ModelName = "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
	}

	if config.VLLMImage == "" {
		// Default vLLM image with Neuron support
		config.VLLMImage = "public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04"
	}

	if config.StorageClassName == "" {
		config.StorageClassName = "gp3-csi"
	}

	if config.KServeModelName == "" {
		config.KServeModelName = "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
	}

	if config.KServeNamespace == "" {
		config.KServeNamespace = "neuron-inference"
	}

	if config.KServeTensorParallelSize == "" {
		config.KServeTensorParallelSize = "1"
	}

	klog.V(params.NeuronLogLevel).Infof("NeuronConfig loaded: DriversImage=%s, DevicePluginImage=%s, NodeMetricsImage=%s",
		config.DriversImage, config.DevicePluginImage, config.NodeMetricsImage)

	return config
}

// IsValid checks if the minimum required configuration is present.
func (c *NeuronConfig) IsValid() bool {
	return c.DriversImage != "" && c.DriverVersion != "" && c.DevicePluginImage != "" && c.NodeMetricsImage != ""
}

// IsVLLMConfigured checks if vLLM testing configuration is present.
func (c *NeuronConfig) IsVLLMConfigured() bool {
	return c.HuggingFaceToken != ""
}

// IsUpgradeConfigured checks if upgrade testing configuration is present.
func (c *NeuronConfig) IsUpgradeConfigured() bool {
	return c.UpgradeTargetVersion != "" && c.UpgradeTargetDriversImage != ""
}

// IsKServeConfigured checks if KServe testing configuration is present.
func (c *NeuronConfig) IsKServeConfigured() bool {
	return c.HuggingFaceToken != "" && c.KServeNamespace != ""
}
