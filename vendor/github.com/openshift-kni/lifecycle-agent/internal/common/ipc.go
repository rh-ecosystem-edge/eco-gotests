package common

import (
	"strings"
)

type IPConfigPrePivotConfig struct {
	NewStaterootName              string   `json:"new-stateroot-name,omitempty"`
	IPv4Address                   string   `json:"ipv4-address,omitempty"`
	IPv4MachineNetwork            string   `json:"ipv4-machine-network,omitempty"`
	IPv6Address                   string   `json:"ipv6-address,omitempty"`
	IPv6MachineNetwork            string   `json:"ipv6-machine-network,omitempty"`
	DesiredIPv4Gateway            string   `json:"ipv4-gateway,omitempty"`
	DesiredIPv6Gateway            string   `json:"ipv6-gateway,omitempty"`
	CurrentIPv4Gateway            string   `json:"current-ipv4-gateway,omitempty"`
	CurrentIPv6Gateway            string   `json:"current-ipv6-gateway,omitempty"`
	DNSServers                    []string `json:"dns-servers,omitempty"`
	VLANID                        int      `json:"vlan-id,omitempty"`
	DNSFilterOutFamily            string   `json:"dns-filter-out-family,omitempty"`
	PullSecretRefName             string   `json:"pull-secret-ref-name,omitempty"`
	InstallInitMonitor            bool     `json:"install-init-monitor,omitempty"`
	InstallIPConfigurationService bool     `json:"install-ip-configuration-service,omitempty"`
}

type IPConfigPostPivotConfig struct {
	RecertImage string `json:"recert-image,omitempty"`
}

// DetectClusterIPFamilies inspects cluster info to determine whether the
// cluster is configured with IPv4, IPv6, or both (dual-stack).
func DetectClusterIPFamilies(ips []string) (bool, bool) {
	var nodeIPv4, nodeIPv6 string
	for _, ip := range ips {
		if strings.Contains(ip, ":") {
			if nodeIPv6 == "" {
				nodeIPv6 = ip
			}
		} else {
			if nodeIPv4 == "" {
				nodeIPv4 = ip
			}
		}
	}

	clusterHasIPv4 := nodeIPv4 != ""
	clusterHasIPv6 := nodeIPv6 != ""

	return clusterHasIPv4, clusterHasIPv6
}
