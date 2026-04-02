package networkconfig

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// NetworkConfig is the top-level struct containing
// a full nmstate network configuration.
type NetworkConfig struct {
	Interfaces  []Interface `yaml:"interfaces"`
	Routes      Routes      `yaml:"routes"`
	DNSResolver DNSResolver `yaml:"dns-resolver"`
}

// Interface defines an nmstate interface and its properties.
// For type: vlan, set VLAN to the VLAN ID as a string (same as mgmt config) and
// Name to "<base>.<vlan>" (e.g. enp1s0.100); MarshalYAML emits nmstate's nested
// vlan: { id, base-iface } block with base-iface derived from Name.
type Interface struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	State      string   `yaml:"state"`
	Identifier string   `yaml:"identifier"`
	MACAddress string   `yaml:"mac-address"`
	VLANID     string   `yaml:"vlan_id,omitempty"`
	IPv4       IPConfig `yaml:"ipv4"`
	IPv6       IPConfig `yaml:"ipv6"`
}

// MarshalYAML encodes VLAN as nmstate's mapping (id + base-iface), not a scalar string.
func (i Interface) MarshalYAML() (interface{}, error) {
	type ifaceYAML struct {
		Name       string   `yaml:"name"`
		Type       string   `yaml:"type"`
		State      string   `yaml:"state"`
		Identifier string   `yaml:"identifier,omitempty"`
		MACAddress string   `yaml:"mac-address,omitempty"`
		IPv4       IPConfig `yaml:"ipv4"`
		IPv6       IPConfig `yaml:"ipv6"`
	}

	aux := ifaceYAML{
		Name:       i.Name,
		Type:       i.Type,
		State:      i.State,
		Identifier: i.Identifier,
		MACAddress: i.MACAddress,
		IPv4:       i.IPv4,
		IPv6:       i.IPv6,
	}

	raw, err := yaml.Marshal(aux)
	if err != nil {
		return nil, err
	}

	var iface map[string]interface{}
	if err := yaml.Unmarshal(raw, &iface); err != nil {
		return nil, err
	}

	if strings.TrimSpace(i.VLANID) != "" && i.Type == "vlan" {
		vid := strings.TrimSpace(i.VLANID)

		vlanID, err := strconv.Atoi(vid)
		if err != nil {
			return nil, fmt.Errorf("interface %q: vlan must be a numeric VLAN ID: %w", i.Name, err)
		}

		suffix := "." + vid
		if !strings.HasSuffix(i.Name, suffix) || len(i.Name) <= len(suffix) {
			return nil, fmt.Errorf("interface %q: vlan interface name must be <base>%s", i.Name, suffix)
		}

		baseIface := strings.TrimSuffix(i.Name, suffix)
		iface["vlan"] = map[string]interface{}{
			"id":         vlanID,
			"base-iface": baseIface,
		}
	}

	return iface, nil
}

// Routes contains the route configuration portion of the nmstate configuration.
type Routes struct {
	Config []RouteConfig `yaml:"config"`
}

// RouteConfig defines an nmstate route and its properties.
type RouteConfig struct {
	Destination      string `yaml:"destination"`
	NextHopAddress   string `yaml:"next-hop-address"`
	NextHopInterface string `yaml:"next-hop-interface"`
}

// DNSResolver contains the dns-resolver configuration portion of the nmstate configuration.
type DNSResolver struct {
	Config DNSResolverConfig `yaml:"config"`
}

// DNSResolverConfig defines an nmstate dns-resolver and its properties.
type DNSResolverConfig struct {
	Server []string `yaml:"server"`
}

// IPConfig defines the IP configuration applied to an interface.
type IPConfig struct {
	DHCP     bool        `yaml:"dhcp"`
	Autoconf *bool       `yaml:"autoconf,omitempty"`
	Address  []IPAddress `yaml:"address"`
	Enabled  bool        `yaml:"enabled"`
}

// IPAddress provides the IP address details to an IP configuration.
type IPAddress struct {
	IP           string `yaml:"ip"`
	PrefixLength string `yaml:"prefix-length"`
}
