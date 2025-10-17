package ptp

// Types for the E810 plugin are based on the [linuxptp-daemon repo] and are licensed under the Apache License 2.0.
// [linuxptp-daemon repo]: https://github.com/openshift/linuxptp-daemon/blob/main/addons/intel/e810.go

// E810Plugin is a struct that can unmarshal the e810 plugin section of a PtpProfile.
type E810Plugin struct {
	EnableDefaultConfig bool         `json:"enableDefaultConfig"`
	Settings            E810Settings `json:"settings"`

	// Pins is a map of interfaces to pins and their corresponding values. The outer map key is the interface name,
	// and the inner map key is the pin name. Note that the inner string values must be of the form "%d %d" where
	// the first %d is the pin state and the second %d is the pin channel.
	//
	// Pin states are either 0 for disabled, 1 for rx, or 2 for tx. Pin channels match the pin names, where SMA1 and
	// U.FL1 use channel 1; and SMA2 and U.FL2 use channel 2.
	Pins map[string]map[E810InterfacePin]string `json:"pins"`

	// UblxCmds is a list of arguments passed to the ubxtool command.
	UblxCmds []E810UblxCmd `json:"ublxCmds"`
	// PhaseOffsetPins uses the interface name as the key to the outer map.
	PhaseOffsetPins map[string]map[string]string `json:"phaseOffsetPins"`
	InputDelays     []E810InputPhaseDelays       `json:"interconnections"`
}

// E810Settings is a struct that contains the DPLL settings for the E810 plugin.
type E810Settings struct {
	LocalHoldoverTimeout   int `json:"LocalHoldoverTimeout"`
	LocalMaxHoldoverOffSet int `json:"LocalMaxHoldoverOffSet"`
	MaxInSpecOffset        int `json:"MaxInSpecOffset"`
}

// E810InterfacePin is a string type that represents the pin names for the E810 plugin. It is either SMA1, SMA2, U.FL1,
// or U.FL2.
type E810InterfacePin string

//nolint:revive // Pin names are explained by feature documentation and do not need specific comments.
const (
	E810InterfacePinSMA1 E810InterfacePin = "SMA1"
	E810InterfacePinSMA2 E810InterfacePin = "SMA2"
	E810InterfacePinUFL1 E810InterfacePin = "U.FL1"
	E810InterfacePinUFL2 E810InterfacePin = "U.FL2"
)

// E810UblxCmd contains the arguments for a ubxtool command.
type E810UblxCmd struct {
	Args         []string `json:"args"`
	ReportOutput bool     `json:"reportOutput"`
}

// E810InputPhaseDelays contains configurations for input phase delays.
type E810InputPhaseDelays struct {
	ID                    string          `json:"id"`
	Part                  string          `json:"Part"`
	Input                 *E810InputDelay `json:"inputPhaseDelay"`
	GnssInput             bool            `json:"gnssInput"`
	PhaseOutputConnectors []string        `json:"phaseOutputConnectors"`
	UpstreamPort          string          `json:"upstreamPort"`
}

// E810InputDelay contains the connector and delay in picoseconds.
type E810InputDelay struct {
	Connector string `json:"connector"`
	DelayPs   int    `json:"delayPs"`
}
