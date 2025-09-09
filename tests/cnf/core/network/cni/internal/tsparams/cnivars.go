package tsparams

import (
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/openshift-kni/k8sreporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// LabelTapTestCases tap test cases label.
	LabelTapTestCases = "tap"
	// TestNamespaceName cni namespace where all test cases are performed.
	TestNamespaceName = "cni-tests"
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.MultusNamesapce: NetConfig.MultusNamesapce,
		TestNamespaceName:         "other",
	}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &nadv1.NetworkAttachmentDefinitionList{}}}
)
