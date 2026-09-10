package nad

// TapPlugin returns tap network plugin configuration.
func TapPlugin(owner, group int, multiQueue bool) *Plugin {
	return &Plugin{
		Type:           "tap",
		Owner:          owner,
		Group:          group,
		MultiQueue:     multiQueue,
		SelinuxContext: "system_u:system_r:container_t:s0",
	}
}

// TuningSysctlPlugin returns sysctl plugin configuration.
func TuningSysctlPlugin(macCap bool, sysctlConfig map[string]string) *Plugin {
	return &Plugin{
		Type:         "tuning",
		Capabilities: &Capability{Mac: macCap},
		Sysctl:       sysctlConfig,
	}
}

// TuningMacPlugin returns mac plugin configuration.
func TuningMacPlugin(macCap bool) *Plugin {
	return &Plugin{
		Type:         "tuning",
		Capabilities: &Capability{Mac: macCap},
	}
}

// BondPluginOptions holds bond plugin fields for BondPlugin.
type BondPluginOptions struct {
	FailOverMac      int
	LinksInContainer bool
	Miimon           string
}

// BondPlugin returns bond plugin configuration.
func BondPlugin(ipam *IPAM, bondPorts []string, bondMode string, opts BondPluginOptions) *Plugin {
	if !validBondModes[bondMode] {
		return nil
	}

	links := make([]Link, 0, len(bondPorts))
	for _, port := range bondPorts {
		links = append(links, Link{Name: port})
	}

	return &Plugin{
		Type:             "bond",
		Mode:             bondMode,
		FailOverMac:      opts.FailOverMac,
		LinksInContainer: opts.LinksInContainer,
		Miimon:           opts.Miimon,
		Ipam:             ipam,
		Links:            links,
	}
}
