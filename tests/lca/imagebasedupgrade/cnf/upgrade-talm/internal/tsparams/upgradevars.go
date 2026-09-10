package tsparams

import (
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	cguv1alpha1 "github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	oadpv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/oadp/api/v1alpha1"
	olmv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
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
		{Cr: &cguv1alpha1.ClusterGroupUpgradeList{}},
	}

	// ReporterSpokeNamespacesToDump tells the reporter which namespaces on the spokes to collect pod logs from.
	ReporterSpokeNamespacesToDump = map[string]string{
		LCANamespace:                     "lca",
		LCAWorkloadName:                  "workload",
		LCAKlusterletNamespace:           "klusterlet",
		LCAOADPNamespace:                 "oadp",
		CNFConfig.SriovOperatorNamespace: "sriov",
		CNFConfig.MCONamespace:           "mco",
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
		{Cr: &machineconfigv1.MachineConfigPoolList{}},
		{Cr: &olmv1alpha1.ClusterServiceVersionList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &corev1.NodeList{}},
		{Cr: &certificatesv1.CertificateSigningRequestList{}},
		{Cr: &velerov1.BackupStorageLocationList{}},
		{Cr: &velerov1.BackupList{}},
		{Cr: &velerov1.RestoreList{}},
		{Cr: &oadpv1alpha1.DataProtectionApplicationList{}},
	}

	// TargetSnoClusterName is the name of target sno cluster.
	TargetSnoClusterName string
)
