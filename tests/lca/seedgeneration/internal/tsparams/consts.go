package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	lcasgv1 "github.com/openshift-kni/lifecycle-agent/api/seedgenerator/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	// LabelSuite represents seedgeneration label that can be used for test cases selection.
	LabelSuite = "seedgeneration"

	// LCANamespace is the namespace used by the lifecycle-agent.
	LCANamespace = "openshift-lifecycle-agent"
)

var (
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		LCANamespace: "openshift-lifecycle-agent",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}, Namespace: ptr.To(LCANamespace)},
		{Cr: &corev1.SecretList{}, Namespace: ptr.To(LCANamespace)},
		{Cr: &lcasgv1.SeedGeneratorList{}},
	}
)
