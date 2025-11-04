package nodes

// ExternalNetworks contains external node ip4/ipv6 addresses.
type ExternalNetworks struct {
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
}

const ovnExternalAddresses = "k8s.ovn.org/node-primary-ifaddr"
