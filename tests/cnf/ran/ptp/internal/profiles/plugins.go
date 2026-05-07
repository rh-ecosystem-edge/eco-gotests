package profiles

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
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
