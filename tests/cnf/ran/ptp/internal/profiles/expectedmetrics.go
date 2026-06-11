package profiles

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
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
//   - process=ptp4l, iface=<raw_interface>: for each slave (client) interface (uses raw names, not NIC names)
//   - process=dpll, iface=<nic>: for each DPLL-monitored interface (RX pins) in GM profiles with Intel plugins
//   - process=gnss, iface=<nic>: for each GNSS interface (TX/GPS) in GM profiles with Intel plugins
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

	// ptp4l: expected for each slave (client) interface. Uses raw interface names because ptp4l metrics
	// report per-port with the actual interface name (e.g., "ens1f0"), not the NIC name (e.g., "ens1fx").
	clientInterfaces := profileInfo.GetInterfacesByClockType(ClockTypeClient)
	for _, ifaceInfo := range clientInterfaces {
		expected = append(expected, metrics.ExpectedClockState{
			Process:   metrics.ProcessPTP4L,
			Interface: string(ifaceInfo.Name),
			Node:      nodeName,
		})
	}

	// DPLL and GNSS: only applicable to GM/MultiNICGM/NTPFallback profiles that have Intel plugins.
	if isGMProfile(profileInfo.ProfileType) {
		dpllExpected, gnssExpected, err := getExpectedForGMPlugins(client, nodeName, profileInfo)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof(
				"Could not determine DPLL/GNSS expected metrics for profile %s on node %s: %v",
				profileInfo.Reference.ProfileName, nodeName, err)
		} else {
			expected = append(expected, dpllExpected...)
			expected = append(expected, gnssExpected...)
		}
	}

	return expected, nil
}

// isGMProfile returns true if the profile type is a grandmaster variant that may have DPLL/GNSS hardware.
func isGMProfile(profileType PtpProfileType) bool {
	switch profileType {
	case ProfileTypeGM, ProfileTypeMultiNICGM, ProfileTypeNTPFallback:
		return true
	default:
		return false
	}
}

// getExpectedForGMPlugins inspects the raw PtpProfile plugins to determine expected DPLL and GNSS metrics.
func getExpectedForGMPlugins(
	client *clients.Settings, nodeName string, profileInfo *ProfileInfo,
) (dpll []metrics.ExpectedClockState, gnss []metrics.ExpectedClockState, err error) {
	rawProfile, err := profileInfo.PullProfile(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to pull raw profile: %w", err)
	}

	// DPLL: check for Intel plugin with DpllSettings and RX interfaces. DPLL metrics use NIC names.
	_, plugin, resolveErr := resolveHoldoverPlugin(rawProfile)
	if resolveErr == nil && plugin != nil && plugin.DpllSettings != nil {
		rxInterfaces, rxErr := GetRxInterfaces(rawProfile)
		if rxErr == nil {
			for _, ifaceName := range rxInterfaces {
				nicName := ifaceName.GetNIC()
				if nicName == "" {
					continue
				}

				dpll = append(dpll, metrics.ExpectedClockState{
					Process:   metrics.ProcessDPLL,
					Interface: string(nicName),
					Node:      nodeName,
				})
			}
		} else {
			klog.V(tsparams.LogLevel).Infof("No RX interfaces for DPLL in profile %s: %v",
				profileInfo.Reference.ProfileName, rxErr)
		}
	}

	// GNSS: check for GM interface to GPS (TX pin or device). GNSS metrics use NIC names.
	gpsInterface, gpsErr := GetGmInterfaceToGPS(rawProfile)
	if gpsErr == nil {
		nicName := gpsInterface.GetNIC()
		if nicName != "" {
			gnss = append(gnss, metrics.ExpectedClockState{
				Process:   metrics.ProcessGNSS,
				Interface: string(nicName),
				Node:      nodeName,
			})
		}
	} else {
		klog.V(tsparams.LogLevel).Infof("No GNSS interface in profile %s: %v",
			profileInfo.Reference.ProfileName, gpsErr)
	}

	return dpll, gnss, nil
}
