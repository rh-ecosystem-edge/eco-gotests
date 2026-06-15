package tsparams

import (
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/ranparams"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ranparams.Labels, LabelSuite)

	// ReporterNamespacesToDump tells the reporter from where to collect logs on failure.
	ReporterNamespacesToDump = map[string]string{
		"openshift-config":      "hub-config",
		"openshift-machine-api": "machine-api",
	}

	// ReporterCRDsToDump tells the reporter what CRs to dump on failure.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &corev1.SecretList{}},
		{Cr: &corev1.ConfigMapList{}},
		{Cr: &appsv1.DeploymentList{}},
		{Cr: &bmhv1alpha1.BareMetalHostList{}},
	}
)
