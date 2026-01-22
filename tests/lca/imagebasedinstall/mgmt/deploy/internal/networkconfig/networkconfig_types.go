package networkconfig

// NetworkConfig is the top-level struct containing
// a full nmstate network configuration.
type NetworkConfig struct {
	Interfaces  []Interface `yaml:"interfaces"`
	Routes      Routes      `yaml:"routes"`
	DNSResolver DNSResolver `yaml:"dns-resolver"`
}

// Interface defines an nmstate interface and its properties.
type Interface struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	State      string   `yaml:"state"`
	Identifier string   `yaml:"identifier"`
	MACAddress string   `yaml:"mac-address"`
	IPv4       IPConfig `yaml:"ipv4"`
	IPv6       IPConfig `yaml:"ipv6"`
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
