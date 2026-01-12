package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
)

var (
	// Labels is the labels applied to all test cases in the suite.
	Labels = append(ranparam.Labels, LabelSuite)

	// ReporterSpokeNamespacesToDump tells the reporter which namespaces on the spoke to collect pod logs from.
	ReporterSpokeNamespacesToDump = map[string]string{
		ranparam.PtpOperatorNamespace: "",
		CloudEventsNamespace:          "",
	}

	// ReporterSpokeCRsToDump is the CRs the reporter should dump on the spoke.
	ReporterSpokeCRsToDump = []k8sreporter.CRData{
		{Cr: &ptpv1.PtpOperatorConfigList{}},
		{Cr: &ptpv1.PtpConfigList{}},
		{Cr: &ptpv1.NodePtpDeviceList{}},
		{Cr: &appsv1.DeploymentList{}, Namespace: ptr.To(CloudEventsNamespace)},
		{Cr: &appsv1.DaemonSetList{}, Namespace: ptr.To(string(ranparam.PtpOperatorNamespace))},
	}
)
