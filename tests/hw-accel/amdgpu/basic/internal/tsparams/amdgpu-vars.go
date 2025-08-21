package tsparams

import "github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"

// Re-export variables from amdgpuparams for convenience.
var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = amdgpuparams.Labels

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = amdgpuparams.ReporterNamespacesToDump

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = amdgpuparams.ReporterCRDsToDump
)
