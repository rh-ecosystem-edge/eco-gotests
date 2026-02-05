package tsparams

import (
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	nmstateV1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	nmstateV1beta1 "github.com/nmstate/kubernetes-nmstate/api/v1beta1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// WaitTimeout represents timeout for the most ginkgo Eventually functions.
	WaitTimeout = 3 * time.Minute
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// DefaultStableDuration represents the default stableDuration for most StableFor functions.
	DefaultStableDuration = 10 * time.Second
	// RetryInterval represents retry interval for the most ginkgo Eventually functions.
	RetryInterval = 3 * time.Second
	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 35 * time.Minute
	// PollingIntervalBMC interval to poll the BMC after an error.
	PollingIntervalBMC = 30 * time.Second
	// NADWaitTimeout represents timeout for the most ginkgo Eventually functions for NAD creation.
	NADWaitTimeout = 5 * time.Second
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
		{Cr: &nmstateV1.NMStateList{}},
		{Cr: &nmstateV1.NodeNetworkConfigurationPolicyList{}},
		{Cr: &nmstateV1beta1.NodeNetworkStateList{}},
		{Cr: &nmstateV1beta1.NodeNetworkConfigurationEnactmentList{}},
	}

	// ReporterNamespacesToDump tells to the reporter what namespaces to dump.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.SriovOperatorNamespace: NetConfig.SriovOperatorNamespace,
		TestNamespaceName:                "other",
		"openshift-nmstate":              "nmstate operator",
	}
	// ClientIPv4IPAddress represents the IPv4 address (with CIDR range) for test pods.
	ClientIPv4IPAddress = "192.168.0.1/24"
	// ServerIPv4IPAddress represents the IPv4 address (with CIDR range) for test pods.
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
	// ClientMacAddress2 represents the second test client MAC address.
	ClientMacAddress2 = "20:04:0f:f1:88:03"
	// ServerMacAddress2 represents the second test server MAC address.
	ServerMacAddress2 = "20:04:0f:f1:88:04"

	// WhereaboutsIPv4Range is the IP range for whereabouts IPAM.
	WhereaboutsIPv4Range = "192.168.100.0/24"
	// WhereaboutsIPv4Gateway is the gateway for whereabouts IPAM.
	WhereaboutsIPv4Gateway = "192.168.100.1"
	// WhereaboutsIPv4Range2 is an additional IP range for whereabouts IPAM (separate range to avoid conflicts).
	WhereaboutsIPv4Range2 = "192.168.101.0/24"
	// WhereaboutsIPv4Gateway2 is the gateway for whereabouts IPAM range 2.
	WhereaboutsIPv4Gateway2 = "192.168.101.1"

	// OperatorConfigDaemon defaults SR-IOV config daemon daemonset.
	OperatorConfigDaemon = "sriov-network-config-daemon"
	// OperatorWebhook defaults SR-IOV webhook daemonset.
	OperatorWebhook = "operator-webhook"
	// OperatorResourceInjector defaults SR-IOV network resource injector daemonset.
	OperatorResourceInjector = "network-resources-injector"
	// OperatorSriovDaemonsets represents all default SR-IOV operator daemonset names.
	OperatorSriovDaemonsets = []string{OperatorConfigDaemon, OperatorWebhook, OperatorResourceInjector}
)
