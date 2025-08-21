package amdgpuparams

import (
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/openshift-kni/k8sreporter"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{hwaccelparams.Label, "amdgpu"}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		hwaccelparams.NFDNamespace: "nfd-operator",
		"openshift-kmm":            "kmm-operator",
		AMDGPUOperatorNamespace:    "amd-gpu-operator",
		AMDGPUTestNamespace:        "amd-gpu-test",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		// AMD GPU operator CRDs will be added when available
		// {Cr: &amdgpuv1.DeviceConfigList{}},
	}

	// ValidPodNameList contains expected AMD GPU operator pod names.
	ValidPodNameList = []string{
		"amd-gpu-operator-controller-manager",
		"amd-gpu-device-plugin",
		"amd-gpu-driver",
	}

	// DefaultWorkerConfig contains the NFD worker configuration for AMD GPU detection.
	DefaultWorkerConfig = `
sources:
  pci:
    deviceClassWhitelist:
      - "03"
      - "0300"  
      - "0302"
    deviceLabelFields:
      - vendor
      - device
      - class
      - subsystem_vendor
      - subsystem_device
  custom:
    - name: "amd-gpu"
      matchOn:
        - pciId:
            vendor: ["1002"]
            device: ["15d8", "67df", "6fdf", "731f", "73ef", "73ff", "7340", "744c"]
      labels:
        amd-gpu: "true"
`
)
