package farparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{rhwaparams.Label, Label}

	// OperatorDeploymentName represents FAR deployment name.
	OperatorDeploymentName = "fence-agents-remediation-controller-manager"

	// OperatorControllerPodLabel is how the controller pod is labeled.
	OperatorControllerPodLabel = "fence-agents-remediation-operator"

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

	// RequiredAnnotations defines the required annotations and their expected values for FAR CSV.
	RequiredAnnotations = map[string]string{
		"features.operators.openshift.io/tls-profiles":   "false",
		"features.operators.openshift.io/disconnected":   "true",
		"features.operators.openshift.io/fips-compliant": "true",
		"features.operators.openshift.io/proxy-aware":    "false",
		"features.operators.openshift.io/cnf":            "false",
		"features.operators.openshift.io/cni":            "false",
		"features.operators.openshift.io/csi":            "false",
		"operatorframework.io/suggested-namespace":       rhwaparams.RhwaOperatorNs,
	}
)
