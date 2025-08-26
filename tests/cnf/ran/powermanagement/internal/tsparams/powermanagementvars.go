package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ranparam.Labels, LabelSuite)

	// ReporterNamespacesToDump tells to the reporter which namespaces to collect pod logs from.
	ReporterNamespacesToDump = map[string]string{
		TestingNamespace:       "",
		RANConfig.MCONamespace: "",
	}

	// ReporterCRsToDump is the CRs the reporter should dump.
	ReporterCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &corev1.NamespaceList{}},
		{Cr: &performancev2.PerformanceProfileList{}},
	}
)
