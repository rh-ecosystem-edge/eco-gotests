// Package tsparams provides test suite parameters and constants for OCP HWOL tests.
package tsparams

import (
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
)

var (
	// TestNamespaceName is assigned once before creating the suite namespace.
	TestNamespaceName string

	// Labels represent the suite-level labels applied to all tests in the suite.
	Labels = []string{LabelSuite}

	// ReporterCRDsToDump tells the reporter what CRDs to dump on failure.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkPoolConfigList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
		{Cr: &sriovv1.OVSNetworkList{}},
	}

	// ReporterNamespacesToDump tells the reporter what namespaces to dump on failure.
	// Use the default operator namespace literal so a nil HwolOcpConfig at package
	// init does not panic before suite setup can Fail clearly.
	ReporterNamespacesToDump = map[string]string{
		"openshift-sriov-network-operator": "openshift-sriov-network-operator",
	}
)
