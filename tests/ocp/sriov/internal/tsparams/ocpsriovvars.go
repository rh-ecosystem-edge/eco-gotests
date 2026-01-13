// Package tsparams provides test suite parameters and constants for OCP SR-IOV tests.
package tsparams

import (
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
)

var (
	// ClientIPv4IPAddress represents the full test client IPv4 address.
	ClientIPv4IPAddress = "192.168.0.1/24"
	// ServerIPv4IPAddress represents the full test server IPv4 address.
	ServerIPv4IPAddress = "192.168.0.2/24"
	// Labels represent the suite-level labels applied to all tests in the suite.
	// Feature-specific labels (LabelBasic, LabelGUI, etc.) should be applied to individual tests.
	Labels = []string{LabelSuite}

	// ReporterCRDsToDump tells to the reporter what CRDs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
	}

	// ReporterNamespacesToDump tells to the reporter what namespaces to dump.
	ReporterNamespacesToDump = map[string]string{
		SriovOcpConfig.OcpSriovOperatorNamespace: SriovOcpConfig.OcpSriovOperatorNamespace,
		TestNamespaceName:                        "other"}
)
