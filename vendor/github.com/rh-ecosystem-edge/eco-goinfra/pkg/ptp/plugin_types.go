package ptp

// Types for Intel PTP plugins are based on the [linuxptp-daemon repo] and are licensed under the Apache License 2.0.
// [linuxptp-daemon repo]: https://github.com/openshift/linuxptp-daemon/blob/main/addons/intel/

// PluginType represents the Intel PTP plugin type.
type PluginType string

const (
	// PluginTypeE810 represents the plugin for Intel E810 NICs.
	PluginTypeE810 PluginType = "e810"
	// PluginTypeE825 represents the plugin for Intel E825 NICs.
	PluginTypeE825 PluginType = "e825"
	// PluginTypeE830 represents the plugin for Intel E830 NICs.
	PluginTypeE830 PluginType = "e830"
)

// IntelPlugin is a unified struct that can unmarshal any Intel PTP plugin (E810, E825, E830).
// The Type field identifies which plugin type this represents and is not serialized - it is
// derived from the plugin key in the PtpProfile's Plugins map.
type IntelPlugin struct {
	// Type identifies which plugin type this is (e810, e825, e830).
	// Not serialized - derived from the plugin key in the Plugins map.
	Type PluginType `json:"-"`

	// Common fields across all Intel plugins

	// UblxCmds is a list of arguments passed to the ubxtool command.
	UblxCmds []UblxCmd `json:"ublxCmds,omitempty"`

	// Pins is a map of interfaces to pins and their corresponding values. The outer map key is the interface name,
	// and the inner map key is the pin name. For E810, note that the inner string values must be of the form "%d %d"
	// where the first %d is the pin state and the second %d is the pin channel.
	//
	// Pin states are either 0 for disabled, 1 for rx, or 2 for tx. Pin channels match the pin names, where SMA1 and
	// U.FL1 use channel 1; and SMA2 and U.FL2 use channel 2.
	Pins map[string]map[string]string `json:"pins,omitempty"`

	// DpllSettings contains DPLL settings as key-value pairs.
	DpllSettings map[string]uint64 `json:"settings,omitempty"`

	// PhaseOffsetPins uses the interface name as the key to the outer map.
	PhaseOffsetPins map[string]map[string]string `json:"phaseOffsetPins,omitempty"`

	// E810-specific fields

	// EnableDefaultConfig enables the default E810 configuration.
	EnableDefaultConfig bool `json:"enableDefaultConfig,omitempty"`

	// InputDelays contains configurations for input phase delays (E810 interconnections).
	InputDelays []InputPhaseDelays `json:"interconnections,omitempty"`

	// E825/E830-specific fields

	// Devices is a list of device names for E825/E830 plugins.
	Devices []string `json:"devices,omitempty"`

	// DeviceFrequencies contains frequency settings per device for E825/E830 plugins.
	DeviceFrequencies map[string]map[string]uint64 `json:"frequencies,omitempty"`

	// E825-specific fields

	// Gnss contains GNSS-specific options for E825.
	Gnss *GnssOptions `json:"gnss,omitempty"`
}

// UblxCmd contains the arguments for a ubxtool command.
type UblxCmd struct {
	Args         []string `json:"args"`
	ReportOutput bool     `json:"reportOutput"`
}

// InputPhaseDelays contains configurations for input phase delays.
type InputPhaseDelays struct {
	ID                    string      `json:"id"`
	Part                  string      `json:"Part"`
	Input                 *InputDelay `json:"inputPhaseDelay"`
	GnssInput             bool        `json:"gnssInput"`
	PhaseOutputConnectors []string    `json:"phaseOutputConnectors"`
	UpstreamPort          string      `json:"upstreamPort"`
}

// InputDelay contains the connector and delay in picoseconds.
type InputDelay struct {
	Connector string `json:"connector"`
	DelayPs   int    `json:"delayPs"`
}

// GnssOptions defines GNSS-specific options for the E825 plugin.
type GnssOptions struct {
	Disabled bool `json:"disabled"`
}

// InterfacePin is a string type that represents common pin names for Intel plugins. For E810, valid values are SMA1,
// SMA2, U.FL1, or U.FL2.
type InterfacePin string

const (
	// InterfacePinSMA1 represents the SMA1 pin for Intel E810 NICs.
	InterfacePinSMA1 InterfacePin = "SMA1"
	// InterfacePinSMA2 represents the SMA2 pin for Intel E810 NICs.
	InterfacePinSMA2 InterfacePin = "SMA2"
	// InterfacePinUFL1 represents the U.FL1 pin for Intel E810 NICs.
	InterfacePinUFL1 InterfacePin = "U.FL1"
	// InterfacePinUFL2 represents the U.FL2 pin for Intel E810 NICs.
	InterfacePinUFL2 InterfacePin = "U.FL2"
)
