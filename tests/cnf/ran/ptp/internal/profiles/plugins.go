package profiles

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// PinStateType enumerates the supported pin states.
type PinStateType int

const (
	// PinStateDisabled is the disabled state: 0.
	PinStateDisabled PinStateType = iota
	// PinStateRx is the RX state: 1.
	PinStateRx
	// PinStateTx is the TX state: 2.
	PinStateTx
)

// GetGmInterfaceToGPS returns the TX interface for the grand master profile to GPS. Returns an error if the profile
// is nil, has no plugins, or the plugin is missing or invalid.
// When both E810 and E825 plugins are present, E810 is used (pins / TX state);
// E825 is only considered if E810 is absent.
func GetGmInterfaceToGPS(profile *ptpv1.PtpProfile) (iface.Name, error) {
	if profile == nil {
		return "", fmt.Errorf("profile is nil")
	}

	if profile.Plugins == nil {
		return "", fmt.Errorf("profile has no plugins")
	}

	var txInterfaces []iface.Name

	var err error

	if _, ok := profile.Plugins[string(ptp.PluginTypeE810)]; ok {
		txInterfaces, err = getInterfacesWithPluginPins(profile, ptp.PluginTypeE810, PinStateTx)
		if err != nil {
			return "", fmt.Errorf("failed to get interfaces for E810 plugin: %w", err)
		}
	} else if _, ok := profile.Plugins[string(ptp.PluginTypeE825)]; ok {
		txInterfaces, err = getInterfacesWithDevices(profile, ptp.PluginTypeE825)
		if err != nil {
			return "", fmt.Errorf("failed to get interfaces for E825 plugin: %w", err)
		}
	}

	if len(txInterfaces) == 0 {
		return "", fmt.Errorf("no GM interface to GPS found in profile")
	}

	if len(txInterfaces) != 1 {
		return "", fmt.Errorf("expected 1 GM interface to GPS, got %d", len(txInterfaces))
	}

	return txInterfaces[0], nil
}

// getIntelPlugin returns the unmarshaled Intel plugin for the given profile and plugin type. Returns an error if the
// profile is nil, has no plugins, or the plugin is missing or invalid.
func getIntelPlugin(profile *ptpv1.PtpProfile, pluginType ptp.PluginType) (*ptp.IntelPlugin, error) {
	if profile == nil || profile.Plugins == nil {
		return nil, fmt.Errorf("profile is nil or has no plugins")
	}

	pluginJSON, ok := profile.Plugins[string(pluginType)]
	if !ok || pluginJSON == nil || len(pluginJSON.Raw) == 0 {
		return nil, fmt.Errorf("%s plugin not found in profile", pluginType)
	}

	var intelPlugin ptp.IntelPlugin
	if err := json.Unmarshal(pluginJSON.Raw, &intelPlugin); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s plugin: %w", pluginType, err)
	}

	return &intelPlugin, nil
}

// getInterfacesWithDevices returns devices names that are configured for the given plugin type.
// Devices is a list of device names for E825/E830 plugins.
// Returns an error if the profile is nil, has no plugins, or the plugin is missing or invalid.
func getInterfacesWithDevices(profile *ptpv1.PtpProfile,
	pluginType ptp.PluginType) ([]iface.Name, error) {
	intelPlugin, err := getIntelPlugin(profile, pluginType)
	if err != nil {
		return nil, err
	}

	if intelPlugin.Devices == nil {
		return nil, fmt.Errorf("%s plugin has no devices", pluginType)
	}

	var interfaceNames []iface.Name

	for _, device := range intelPlugin.Devices {
		interfaceNames = append(interfaceNames, iface.Name(device))
	}

	return interfaceNames, nil
}

// getInterfacesWithPluginPins returns interface names that have at least one pin
// configured as the specified pin state in the profile's plugin pins. Returns error if the
// profile has no plugin or no pins or an error occurs. Pins use "pin-state channel" syntax;
// pinState is the pin state to look for.
func getInterfacesWithPluginPins(profile *ptpv1.PtpProfile,
	pluginType ptp.PluginType,
	pinState PinStateType) ([]iface.Name, error) {
	intelPlugin, err := getIntelPlugin(profile, pluginType)
	if err != nil {
		return nil, err
	}

	if intelPlugin.Pins == nil {
		return nil, fmt.Errorf("%s plugin has no pins", pluginType)
	}

	var interfaceNames []iface.Name

	for ifaceName, connectorToValue := range intelPlugin.Pins {
		for _, value := range connectorToValue {
			first := strings.Fields(value)
			if len(first) >= 1 && first[0] == fmt.Sprintf("%d", pinState) {
				interfaceNames = append(interfaceNames, iface.Name(ifaceName))

				break
			}
		}
	}

	return interfaceNames, nil
}

// GetPluginTypesFromProfile returns the plugin types from the profile. Returns an error if the profile has no plugins.
func GetPluginTypesFromProfile(profile *ptpv1.PtpProfile) ([]ptp.PluginType, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile is nil")
	}

	if profile.Plugins == nil {
		return nil, fmt.Errorf("profile has no plugins")
	}

	pluginTypes := make([]ptp.PluginType, 0, len(profile.Plugins))
	for pluginType := range profile.Plugins {
		pluginTypes = append(pluginTypes, ptp.PluginType(pluginType))
	}

	return pluginTypes, nil
}

// HoldoverPluginSettings groups the PTP plugin settings that control holdover behavior.
type HoldoverPluginSettings struct {
	LocalHoldoverTimeout   uint
	LocalMaxHoldoverOffSet uint
	MaxInSpecOffset        uint
}

// GetHoldoverPluginSettings reads the holdover plugin settings from the E810 plugin in the profile.
// Uses the typed IntelPlugin struct for safe deserialization.
func GetHoldoverPluginSettings(profile *ptpv1.PtpProfile) (*HoldoverPluginSettings, error) {
	intelPlugin, err := getIntelPlugin(profile, ptp.PluginTypeE810)
	if err != nil {
		return nil, fmt.Errorf("failed to get Intel plugin for holdover settings: %w", err)
	}

	if intelPlugin.DpllSettings == nil {
		return nil, fmt.Errorf("E810 plugin has no settings")
	}

	localHoldoverTimeout, timeoutFound := intelPlugin.DpllSettings["LocalHoldoverTimeout"]
	if !timeoutFound {
		return nil, fmt.Errorf("'LocalHoldoverTimeout' not found in plugin settings")
	}

	localMaxHoldoverOffset, offsetFound := intelPlugin.DpllSettings["LocalMaxHoldoverOffSet"]
	if !offsetFound {
		return nil, fmt.Errorf("'LocalMaxHoldoverOffSet' not found in plugin settings")
	}

	maxInSpecOffset, specFound := intelPlugin.DpllSettings["MaxInSpecOffset"]
	if !specFound {
		return nil, fmt.Errorf("'MaxInSpecOffset' not found in plugin settings")
	}

	return &HoldoverPluginSettings{
		LocalHoldoverTimeout:   uint(localHoldoverTimeout),
		LocalMaxHoldoverOffSet: uint(localMaxHoldoverOffset),
		MaxInSpecOffset:        uint(maxInSpecOffset),
	}, nil
}

// SetHoldoverPluginSettings patches the holdover plugin settings on the E810 plugin in the profile.
func SetHoldoverPluginSettings(profile *ptpv1.PtpProfile, settings HoldoverPluginSettings) error {
	intelPlugin, err := getIntelPlugin(profile, ptp.PluginTypeE810)
	if err != nil {
		return fmt.Errorf("failed to get Intel plugin for holdover settings: %w", err)
	}

	if intelPlugin.DpllSettings == nil {
		intelPlugin.DpllSettings = make(map[string]uint64)
	}

	intelPlugin.DpllSettings["LocalHoldoverTimeout"] = uint64(settings.LocalHoldoverTimeout)
	intelPlugin.DpllSettings["LocalMaxHoldoverOffSet"] = uint64(settings.LocalMaxHoldoverOffSet)
	intelPlugin.DpllSettings["MaxInSpecOffset"] = uint64(settings.MaxInSpecOffset)

	raw, err := json.Marshal(intelPlugin)
	if err != nil {
		return fmt.Errorf("failed to marshal E810 plugin: %w", err)
	}

	profile.Plugins[string(ptp.PluginTypeE810)] = &apiextv1.JSON{Raw: raw}

	return nil
}

// GetUpstreamPort returns the upstream port from the E810 plugin's interconnections. Returns the first
// interconnection entry that has an upstreamPort set.
func GetUpstreamPort(profile *ptpv1.PtpProfile) (iface.Name, error) {
	intelPlugin, err := getIntelPlugin(profile, ptp.PluginTypeE810)
	if err != nil {
		return "", fmt.Errorf("failed to get Intel plugin for upstream port: %w", err)
	}

	for _, interconnection := range intelPlugin.InputDelays {
		if interconnection.UpstreamPort != "" {
			return iface.Name(interconnection.UpstreamPort), nil
		}
	}

	return "", fmt.Errorf("no upstream port found in E810 plugin interconnections")
}

// GetRxInterfaces returns the interfaces configured as RX (pin state 1) in the E810 plugin.
func GetRxInterfaces(profile *ptpv1.PtpProfile) ([]iface.Name, error) {
	return getInterfacesWithPluginPins(profile, ptp.PluginTypeE810, PinStateRx)
}

// GetSmaPinFromProfile returns the first active SMA pin name and its configuration string for
// ifaceName from the E810 plugin. A pin is active when its state value (the first field in the
// "pin-state channel" string) is not "0". Pin names are checked in sorted order so the result
// is deterministic (e.g. SMA1 before SMA2).
func GetSmaPinFromProfile(profile *ptpv1.PtpProfile, ifaceName iface.Name) (string, string, error) {
	intelPlugin, err := getIntelPlugin(profile, ptp.PluginTypeE810)
	if err != nil {
		return "", "", fmt.Errorf("failed to get E810 plugin: %w", err)
	}

	ifacePins, ok := intelPlugin.Pins[string(ifaceName)]
	if !ok {
		return "", "", fmt.Errorf("interface %s not found in E810 plugin pins", ifaceName)
	}

	pinNames := make([]string, 0, len(ifacePins))
	for p := range ifacePins {
		pinNames = append(pinNames, p)
	}

	slices.Sort(pinNames)

	for _, name := range pinNames {
		config := ifacePins[name]
		fields := strings.Fields(config)

		if len(fields) > 0 && fields[0] != "0" {
			return name, config, nil
		}
	}

	return "", "", fmt.Errorf("no active SMA pin found for interface %s", ifaceName)
}
