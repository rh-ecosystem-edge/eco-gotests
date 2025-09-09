package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtparams"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(mgmtparams.Labels, LabelSuite)

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		mgmtparams.LCANamespace:           "lca",
		mgmtparams.LCAWorkloadName:        "workload",
		mgmtparams.LCAKlusterletNamespace: "klusterlet",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &batchv1.JobList{}},
		{Cr: &corev1.ConfigMapList{}},
		{Cr: &appsv1.DeploymentList{}},
		{Cr: &corev1.ServiceList{}},
		{Cr: &lcav1.ImageBasedUpgradeList{}},
		{Cr: &configv1.ClusterOperatorList{}},
	}
)
