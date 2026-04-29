package tsparams

import "time"

const (
	LabelSuite = "kserve"

	KServeTestNamespace = "neuron-inference"

	ServingRuntimeName = "vllm-neuron-runtime"

	ModelFormatName = "vllm-neuron"

	ServiceAccountName = "kserve-neuron-sa"

	InferenceServiceReadyTimeout = 30 * time.Minute

	InferenceRequestTimeout = 5 * time.Minute

	CurlPodName = "kserve-inference-test-curl"
)
