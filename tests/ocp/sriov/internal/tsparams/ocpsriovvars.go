// Package tsparams provides test suite parameters and constants for OCP SR-IOV tests.
package tsparams

import (
	"time"

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
	// ClientIPv4IPAddress2 represents the IPv4 address (with CIDR range) for a second test pod.
	ClientIPv4IPAddress2 = "192.168.1.1/24"
	// ServerIPv4IPAddress2 represents the IPv4 address (with CIDR range) for a second test pod.
	ServerIPv4IPAddress2 = "192.168.1.2/24"
	// ClientIPv6IPAddress represents the full test IPv6 address.
	ClientIPv6IPAddress = "2001::1/64"
	// ServerIPv6IPAddress represents the full test IPv6 address.
	ServerIPv6IPAddress = "2001::2/64"
	// ClientIPv6IPAddress2 represents the full test IPv6 address.
	ClientIPv6IPAddress2 = "2001:100::1/64"
	// ServerIPv6IPAddress2 represents the full test IPv6 address.
	ServerIPv6IPAddress2 = "2001:100::2/64"
	// ClientMacAddress represents the test client MAC address.
	ClientMacAddress = "20:04:0f:f1:88:01"
	// ServerMacAddress represents the test server MAC address.
	ServerMacAddress = "20:04:0f:f1:88:02"
	// NADWaitTimeout represents timeout for NAD creation in QinQ tests.
	NADWaitTimeout = 30 * time.Second
	// ClusterMonitoringNSLabel represents Cluster Monitoring label for a NS to enable Prometheus Scraping.
	ClusterMonitoringNSLabel = map[string]string{"openshift.io/cluster-monitoring": "true"}
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
