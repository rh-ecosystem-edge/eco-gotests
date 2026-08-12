package tsparams

import (
	"fmt"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/openshift-kni/k8sreporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

const (
	// MultusFirstInterfaceName is the default name of the first secondary network interface.
	MultusFirstInterfaceName = "net1"
	// ClientIPv4 is the sysctl client address on the secondary network.
	ClientIPv4 = "10.100.100.210"
	// ServerIPv4 is the sysctl server address on the secondary network.
	ServerIPv4 = "10.100.100.200"
	// RedirectIPv4 is the sysctl redirect-pod address on the secondary network.
	RedirectIPv4 = "10.100.100.1"
	// ClientIPv4CIDR is ClientIPv4 with the secondary-network prefix.
	ClientIPv4CIDR = ClientIPv4 + "/24"
	// ServerIPv4CIDR is ServerIPv4 with the secondary-network prefix.
	ServerIPv4CIDR = ServerIPv4 + "/24"
	// RedirectIPv4CIDR is RedirectIPv4 with the secondary-network prefix.
	RedirectIPv4CIDR = RedirectIPv4 + "/24"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// LabelTapTestCases tap test cases label.
	LabelTapTestCases = "tap"
	// LabelSysctlTestCases sysctl test cases label.
	LabelSysctlTestCases = "sysctl"
	// TestNamespaceName cni namespace where all test cases are performed.
	TestNamespaceName = "cni-tests"
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.MultusNamesapce:        NetConfig.MultusNamesapce,
		NetConfig.SriovOperatorNamespace: NetConfig.SriovOperatorNamespace,
		TestNamespaceName:                "other",
	}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &nadv1.NetworkAttachmentDefinitionList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
	}
	// NetworkWithSysctlMutation is the NAD name with accept_redirects sysctl mutation.
	NetworkWithSysctlMutation = "test-sysct-mutation"
	// NetworkWithoutSysctlMutation is the NAD name without sysctl mutation.
	NetworkWithoutSysctlMutation = "test-no-sysct-mutation"
	// FirstSysctlNetworkName is the primary NAD name used by sysctl API tests.
	FirstSysctlNetworkName = "test-nad-sysctl-first"
	// AllFlagsSysctlPluginConfig contains all valid interface-level sysctl flags.
	AllFlagsSysctlPluginConfig = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects":        "0",
		"net.ipv4.conf.IFNAME.accept_source_route":     "0",
		"net.ipv4.conf.IFNAME.disable_policy":          "1",
		"net.ipv4.conf.IFNAME.secure_redirects":        "0",
		"net.ipv4.conf.IFNAME.send_redirects":          "0",
		"net.ipv6.conf.IFNAME.accept_redirects":        "0",
		"net.ipv6.conf.IFNAME.accept_source_route":     "1",
		"net.ipv6.neigh.IFNAME.base_reachable_time_ms": "20000",
		"net.ipv6.neigh.IFNAME.retrans_time_ms":        "2000",
	}
	// GlobalSysctlFlag is a global kernel sysctl that is not permitted via the tuning CNI.
	GlobalSysctlFlag = "kernel.shm_rmid_forced"
	// SingleSysctlFlag sets accept_redirects=0 on the pod interface.
	SingleSysctlFlag = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects": "0",
	}
	// SingleAcceptRedirectSysctlFlag is the default accept_redirects value when mutation is not applied.
	SingleAcceptRedirectSysctlFlag = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects": "1",
	}
	// SrvLopIPAddr is the server loopback destination address used for ICMP redirect tests.
	SrvLopIPAddr = "4.4.4.4"
	// SrvInitCMD configures the server pod routing for redirect tests.
	SrvInitCMD = fmt.Sprintf(
		"ip addr add %s/32 dev lo && ip route add blackhole %s/32", SrvLopIPAddr, RedirectIPv4)
	// RdrInitCMD is the redirect-pod init script body for createSysctlPod's bash -c.
	RdrInitCMD = fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true;"+
		" ip route add %s/32 via %s", SrvLopIPAddr, ServerIPv4)
	// ClientInitCMD is the client-pod init script body for createSysctlPod's bash -c.
	ClientInitCMD = fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true;"+
		" sysctl -w net.ipv4.conf.all.accept_redirects=1 && ip route add %s/32 via %s",
		SrvLopIPAddr, RedirectIPv4)
)
