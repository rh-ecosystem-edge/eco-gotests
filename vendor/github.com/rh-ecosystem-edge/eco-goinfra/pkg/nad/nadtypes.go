package nad

type (
	// Capability tells if the plugin supports MAC.
	Capability struct {
		Mac bool `json:"mac,omitempty"`
		IPs bool `json:"ips,omitempty"`
	}

	// Link contains the link name of a link.
	Link struct {
		Name string `json:"name,omitempty"`
	}

	// Plugin contains all plugin details and information of a single plugin.
	Plugin struct {
		LinksInContainer bool              `json:"linksInContainer,omitempty"`
		IPMasq           bool              `json:"ipMasq,omitempty"`
		IsGateway        bool              `json:"isGateway,omitempty"`
		IsDefaultGateway bool              `json:"isDefaultGateway,omitempty"`
		ForceAddress     bool              `json:"forceAddress,omitempty"`
		HairpinMode      bool              `json:"hairpinMode,omitempty"`
		PromiscuousMode  bool              `json:"promiscuousMode,omitempty"`
		FailOverMac      int               `json:"failOverMac,omitempty"`
		CNIVersion       string            `json:"cniVersion,omitempty"`
		Name             string            `json:"name,omitempty"`
		Type             string            `json:"type,omitempty"`
		Bridge           string            `json:"bridge,omitempty"`
		Master           string            `json:"master,omitempty"`
		Vlan             string            `json:"vlan,omitempty"`
		Mtu              string            `json:"mtu,omitempty"`
		VrfName          string            `json:"vrfName,omitempty"`
		Mode             string            `json:"mode,omitempty"`
		Miimon           string            `json:"miimon,omitempty"`
		Capabilities     *Capability       `json:"capabilities,omitempty"`
		Sysctl           map[string]string `json:"sysctl,omitempty"`
		Links            []Link            `json:"links,omitempty"`
		Ipam             *IPAM             `json:"ipam,omitempty"`
		Owner            int               `json:"owner,omitempty"`
		Group            int               `json:"group,omitempty"`
		MultiQueue       bool              `json:"multiQueue,omitempty"`
		SelinuxContext   string            `json:"selinuxcontext,omitempty"`
	}

	// MasterPlugin contains the master plugin configuration for a NAD.
	MasterPlugin struct {
		CniVersion       string      `json:"cniVersion,omitempty"`
		Name             string      `json:"name,omitempty"`
		Type             string      `json:"type,omitempty"`
		Master           string      `json:"master,omitempty"`
		Mode             string      `json:"mode,omitempty"`
		Plugins          *[]Plugin   `json:"plugins,omitempty"`
		Bridge           string      `json:"bridge,omitempty"`
		Device           string      `json:"device,omitempty"`
		Ipam             *IPAM       `json:"ipam,omitempty"`
		LinksInContainer bool        `json:"linksInContainer,omitempty"`
		LinkInContainer  bool        `json:"linkInContainer,omitempty"`
		VlanID           uint16      `json:"vlanId,omitempty"`
		FailOverMac      int         `json:"failOverMac,omitempty"`
		Miimon           string      `json:"miimon,omitempty"`
		Mtu              int         `json:"mtu,omitempty"`
		Links            []Link      `json:"links,omitempty"`
		Capabilities     *Capability `json:"capabilities,omitempty"`
	}

	// IPRanges contains ip range for WhereAbout IPAM plugin.
	IPRanges struct {
		Range   string `json:"range,omitempty"`
		Gateway string `json:"gateway,omitempty"`
	}

	// Routes represent routing entries for IPAM plugin.
	Routes struct {
		Dst string `json:"dst,omitempty"`
		Gw  string `json:"gw,omitempty"`
	}

	// IPAM container the IPAM configuration for a NAD.
	IPAM struct {
		Type       string     `json:"type,omitempty"`
		AddrRange  string     `json:"range,omitempty"`
		RangeStart string     `json:"range_start,omitempty"`
		RangeEnd   string     `json:"range_end,omitempty"`
		Gateway    string     `json:"gateway,omitempty"`
		Exclude    []string   `json:"exclude,omitempty"`
		Routes     []Routes   `json:"routes,omitempty"`
		IPRanges   []IPRanges `json:"ipRanges,omitempty"`
	}
)
