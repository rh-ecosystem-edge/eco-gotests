package params

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{hwaccelparams.Label, Label}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		hwaccelparams.NFDNamespace: "nfd-operator",
		NeuronNamespace:            "neuron-operator",
		"openshift-kmm":            "kmm-operator",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{}
)

var (
	// NeuronMetrics - List of Neuron metrics that should be available.
	NeuronMetrics = []string{
		"neuron_runtime_memory_used_bytes",
		"neuron_hardware_info",
		"neuroncore_utilization_ratio",
		"neuron_instance_info",
		"neuroncore_memory_usage_model_shared_scratchpad",
		"neuroncore_memory_usage_constants",
	}
)
