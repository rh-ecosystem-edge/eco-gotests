package tsparams

import (
	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	configv1 "github.com/openshift/api/config/v1"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/openshift-kni/k8sreporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
)

const (
	// LCANamespace is the namespace used by the lifecycle-agent.
	LCANamespace = "openshift-lifecycle-agent"

	// LCAWorkloadName is the name used for creating resources needed to backup workload app.
	LCAWorkloadName = "ibu-workload-app"

	// LCAOADPNamespace is the namespace used by the OADP operator.
	LCAOADPNamespace = "openshift-adp"

	// LCAKlusterletNamespace is the namespace that contains the klusterlet.
	LCAKlusterletNamespace = "open-cluster-management-agent"

	// OCPOperatorsNamespace is the namespace where ocp operators are installed.
	OCPOperatorsNamespace = "openshift-operators"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(cnfparams.Labels, LabelSuite)

	// ReporterHubNamespacesToDump tells to the reporter which namespaces on the hub to collect pod logs from.
	ReporterHubNamespacesToDump = map[string]string{
		OCPOperatorsNamespace:          "",
		CNFConfig.AcmOperatorNamespace: "",
	}

	// ReporterHubCRsToDump is the CRs the reporter should dump on the hub.
	ReporterHubCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.NamespaceList{}},
		{Cr: &corev1.PodList{}},
		{Cr: &policiesv1.PolicyList{}},
		{Cr: &ibguv1alpha1.ImageBasedGroupUpgradeList{}},
	}

	// ReporterSpokeNamespacesToDump tells the reporter which namespaces on the spokes to collect pod logs from.
	ReporterSpokeNamespacesToDump = map[string]string{
		LCANamespace:           "lca",
		LCAWorkloadName:        "workload",
		LCAKlusterletNamespace: "klusterlet",
	}

	// ReporterSpokeCRsToDump is the CRs the reporter should dump on the spokes.
	ReporterSpokeCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &batchv1.JobList{}},
		{Cr: &corev1.ConfigMapList{}},
		{Cr: &appsv1.DeploymentList{}},
		{Cr: &corev1.ServiceList{}},
		{Cr: &lcav1.ImageBasedUpgradeList{}},
		{Cr: &configv1.ClusterOperatorList{}},
	}

	// TargetSnoClusterName is the name of target sno cluster.
	TargetSnoClusterName string

	// ClusterLabelSelector is the cluster label passed to IBGUs.
	ClusterLabelSelector = map[string]string{"common": "true"}
)
