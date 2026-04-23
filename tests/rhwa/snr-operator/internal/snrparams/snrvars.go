package snrparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{rhwaparams.Label, Label}

	// OperatorDeploymentName represents SNR deployment name.
	OperatorDeploymentName = "self-node-remediation-controller-manager"

	// OperatorControllerPodLabel is how the controller pod is labeled.
	OperatorControllerPodLabel = "self-node-remediation-operator"

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		rhwaparams.RhwaOperatorNs: rhwaparams.RhwaOperatorNs,
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}
)
