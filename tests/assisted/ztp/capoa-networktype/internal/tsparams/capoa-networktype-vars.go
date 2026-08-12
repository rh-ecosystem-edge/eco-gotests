package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	hiveext "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	v1alpha3 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/capoa/controlplane/v1alpha3"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ztpparams.Labels, LabelSuite)

	// HubPullSecretData is set by BeforeSuite with the hub cluster pull secret data.
	HubPullSecretData map[string][]byte

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"multicluster-engine": "mce",
		TestNamespace:         "capoa-networktype test namespace",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &v1alpha3.OpenshiftAssistedControlPlaneList{}},
		{Cr: &hiveext.AgentClusterInstallList{}},
		{Cr: &hivev1.ClusterDeploymentList{}},
	}
)
