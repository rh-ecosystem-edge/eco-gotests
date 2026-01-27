package tsparams

import "time"

const (
	// LabelSuite represents vLLM test suite label.
	LabelSuite = "vllm"

	// VLLMTestNamespace represents the namespace for vLLM tests.
	VLLMTestNamespace = "neuron-vllm-test"

	// VLLMStartupTimeout represents the timeout for vLLM pod startup.
	VLLMStartupTimeout = 20 * time.Minute
	// VLLMInferenceTimeout represents the timeout for inference requests.
	VLLMInferenceTimeout = 2 * time.Minute
	// OperatorDeployTimeout represents the timeout for operator deployment.
	OperatorDeployTimeout = 10 * time.Minute
	// DevicePluginReadyTimeout represents the timeout for device plugin readiness.
	DevicePluginReadyTimeout = 10 * time.Minute
)
