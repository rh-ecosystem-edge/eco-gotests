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
