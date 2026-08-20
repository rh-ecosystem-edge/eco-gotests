package clock

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/resource"
)

// ClockType is a PTP clock type.
type ClockType string

// ClockType values.
const (
	ClockTypeTBC  ClockType = "T-BC"
	ClockTypeTTSC ClockType = "T-TSC"
)

// PtpOperatorVersion is the installed PTP Operator's own version, not the OCP cluster version -- the two can
// diverge.
type PtpOperatorVersion string

// Clock is a PTP clock.
type Clock struct {
	// PTP Operator
	PtpOperatorVersion string
	// PTP Linux
	Type ClockType
	// Linux Infra
	NodeName string
	// NIC Hardware
	Interfaces      map[iface.Name]*Interface
	HoldoverEnabled bool
	HoldoverSource  HoldoverSource
	// ReceivingProfileRef locates the PtpConfig profile this clock's own receiver interfaces came from.
	ReceivingProfileRef resource.ProfileReference
	// HoldoverHardwareConfigRef locates the HardwareConfig CR backing this clock's own holdover settings,
	// set only when HoldoverSource is HoldoverSourceHardwareConfig.
	HoldoverHardwareConfigRef resource.HardwareConfigReference
}

// HoldoverSource is where a clock's holdover settings live.
type HoldoverSource int

// HoldoverSource values.
const (
	HoldoverSourceNone HoldoverSource = iota
	HoldoverSourcePlugin
	HoldoverSourceHardwareConfig
)

// HoldoverParameters holds a Holdover supporting Clock parameters.
type HoldoverParameters struct {
	LocalHoldoverTimeout   uint
	MaxInSpecOffset        uint
	LocalMaxHoldoverOffSet uint
}

// Interface is a network interface owned by a Clock.
type Interface struct {
	Name iface.Name
	Role profiles.PtpInterfaceRole
}

// InterfaceNames returns the names of the given interfaces.
func InterfaceNames(interfaces []*Interface) []iface.Name {
	names := make([]iface.Name, 0, len(interfaces))

	for _, interfaceInfo := range interfaces {
		names = append(names, interfaceInfo.Name)
	}

	return names
}

// Clocks is a collection of Clock, with cluster-wide filters as its own methods.
type Clocks []*Clock

// HoldoverEnabled returns the subset of clocks that are holdover-capable, or false if none are.
func (clks Clocks) HoldoverEnabled() (Clocks, bool) {
	var capable Clocks

	for _, clk := range clks {
		if clk.HoldoverEnabled {
			capable = append(capable, clk)
		}
	}

	return capable, len(capable) > 0
}

// OfType returns every clock of the given type.
func (clks Clocks) OfType(clkType ClockType) (matches Clocks, matched bool) {
	for _, clk := range clks {
		if clk.Type == clkType {
			matches = append(matches, clk)
		}
	}

	return matches, len(matches) > 0
}

// TimeReceiverIfaces returns every client-role interface owned by the clock.
func (clock *Clock) TimeReceiverIfaces() []*Interface {
	return clock.ifacesByRole(profiles.InterfaceRoleClient)
}

// TimeReceiverIface returns the clock's own single receiver-role interface, if any.
func (clock *Clock) TimeReceiverIface() (*Interface, bool) {
	receivers := clock.TimeReceiverIfaces()
	if len(receivers) == 0 {
		return nil, false
	}

	return receivers[0], true
}

// TimeTransmitterIfaces returns every server-role interface owned by the clock.
func (clock *Clock) TimeTransmitterIfaces() []*Interface {
	return clock.ifacesByRole(profiles.InterfaceRoleServer)
}

func (clock *Clock) ifacesByRole(role profiles.PtpInterfaceRole) []*Interface {
	var interfaces []*Interface

	for _, interfaceInfo := range clock.Interfaces {
		if interfaceInfo.Role == role {
			interfaces = append(interfaces, interfaceInfo)
		}
	}

	return interfaces
}

// clockFacts is the composition DetermineType decides ClockType from.
type clockFacts struct {
	HasTimeReceiver    bool
	HasTimeTransmitter bool
}

func (clock *Clock) facts() clockFacts {
	return clockFacts{
		HasTimeReceiver:    len(clock.TimeReceiverIfaces()) > 0,
		HasTimeTransmitter: len(clock.TimeTransmitterIfaces()) > 0,
	}
}

// DetermineType derives the clock's ClockType from its own composition.
func (clock *Clock) DetermineType() error {
	facts := clock.facts()

	switch {
	case facts.HasTimeReceiver && facts.HasTimeTransmitter:
		clock.Type = ClockTypeTBC
	case facts.HasTimeReceiver:
		clock.Type = ClockTypeTTSC
	default:
		return fmt.Errorf("clock on node %s has no Time Receiver interface", clock.NodeName)
	}

	return nil
}

// ClockClassPhase is a step in the clock-class cascade ExpectedClockClass keys off of. Locked/Freerun
// mirror SyncState (redhat-cne/sdk-go); HoldoverInSpec/OutOfSpec name ITU-T G.8275.1's own holdover
// modes (https://www.itu.int/rec/T-REC-G.8275.1).
type ClockClassPhase int

// ClockClassPhase values.
const (
	ClockClassPhaseLocked ClockClassPhase = iota
	ClockClassPhaseHoldoverInSpec
	ClockClassPhaseHoldoverOutOfSpec
	ClockClassPhaseFreerun
)

// clockClassStrategy computes a clock's expected class for a given phase. Each ClockType gets its own
// implementation -- callers never see the difference.
type clockClassStrategy interface {
	expectedClass(phase ClockClassPhase) metrics.PtpClockClass
}

// tbcClockClassStrategy is the T-BC cascade: 6 while locked (inherited from the upstream T-GM it
// tracks), 135 in holdover within spec, 165 out of spec, 248 with no time reference at all.
type tbcClockClassStrategy struct{}

func (tbcClockClassStrategy) expectedClass(phase ClockClassPhase) metrics.PtpClockClass {
	switch phase {
	case ClockClassPhaseLocked:
		return metrics.ClockClass6
	case ClockClassPhaseHoldoverInSpec:
		return metrics.ClockClass135
	case ClockClassPhaseHoldoverOutOfSpec:
		return metrics.ClockClass165
	case ClockClassPhaseFreerun:
		return metrics.ClockClass248
	default:
		return metrics.ClockClass6
	}
}

// ttscClockClassStrategy is spec-fixed at 255 (slave-only OC, never sends Announce messages).
type ttscClockClassStrategy struct{}

func (ttscClockClassStrategy) expectedClass(ClockClassPhase) metrics.PtpClockClass {
	return metrics.ClockClass255
}

func (clock *Clock) clockClassStrategyFor() clockClassStrategy {
	switch clock.Type {
	case ClockTypeTBC:
		return tbcClockClassStrategy{}
	case ClockTypeTTSC:
		// QUIRK: PTP operator 4.20 reports a standalone T-TSC's clock class following the T-BC cascade
		// instead of the spec-fixed 255; fixed independently in 4.21 and 4.22 --
		// https://github.com/openshift/linuxptp-daemon/commit/04c94fd1872d03d3712273efe22612e0c52398c0
		quirky, err := version.IsVersionStringInRange(clock.PtpOperatorVersion, "4.20", "4.21")
		if err == nil && quirky {
			return tbcClockClassStrategy{}
		}

		return ttscClockClassStrategy{}
	default:
		panic(fmt.Sprintf("clock.clockClassStrategyFor: unhandled ClockType %q", clock.Type))
	}
}

// ExpectedClockClass returns the clock class for the given phase (ITU-T G.8275.1, Clock Class
// Assignment table).
func (clock *Clock) ExpectedClockClass(phase ClockClassPhase) metrics.PtpClockClass {
	return clock.clockClassStrategyFor().expectedClass(phase)
}
