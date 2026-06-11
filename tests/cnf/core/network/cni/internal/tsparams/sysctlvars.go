package tsparams

import "fmt"

const (
	// MultusFirstInterfaceName is the default name of the first secondary network interface.
	MultusFirstInterfaceName = "net1"
	// SysctlPolicyNumVFs is the number of VFs configured by the sysctl SR-IOV policy.
	SysctlPolicyNumVFs = 12
	// SysctlPolicyVFStart is the first VF index in the sysctl SR-IOV policy range.
	SysctlPolicyVFStart = 0
	// SysctlPolicyVFEnd is the last VF index in the sysctl SR-IOV policy range.
	SysctlPolicyVFEnd = SysctlPolicyNumVFs - 1
	// SysctlPolicyMTU is the MTU configured on the sysctl SR-IOV policy.
	SysctlPolicyMTU = 1500
)

var (
	// NetworkWithSysctlMutation is the SriovNetwork name with accept_redirects sysctl mutation.
	NetworkWithSysctlMutation = "test-sysct-mutation"
	// NetworkWithoutSysctlMutation is the SriovNetwork name without sysctl mutation.
	NetworkWithoutSysctlMutation = "test-no-sysct-mutation"
	// ResourceNameSysctl is the SR-IOV resource name for sysctl tests.
	ResourceNameSysctl = "sriovnicsysctl"
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
		"ip addr add %s/32 dev lo && ip route add blackhole 10.100.100.1/32", SrvLopIPAddr)
	// RdrInitCMD configures the redirect pod routing for redirect tests.
	RdrInitCMD = fmt.Sprintf("bash -c 'echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true;"+
		" ip route add %s/32 via 10.100.100.200'", SrvLopIPAddr)
	// ClientInitCMD configures the client pod routing and enables accept_redirects globally.
	ClientInitCMD = fmt.Sprintf("bash -c 'echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true;"+
		" sysctl -w net.ipv4.conf.all.accept_redirects=1 && ip route add %s/32 via 10.100.100.1'", SrvLopIPAddr)
)
