package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	mcmV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm-hub/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(kmmparams.Labels, LabelSuite)

	// ReporterNamespacesToDump configures reporter namespaces to dump.
	ReporterNamespacesToDump = map[string]string{
		kmmparams.KmmHubOperatorNamespace: "kmm-hub",
	}

	// ReporterCRDsToDump configures the CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcmV1Beta1.ManagedClusterModuleList{}},
	}
)
