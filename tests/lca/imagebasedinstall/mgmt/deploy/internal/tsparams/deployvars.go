package tsparams

import (
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	ibiv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedinstall/api/hiveextensions/v1alpha1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"

	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtparams"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(mgmtparams.Labels, LabelSuite)

	// RHACMNamespace holds the namespace for ACM resources.
	RHACMNamespace = "rhacm"

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		mgmtparams.IBIONamespace: "ibio",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &corev1.SecretList{}},
		{Cr: &corev1.ConfigMapList{}},
		{Cr: &appsv1.DeploymentList{}},
		{Cr: &corev1.ServiceList{}},
		{Cr: &ibiv1alpha1.ImageClusterInstallList{}},
		{Cr: &hivev1.ClusterImageSetList{}},
		{Cr: &hivev1.ClusterDeploymentList{}},
		{Cr: &siteconfigv1alpha1.ClusterInstanceList{}},
		{Cr: &bmhv1alpha1.BareMetalHostList{}},
	}
)
