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
		CatalogSource:             os.Getenv("ECO_HWACCEL_NEURON_CATALOG_SOURCE"),
		CatalogSourceNamespace:    os.Getenv("ECO_HWACCEL_NEURON_CATALOG_SOURCE_NAMESPACE"),
		SubscriptionName:          os.Getenv("ECO_HWACCEL_NEURON_SUBSCRIPTION_NAME"),
		UpgradeTargetVersion:      os.Getenv("ECO_HWACCEL_NEURON_UPGRADE_TARGET_VERSION"),
		UpgradeTargetDriversImage: os.Getenv("ECO_HWACCEL_NEURON_UPGRADE_TARGET_DRIVERS_IMAGE"),
		ImageRepoSecretName:       os.Getenv("ECO_HWACCEL_NEURON_IMAGE_REPO_SECRET"),
		InstanceType:              os.Getenv("ECO_HWACCEL_NEURON_INSTANCE_TYPE"),
		StorageClassName:          os.Getenv("ECO_HWACCEL_NEURON_STORAGE_CLASS"),
	}

	// Set defaults
	if config.CatalogSourceNamespace == "" {
		config.CatalogSourceNamespace = "openshift-marketplace"
	}

	if config.SubscriptionName == "" {
		config.SubscriptionName = "aws-neuron-operator"
	}

	if config.ModelName == "" {
		// Default to Llama-3.1-8B-Instruct as recommended in the Red Hat Developers blog
		// Supported architectures: LlamaForCausalLM, MistralForCausalLM, DbrxForCausalLM,
		// MixtralForCausalLM, MllamaForConditionalGeneration, Qwen2ForCausalLM
		// NOTE: OPTForCausalLM (facebook/opt-*) is NOT supported on Neuron
		config.ModelName = "meta-llama/Llama-3.1-8B-Instruct"
	}

	if config.VLLMImage == "" {
		// Default vLLM image with Neuron support from the Red Hat Developers blog
		config.VLLMImage = "public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04"
	}

	if config.StorageClassName == "" {
		// Default storage class for ROSA/AWS
		config.StorageClassName = "gp3-csi"
	}

	klog.V(params.NeuronLogLevel).Infof("NeuronConfig loaded: DriversImage=%s, DevicePluginImage=%s",
		config.DriversImage, config.DevicePluginImage)

	return config
}

// IsValid checks if the minimum required configuration is present.
// DeviceConfig requires DriversImage, DriverVersion, and DevicePluginImage.
func (c *NeuronConfig) IsValid() bool {
	return c.DriversImage != "" && c.DriverVersion != "" && c.DevicePluginImage != ""
}

// IsVLLMConfigured checks if vLLM testing configuration is present.
func (c *NeuronConfig) IsVLLMConfigured() bool {
	return c.VLLMImage != ""
}

// IsUpgradeConfigured checks if upgrade testing configuration is present.
func (c *NeuronConfig) IsUpgradeConfigured() bool {
	return c.UpgradeTargetVersion != "" && c.UpgradeTargetDriversImage != ""
}
