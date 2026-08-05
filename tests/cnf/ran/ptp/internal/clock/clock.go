// Package clock discovers logical PTP clocks once for the whole suite run, so tests can look up a clock's
// identity and capability without re-discovering the cluster's topology in every test case.
package clock

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// ClockType identifies a clock's role per the ITU-T G.8275.1 Telecom Profile (T-GM, T-BC, T-TSC: role-fixed
// specializations of IEEE 1588's GM, BC, and OC base clock types), independent of the profiles package's
// CR-shaped ProfileType. Today only the one type ByType's callers need is modeled.
type ClockType string

// ClockTypeTBC is the type shared by T-BC and T-TSC clocks: both derive their time from an upstream PTP
// node (IEEE 1588 timeSource PTP, not GPS/GNSS).
const ClockTypeTBC ClockType = "T-BC"

// Clock is a logical PTP clock discovered once for the whole suite run, carrying enough identity and
// capability information for tests to assert against it without re-discovering the cluster's topology in
// every test case.
type Clock struct {
	NodeName      string
	ProfileInfo   *profiles.ProfileInfo
	UpstreamIface iface.Name
	Type          ClockType
	// HoldoverCapable is true when the clock's profile has a working holdover path: an Intel E810 plugin or
	// a HardwareConfig CR.
	HoldoverCapable bool
}

// clockTypeFor returns the ClockType a profile's own clock has. Only T-BC and T-TSC are mapped today: they
// are the only profile types FirstHoldoverCapable is ever called with, so mapping anything else would be
// unused speculation. Extend this when a real scenario needs another type, matching this package's own
// discover-once, capability-aware design.
func clockTypeFor(profileType profiles.PtpProfileType) ClockType {
	//nolint:exhaustive // only T-BC and T-TSC are ever passed to FirstHoldoverCapable today.
	switch profileType {
	case profiles.ProfileTypeTBCReceiver, profiles.ProfileTypeTBCTransmitter, profiles.ProfileTypeTTSC:
		return ClockTypeTBC
	default:
		return ""
	}
}

// discovered caches the result of the most recent Discover call, keyed by profile type, so the package-level
// accessors don't need to re-scan the cluster on every lookup. Populated once from BeforeSuite.
var discovered map[profiles.PtpProfileType][]*Clock

// Discover scans every node in the cluster once for every profile it runs, covering multiple nodes and
// multiple clocks per node, and caches the result for ByType and FirstHoldoverCapable to use for the rest
// of the suite run.
//
// Call once, from BeforeSuite, matching the one-time-init pattern iface.InitNICNaming already uses.
func Discover(client *clients.Settings) error {
	nodeInfoMap, err := profiles.GetNodeInfoMap(client)
	if err != nil {
		return fmt.Errorf("failed to get node info map: %w", err)
	}

	found := make(map[profiles.PtpProfileType][]*Clock)

	for name, nodeInfo := range nodeInfoMap {
		for _, profileInfo := range nodeInfo.Profiles {
			discoveredClock, ok, err := discoverClock(client, name, profileInfo)
			if err != nil {
				return err
			}

			if !ok {
				continue
			}

			found[profileInfo.ProfileType] = append(found[profileInfo.ProfileType], discoveredClock)
		}
	}

	discovered = found

	return nil
}

// discoverClock pulls the live profile for one node and builds its Clock. ok is false when the profile is
// not a modeled ClockType (skipped before any live call, so an unrelated profile's own discovery failure
// can never fail the whole suite) or has no discoverable upstream port; both are expected on some profiles,
// and discovery continues for the rest of the cluster either way.
func discoverClock(
	client *clients.Settings, nodeName string, profileInfo *profiles.ProfileInfo,
) (discoveredClock *Clock, ok bool, err error) {
	clockType := clockTypeFor(profileInfo.ProfileType)
	if clockType == "" {
		return nil, false, nil
	}

	ptpProfile, err := profileInfo.PullProfile(client)
	if err != nil {
		return nil, false, fmt.Errorf(
			"failed to pull profile %s on node %s: %w", profileInfo.Reference.ProfileName, nodeName, err)
	}

	port, err := profiles.GetUpstreamPortForProfile(ptpProfile)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Skipping profile %s on node %s: cannot determine upstream port: %v",
			profileInfo.Reference.ProfileName, nodeName, err)

		return nil, false, nil
	}

	return &Clock{
		NodeName:      nodeName,
		ProfileInfo:   profileInfo,
		UpstreamIface: port,
		Type:          clockType,
		HoldoverCapable: profileInfo.HardwareConfig != nil ||
			profiles.HasPlugin(ptpProfile, ptp.PluginTypeE810),
	}, true, nil
}

// ByType returns every discovered clock of the given profile type. Discover must run first; an empty result
// before that has run is indistinguishable from a cluster with no matching clocks.
func ByType(profileType profiles.PtpProfileType) []*Clock {
	return discovered[profileType]
}

// FirstHoldoverCapable returns the first discovered clock of the given profile type that has a working
// holdover path, or false if none was found.
func FirstHoldoverCapable(profileType profiles.PtpProfileType) (*Clock, bool) {
	for _, candidate := range discovered[profileType] {
		if candidate.HoldoverCapable {
			return candidate, true
		}
	}

	return nil, false
}
