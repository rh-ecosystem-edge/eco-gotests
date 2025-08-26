package mdrparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{rhwaparams.Label, Label}
	// OperatorDeploymentName represents MDR deployment name.
	OperatorDeploymentName = "machine-deletion-remediation-controller-manager"
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		rhwaparams.RhwaOperatorNs: rhwaparams.RhwaOperatorNs,
		"openshift-machine-api":   "openshift-machine-api",
	}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	// For first test, before medik8s API added.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}
)
