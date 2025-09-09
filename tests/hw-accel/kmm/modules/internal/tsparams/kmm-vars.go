package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(kmmparams.Labels, kmmparams.LabelSuite)

	// LocalImageRegistry represents the local registry used in KMM tests.
	LocalImageRegistry = "image-registry.openshift-image-registry.svc:5000"

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		kmmparams.KmmOperatorNamespace:            "kmm",
		kmmparams.UseDtkModuleTestNamespace:       "module",
		kmmparams.SimpleKmodModuleTestNamespace:   "module",
		kmmparams.DevicePluginTestNamespace:       "module",
		kmmparams.RealtimeKernelNamespace:         "module",
		kmmparams.FirmwareTestNamespace:           "module",
		kmmparams.ModuleBuildAndSignNamespace:     "module",
		kmmparams.InTreeReplacementNamespace:      "module",
		kmmparams.UseLocalMultiStageTestNamespace: "module",
		kmmparams.WebhookModuleTestNamespace:      "module",
		kmmparams.MultipleModuleTestNamespace:     "module",
		kmmparams.VersionModuleTestNamespace:      "module",
		kmmparams.ScannerTestNamespace:            "module",
		kmmparams.TolerationModuleTestNamespace:   "module",
		kmmparams.DefaultNodesNamespace:           "nodes",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &moduleV1Beta1.ModuleList{}},
		{Cr: &corev1.EventList{}},
	}
)
