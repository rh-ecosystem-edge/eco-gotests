package tsparams

import (
	"time"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.SriovOperatorNamespace: NetConfig.SriovOperatorNamespace,
		NetConfig.MultusNamesapce:        NetConfig.MultusNamesapce,
		TestNamespaceName:                "other",
	}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
		{Cr: &nadv1.NetworkAttachmentDefinitionList{}},
		{Cr: &multinetpolicyapiv1.MultiNetworkPolicyList{}},
	}

	// MCOWaitTimeout represents timeout for mco operations.
	MCOWaitTimeout = time.Hour
	// MultiNetworkPolicyDSName represents multi-networkPolicy daemonSet name.
	MultiNetworkPolicyDSName = "multus-networkpolicy"
	// WaitTimeout represents timeout for the most ginkgo Eventually functions.
	WaitTimeout = 3 * time.Minute
	// RetryInterval represents retry interval for the most ginkgo Eventually functions.
	RetryInterval = 3 * time.Second
	// WaitTrafficTimeout represents timeout for the traffic Eventually functions.
	WaitTrafficTimeout = 1 * time.Minute
	// RetryTrafficInterval represents retry interval for the traffic Eventually functions.
	RetryTrafficInterval = 20 * time.Second
	// AllOpen represents that ports 5001,5002,5003 to be open.
	AllOpen = map[string]string{"5001": "pass", "5002": "pass", "5003": "pass"}
	// AllClose represents that ports 5001,5002,5003 to be close.
	AllClose = map[string]string{"5001": "fail", "5002": "fail", "5003": "fail"}
	// P5001Open represents that port 5001 to be open and 5002-3 to be closed.
	P5001Open = map[string]string{"5001": "pass", "5002": "fail", "5003": "fail"}
	// P5001p5002Open represents that port 5001 & 5002 to be open and 5003 to be closed.
	P5001p5002Open = map[string]string{"5001": "pass", "5002": "pass", "5003": "fail"}
	// Protocols indicates list of protocols used in policy tests.
	Protocols = []string{"tcp", "tcp", "udp"}
	// Ports indicates list of ports used in policy tests.
	Ports = []string{"5001", "5002", "5003"}
	// TestData represents test resource data for policy tests.
	TestData = PodsData{
		"pod1": {IPv4: "192.168.10.10/24", IPv6: "2001:0:0:1::10/64", Protocols: Protocols, Ports: Ports},
		"pod2": {IPv4: "192.168.10.11/24", IPv6: "2001:0:0:1::11/64", Protocols: Protocols, Ports: Ports},
		"pod3": {IPv4: "192.168.10.12/24", IPv6: "2001:0:0:1::12/64", Protocols: Protocols, Ports: Ports},
		"pod4": {IPv4: "192.168.20.11/24", IPv6: "2001:0:0:2::11/64", Protocols: Protocols, Ports: Ports},
		"pod5": {IPv4: "192.168.20.12/24", IPv6: "2001:0:0:2::12/64", Protocols: Protocols, Ports: Ports},
	}
)

// PodsData contains test pods data used for policy tests.
type PodsData map[string]struct {
	IPv4      string
	IPv6      string
	Protocols []string
	Ports     []string
}
