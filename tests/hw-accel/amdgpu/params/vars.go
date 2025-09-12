package params

import (
	"github.com/openshift-kni/k8sreporter"
	amdgpuv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/amd/gpu-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{hwaccelparams.Label, Label}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		hwaccelparams.NFDNamespace: "nfd-operator",
		"amd-gpu-operator":         "openshift-amd-gpu",
		"openshift-kmm":            "kmm-operator",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &amdgpuv1.DeviceConfigList{}},
	}
)

var (
	// DeviceIDsMap - Map is taken from: https://instinct.docs.amd.com/projects/gpu-operator/en/release-v1.2.1/
	// installation/openshift-olm.html#create-node-feature-discovery-rule .
	DeviceIDsMap = map[string]string{
		"74a5": "MI325X",
		"74a0": "MI300A",
		"74a1": "MI300X",
		"74a9": "MI300X-HF",
		"74bd": "MI300X-HF",
		"740f": "MI210",
		"7408": "MI250X",
		"740c": "MI250/MI250X",
		"738c": "MI100",
		"738e": "MI100",
	}

	// NodeLabellerLabels - List of labeles that should be added by the Node Labeller.
	NodeLabellerLabels = []string{
		"amd.com/gpu.cu-count",
		"amd.com/gpu.device-id",
		"amd.com/gpu.driver-version",
		"amd.com/gpu.family",
		"amd.com/gpu.simd-count",
		"amd.com/gpu.vram",
		"beta.amd.com/gpu.cu-count",
		"beta.amd.com/gpu.device-id",
		"beta.amd.com/gpu.family",
		"beta.amd.com/gpu.simd-count",
		"beta.amd.com/gpu.vram",
	}
)
