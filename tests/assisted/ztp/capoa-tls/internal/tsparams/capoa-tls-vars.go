package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ztpparams.Labels, LabelSuite)

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"multicluster-engine": "mce",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}
)
