package nmstate

import "net"

// DesiredState provides struct for the NMState desired state object containing all NMState configuration.
type DesiredState struct {
	Interfaces []NetworkInterface `yaml:"interfaces,omitempty"`
}

// NetworkInterface provides struct for the NMState interface state object containing interface information.
type NetworkInterface struct {
	Name            string          `yaml:"name"`
	Type            string          `yaml:"type"`
	State           string          `yaml:"state"`
	Ethernet        Ethernet        `yaml:"ethernet,omitempty"`
	Bridge          Bridge          `yaml:"bridge,omitempty"`
	LinkAggregation LinkAggregation `yaml:"link-aggregation,omitempty"`
	Vlan            Vlan            `yaml:"vlan,omitempty"`
	Ipv4            InterfaceIpv4   `yaml:"ipv4,omitempty"`
	Ipv6            InterfaceIpv6   `yaml:"ipv6,omitempty"`
}

// InterfaceIpv4 enables an IPv4 address on an interface.
type InterfaceIpv4 struct {
	Enabled bool                 `yaml:"enabled,omitempty"`
	Dhcp    bool                 `yaml:"dhcp,omitempty"`
	Address []InterfaceIPAddress `yaml:"address,omitempty"`
}

// InterfaceIpv6 enables an IPv6 address on an interface.
type InterfaceIpv6 struct {
	Enabled  bool                 `yaml:"enabled,omitempty"`
	Dhcp     bool                 `json:"dhcp,omitempty"`
	Autoconf bool                 `json:"autoconf,omitempty"`
	Address  []InterfaceIPAddress `yaml:"address,omitempty"`
}

// InterfaceIPAddress is a struct for constructing an IP with prefix.
type InterfaceIPAddress struct {
	IP        net.IP `yaml:"ip,omitempty"`
	PrefixLen uint8  `yaml:"prefix-length,omitempty"`
}

// Ethernet provides struct for the NMState Interface Ethernet state object containing interface Ethernet information.
type Ethernet struct {
	Sriov Sriov `yaml:"sr-iov,omitempty"`
}

// Sriov provides struct for the NMState Interface Ethernet Sriov state object containing
// interface Ethernet Sriov information.
type Sriov struct {
	TotalVfs *int `yaml:"total-vfs,omitempty"`
	Vfs      []Vf `yaml:"vfs,omitempty"`
}

// Vf provides struct for the NMState SR-IOV VF state object containing SR-IOV VF information.
type Vf struct {
	ID         int    `yaml:"id"`
	MacAddress string `yaml:"mac-address,omitempty"`
	MaxTxRate  *int   `yaml:"max-tx-rate,omitempty"`
	MinTxRate  *int   `yaml:"min-tx-rate,omitempty"`
	Qos        *int   `yaml:"qos,omitempty"`
	SpoofCheck *bool  `yaml:"spoof-check,omitempty"`
	Trust      *bool  `yaml:"trust,omitempty"`
	VlanID     *int   `yaml:"vlan-id,omitempty"`
}

// Bridge provides struct for the NMState Interface Ethernet Bridge state object
// containing interface Bridge information.
type Bridge struct {
	Port []map[string]string `yaml:"port,omitempty"`
}

// LinkAggregation provides struct for the NMState Interface Ethernet LinkAggregation state object
// containing interface LinkAggregation information.
type LinkAggregation struct {
	Mode    string                 `yaml:"mode"`
	Options OptionsLinkAggregation `yaml:"options,omitempty"`
	Port    []string               `yaml:"port,omitempty"`
}

// OptionsLinkAggregation provides struct for the NMState Interface Ethernet LinkAggregation Options state object
// containing interface LinkAggregation Options information.
type OptionsLinkAggregation struct {
	Primary     string `yaml:"primary,omitempty"`
	Miimon      int    `yaml:"miimon,omitempty"`
	FailOverMac string `yaml:"fail_over_mac,omitempty"`
	LacpRate    string `yaml:"lacp_rate,omitempty"`
	MinLinks    int    `yaml:"min_links,omitempty"`
}

// Vlan provides struct for the NMState Interface Ethernet Vlan Options state object
// containing interface Vlan information.
type Vlan struct {
	BaseIface string `yaml:"base-iface"`
	ID        int    `yaml:"id"`
}
