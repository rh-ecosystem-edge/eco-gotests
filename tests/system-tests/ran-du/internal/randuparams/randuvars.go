package randuparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{systemtestsparams.Label, Label}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"test":          "randu-test-workload",
		"openshift-ptp": "randu-ptp-namespace",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}

	// TestNamespaceName is used for defining the namespace name where test resources are created.
	TestNamespaceName = "ran-du-system-tests"

	// TestMultipleLaunchWorkloadLoadAvg is used for defining the node load average threshold to be
	// used in the LaunchWorkloadMultipleIterations test.
	TestMultipleLaunchWorkloadLoadAvg = 100
)
