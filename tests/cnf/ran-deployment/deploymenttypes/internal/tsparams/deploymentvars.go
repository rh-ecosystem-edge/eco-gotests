package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdoperator"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	clusterV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/clusterv1"
	siteconfig "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/ranparam"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ranparam.Labels, LabelSuite)

	// ReporterHubCRsToDump is the CRs the reporter should dump on the hub.
	ReporterHubCRsToDump = []k8sreporter.CRData{
		{Cr: &policiesv1.PolicyList{}},
		{Cr: &argocdoperator.ArgoCDList{}},
		{Cr: &argocdtypes.ApplicationList{}},
		{Cr: &clusterV1.ManagedClusterList{}},
		{Cr: &hiveV1.ClusterDeploymentList{}},
		{Cr: &siteconfig.ClusterInstanceList{}},
	}

	// ReporterSpokeCRsToDump is the CRs the reporter should dump on the spokes.
	ReporterSpokeCRsToDump = []k8sreporter.CRData{
		{Cr: &policiesv1.PolicyList{}},
	}
)
