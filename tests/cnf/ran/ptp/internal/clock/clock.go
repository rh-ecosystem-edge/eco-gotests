// Package clock discovers logical PTP clocks once for the whole suite run and provides capability-aware
// assertions against them. It is built on top of the profiles package's discovery primitives, so tests can
// assert against a clock's expected state without re-discovering the cluster's topology or re-deriving its
// holdover capability in every test case.
package clock

import (
	"context"
	"fmt"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// ClockType identifies a clock's role per the ITU-T G.8275.1 Telecom Profile (T-GM, T-BC, T-TSC: role-fixed
// specializations of IEEE 1588's GM, BC, and OC base clock types), independent of the profiles package's
// CR-shaped ProfileType. Today only the one type AssertLocked needs to dispatch correctly is modeled.
type ClockType string

// ClockTypeTBC is the type shared by T-BC and T-TSC clocks: both derive their time from an upstream PTP
// node (IEEE 1588 timeSource PTP, not GPS/GNSS), and cloud-event-proxy tags their overall sync state
// identically as a result (process "T-BC").
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
	// a HardwareConfig CR. The kernel DPLL netlink UAPI defines LOCKED_HO_ACQ as a state only a
	// holdover-capable clock can reach, so this flag decides which stability-assertion strategy applies (see
	// AssertLocked).
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
		// ConfigIndex lets a caller scope a clock_class query to this profile's own ptp4l instance instead of
		// every ptp4l process on the node. Some profiles have no ptp4l process at all (HA followers running
		// only phc2sys), so a failure here is expected on those nodes and does not block discovery.
		if err := nodeInfo.SetConfigIndices(client); err != nil {
			klog.V(tsparams.LogLevel).Infof("Skipping config index resolution on node %s: %v", name, err)
		}

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

// stabilityAssertion verifies a clock has reached a stable LOCKED state, at the depth its capability
// requires. A holdover-capable clock's DPLL must also reach LOCKED_HO_ACQ before a holdover transition is
// safe to force against it. A clock with no holdover path is verified with the coarser clock-state metric
// alone.
type stabilityAssertion interface {
	assertLocked(
		ctx context.Context, prometheusAPI prometheusv1.API, target *Clock, opts ...metrics.QueryAssertOption,
	) error
}

// clockStateQuery returns the openshift_ptp_clock_state query that carries this clock's own overall sync
// state. cloud-event-proxy tags this metric per role, not uniformly:
//   - ClockTypeTBC (T-BC and T-TSC): process "T-BC".
//   - Everything else (plain OC, a non-T-BC boundary clock): no dedicated tag. cloud-event-proxy
//     publishes phc2sys's own CLOCK_REALTIME sample as this clock's os-clock-sync-state (LOCKED/
//     HOLDOVER/FREERUN, Table 37 of "O-RAN O-Cloud Notification API Specification for Event
//     Consumers" v04.00: https://specifications.o-ran.org/download?id=670), and these profiles
//     combine no other state into it, so that sample alone is the clock's overall LOCKED signal.
func (clock *Clock) clockStateQuery() metrics.ClockStateQuery {
	node := metrics.Equals(clock.NodeName)

	if clock.Type == ClockTypeTBC {
		return metrics.ClockStateQuery{Node: node, Process: metrics.Equals(metrics.ProcessTBC)}
	}

	return metrics.ClockStateQuery{
		Node:      node,
		Process:   metrics.Equals(metrics.ProcessPHC2SYS),
		Interface: metrics.Equals(iface.ClockRealtime),
	}
}

// basicLockedAssertion verifies the daemon's own clock-state metric reports LOCKED. It applies to a clock
// with no holdover path.
type basicLockedAssertion struct{}

func (basicLockedAssertion) assertLocked(
	ctx context.Context, prometheusAPI prometheusv1.API, target *Clock, opts ...metrics.QueryAssertOption,
) error {
	query := target.clockStateQuery()

	if err := metrics.AssertQuery(ctx, prometheusAPI, query, metrics.ClockStateLocked, opts...); err != nil {
		return fmt.Errorf("failed to assert clock state LOCKED: %w", err)
	}

	return nil
}

// holdoverCapableLockedAssertion additionally requires the DPLL phase-status metric to report
// LOCKED_HO_ACQ, confirming the clock can enter holdover before a holdover test starts.
type holdoverCapableLockedAssertion struct{}

func (holdoverCapableLockedAssertion) assertLocked(
	ctx context.Context, prometheusAPI prometheusv1.API, target *Clock, opts ...metrics.QueryAssertOption,
) error {
	if err := (basicLockedAssertion{}).assertLocked(ctx, prometheusAPI, target, opts...); err != nil {
		return err
	}

	query := metrics.PhaseStatusQuery{
		Node:    metrics.Equals(target.NodeName),
		Process: metrics.Equals(metrics.ProcessDPLL),
	}

	err := metrics.AssertQuery(ctx, prometheusAPI, query, metrics.PhaseStatusLockedHoldoverAcquired, opts...)
	if err != nil {
		return fmt.Errorf("failed to assert phase status LOCKED_HO_ACQ: %w", err)
	}

	return nil
}

// strategyFor selects the stability-assertion strategy appropriate to the clock's discovered capability.
func (clock *Clock) strategyFor() stabilityAssertion {
	if clock.HoldoverCapable {
		return holdoverCapableLockedAssertion{}
	}

	return basicLockedAssertion{}
}

// AssertLocked verifies the clock has reached a stable LOCKED state, dispatching to the assertion strategy
// appropriate for its discovered capability (see HoldoverCapable).
func (clock *Clock) AssertLocked(
	ctx context.Context, prometheusAPI prometheusv1.API, opts ...metrics.QueryAssertOption,
) error {
	return clock.strategyFor().assertLocked(ctx, prometheusAPI, clock, opts...)
}
