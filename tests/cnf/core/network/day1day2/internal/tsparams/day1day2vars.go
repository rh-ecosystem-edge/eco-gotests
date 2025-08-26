package tsparams

import (
	nmstatev1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	nmstatev1beta1 "github.com/nmstate/kubernetes-nmstate/api/v1beta1"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"

	"github.com/openshift-kni/k8sreporter"
)

var (
	// TestNamespaceName day1day2 namespace where all test cases are performed.
	TestNamespaceName = "day1day2-tests"

	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &nmstatev1.NMStateList{}},
		{Cr: &nmstatev1.NodeNetworkConfigurationPolicyList{}},
		{Cr: &nmstatev1beta1.NodeNetworkConfigurationEnactmentList{}},
		{Cr: &nmstatev1beta1.NodeNetworkStateList{}},
	}

	// ReporterNamespacesToDump tells to the reporter what namespaces to dump.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.NMStateOperatorNamespace: NetConfig.NMStateOperatorNamespace,
		TestNamespaceName:                  "other",
		"openshift-nmstate":                "nmstate operator",
	}
)
