package profiles

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// GetExpectedClockStates derives the expected openshift_ptp_clock_state metric series from the PtpConfig
// configuration on the cluster. It inspects each node's recommended profiles to determine which (process, interface)
// combinations should be reporting clock state metrics.
//
// The expected metrics are:
//   - process=phc2sys, iface=CLOCK_REALTIME: for each profile that runs phc2sys (all non-TBCTransmitter profiles)
//   - process=ptp4l, iface=<interface>: for each slave (client) interface. Simple OC profiles use raw names
//     (e.g., "ens1f0"); T-BC receiver profiles use NIC names (e.g., "enox") because boundary_clock_jbod mode
//     consolidates the ptp4l clock state to the NIC level.
//   - process=dpll, iface=<nic>: for each DPLL-monitored interface in profiles with DPLL capability
//   - process=gnss, iface=<nic>: for each GNSS interface (TX/GPS) in GM profiles with Intel plugins
//
// DPLL capability is determined by the presence of Intel plugin DpllSettings or a HardwareConfig CR with DPLL
// holdover parameters. For GM profiles, DPLL interfaces are discovered from plugin pins (E810 RX) or devices
// (E825/E830). For other profiles (T-BC, T-TSC, etc.), DPLL interfaces are the client (upstream) interfaces.
//
// The client parameter is used to pull raw PtpProfile specs when plugin inspection is needed for DPLL/GNSS detection.
func GetExpectedClockStates(client *clients.Settings, nodeInfoMap map[string]*NodeInfo) ([]metrics.ExpectedClockState, error) {
	var expected []metrics.ExpectedClockState

	for nodeName, nodeInfo := range nodeInfoMap {
		for _, profileInfo := range nodeInfo.Profiles {
			profileExpected, err := getExpectedForProfile(client, nodeName, profileInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to get expected clock states for profile %s on node %s: %w",
					profileInfo.Reference.ProfileName, nodeName, err)
			}

			expected = append(expected, profileExpected...)
		}
	}

	return expected, nil
}

// getExpectedForProfile determines the expected clock state metrics for a single profile on a node.
func getExpectedForProfile(
	client *clients.Settings, nodeName string, profileInfo *ProfileInfo) ([]metrics.ExpectedClockState, error) {
	var expected []metrics.ExpectedClockState

	// phc2sys / CLOCK_REALTIME: expected for all profiles except TBC transmitters (which only transmit time and
	// do not sync a local clock via phc2sys).
	if profileInfo.ProfileType != ProfileTypeTBCTransmitter {
		expected = append(expected, metrics.ExpectedClockState{
			Process:   metrics.ProcessPHC2SYS,
			Interface: string(iface.ClockRealtime),
			Node:      nodeName,
		})
	}

	// ptp4l: expected for each slave (client) interface. The interface name format depends on the profile
	// type: single-port OC profiles report per-port with raw names (e.g., "ens1f0"), while multi-port
	// profiles (BC, T-BC receiver) consolidate the clock state to the NIC level (e.g., "ens3fx").
	clientInterfaces := profileInfo.GetInterfacesByClockType(ClockTypeClient)
	for _, ifaceInfo := range clientInterfaces {
		ptp4lIface := string(ifaceInfo.Name)

		if ptp4lUsesNICName(profileInfo.ProfileType) {
			if nicName := ifaceInfo.Name.GetNIC(); nicName != "" {
				ptp4lIface = string(nicName)
			}
		}

		expected = append(expected, metrics.ExpectedClockState{
			Process:   metrics.ProcessPTP4L,
			Interface: ptp4lIface,
			Node:      nodeName,
		})
	}

	// Pull the raw profile once for DPLL and GNSS inspection.
	rawProfile, err := profileInfo.PullProfile(client)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Could not pull raw profile %s on node %s for DPLL/GNSS inspection: %v",
			profileInfo.Reference.ProfileName, nodeName, err)

		return expected, nil
	}

	// DPLL: applicable to any profile with DPLL capability (Intel plugin DpllSettings or HardwareConfig
	// with DPLL holdover parameters).
	dpllExpected := getExpectedDpll(nodeName, profileInfo, rawProfile)
	expected = append(expected, dpllExpected...)

	// GNSS: only applicable to GM variant profiles with Intel plugins.
	if isGMProfile(profileInfo.ProfileType) {
		gnssExpected := getExpectedGnss(nodeName, profileInfo, rawProfile)
		expected = append(expected, gnssExpected...)
	}

	return expected, nil
}

// isGMProfile returns true if the profile type is a grandmaster variant that may have GNSS hardware.
func isGMProfile(profileType PtpProfileType) bool {
	switch profileType {
	case ProfileTypeGM, ProfileTypeMultiNICGM, ProfileTypeNTPFallback:
		return true
	default:
		return false
	}
}

// ptp4lUsesNICName returns true if the profile type reports ptp4l clock state metrics with NIC names rather
// than raw interface names. When ptp4l manages multiple ports (clock_type BC, or boundary_clock_jbod mode),
// the PTP operator consolidates the ptp4l clock state to the NIC level (e.g., "ens3fx" instead of "ens3f2").
// Single-port OC profiles report per-port with raw interface names.
func ptp4lUsesNICName(profileType PtpProfileType) bool {
	switch profileType {
	case ProfileTypeBC, ProfileTypeTBCReceiver:
		return true
	default:
		return false
	}
}

// hasDpllCapability reports whether the profile has DPLL monitoring capability, either through an Intel plugin
// with DpllSettings or through a HardwareConfig CR with DPLL holdover parameters.
func hasDpllCapability(profileInfo *ProfileInfo, rawProfile *ptpv1.PtpProfile) bool {
	_, plugin, err := resolveHoldoverPlugin(rawProfile)
	if err == nil && plugin != nil && plugin.DpllSettings != nil {
		return true
	}

	if profileInfo.HardwareConfig != nil {
		_, _, hwErr := firstHoldoverParameters(profileInfo.HardwareConfig)

		return hwErr == nil
	}

	return false
}

// getExpectedDpll determines the expected DPLL clock state metrics for a profile. It is called for all profile
// types and returns nil when the profile has no DPLL capability.
//
// DPLL interface discovery depends on the profile type:
//   - GM variants (T-GM, Multi-NIC GM, NTP Fallback): plugin-based discovery via GetDpllInterfaces
//     (E810 RX pins or E825/E830 Devices).
//   - All other profiles (T-BC, T-TSC, etc.): client (upstream) interfaces from the parsed profile,
//     since the DPLL monitors the clock recovery on the time-receiving NIC.
//
// When plugin-based discovery fails or returns no interfaces, the function falls back to client interfaces.
func getExpectedDpll(
	nodeName string, profileInfo *ProfileInfo, rawProfile *ptpv1.PtpProfile,
) []metrics.ExpectedClockState {
	if !hasDpllCapability(profileInfo, rawProfile) {
		return nil
	}

	dpllIfaces := getDpllInterfaces(profileInfo, rawProfile)
	if len(dpllIfaces) == 0 {
		return nil
	}

	var expected []metrics.ExpectedClockState

	for _, ifaceName := range dpllIfaces {
		nicName := ifaceName.GetNIC()
		if nicName == "" {
			continue
		}

		expected = append(expected, metrics.ExpectedClockState{
			Process:   metrics.ProcessDPLL,
			Interface: string(nicName),
			Node:      nodeName,
		})
	}

	return expected
}

// getDpllInterfaces returns the interfaces that should have DPLL clock state metrics for the given profile.
// For GM profiles, it tries plugin-based discovery first (E810 RX pins, E825/E830 Devices). For non-GM profiles
// or when plugin discovery fails, it falls back to the profile's client interfaces.
func getDpllInterfaces(profileInfo *ProfileInfo, rawProfile *ptpv1.PtpProfile) []iface.Name {
	pluginIfaces, pluginErr := GetDpllInterfaces(rawProfile)
	if pluginErr == nil && len(pluginIfaces) > 0 {
		return pluginIfaces
	}

	if pluginErr != nil {
		klog.V(tsparams.LogLevel).Infof("Plugin-based DPLL interface discovery failed for profile %s: %v",
			profileInfo.Reference.ProfileName, pluginErr)
	}

	clientInterfaces := profileInfo.GetInterfacesByClockType(ClockTypeClient)

	ifaceNames := make([]iface.Name, 0, len(clientInterfaces))
	for _, ifaceInfo := range clientInterfaces {
		ifaceNames = append(ifaceNames, ifaceInfo.Name)
	}

	return ifaceNames
}

// getExpectedGnss determines the expected GNSS clock state metrics for a GM profile. It inspects the raw PtpProfile
// plugins to find the GPS interface (TX pin for E810 or device for E825). Only GM variant profiles have GPS
// connections, so this function should only be called when isGMProfile returns true.
func getExpectedGnss(
	nodeName string, profileInfo *ProfileInfo, rawProfile *ptpv1.PtpProfile,
) []metrics.ExpectedClockState {
	gpsInterface, gpsErr := GetGmInterfaceToGPS(rawProfile)
	if gpsErr != nil {
		klog.V(tsparams.LogLevel).Infof("No GNSS interface in profile %s: %v",
			profileInfo.Reference.ProfileName, gpsErr)

		return nil
	}

	nicName := gpsInterface.GetNIC()
	if nicName == "" {
		return nil
	}

	return []metrics.ExpectedClockState{
		{
			Process:   metrics.ProcessGNSS,
			Interface: string(nicName),
			Node:      nodeName,
		},
	}
}
