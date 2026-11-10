package profiles

import (
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

// HoldoverTestData groups the per-node test context that is discovered once in BeforeEach and shared
// by all test cases within a Context block.
type HoldoverTestData struct {
	PrometheusAPI  prometheusv1.API
	NodeName       string
	ProfileInfo    *ProfileInfo
	UpstreamIfaces []iface.Iface
	ConfigFile     string
}

// HoldoverExpectedClockClasses groups the expected clock class values for each holdover state.
type HoldoverExpectedClockClasses struct {
	Locked            metrics.PtpClockClass
	HoldoverInSpec    metrics.PtpClockClass
	HoldoverOutOfSpec metrics.PtpClockClass
	Freerun           metrics.PtpClockClass
}

var (
	// HoldoverPluginSettingsNoOutOfSpec are WPC/E810 stress holdover settings without out-of-spec
	// (localMaxHoldoverOffSet < maxInSpecOffset so FREERUN is reached before holdover-out-of-spec).
	HoldoverPluginSettingsNoOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        14401,
		LocalMaxHoldoverOffSet: 14400,
	}
	// HoldoverPluginSettingsWithOutOfSpec are WPC/E810 stress holdover settings with out-of-spec
	// (maxInSpecOffset < localMaxHoldoverOffSet so holdover-out-of-spec occurs before FREERUN).
	HoldoverPluginSettingsWithOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        1800,
		LocalMaxHoldoverOffSet: 14400,
	}

	// HoldoverPluginSettingsGNRDNoOutOfSpec are GNR-D/HardwareConfig QE-aligned settings without
	// out-of-spec. localMaxHoldoverOffSet matches the default ptpClockThreshold ±100 lock band
	// (CNF-30026 / OCPBUGS-111642).
	HoldoverPluginSettingsGNRDNoOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   300,
		MaxInSpecOffset:        200,
		LocalMaxHoldoverOffSet: 100,
	}
	// HoldoverPluginSettingsGNRDWithOutOfSpec are GNR-D/HardwareConfig QE-aligned settings with
	// out-of-spec. localMaxHoldoverOffSet is above the default lock band, so changeHoldoverSettings
	// also raises ptpClockThreshold Max/Min to ±1500 for the case duration.
	HoldoverPluginSettingsGNRDWithOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   300,
		MaxInSpecOffset:        40,
		LocalMaxHoldoverOffSet: 1500,
	}
)

// DefaultPtpClockThresholdOffset is the daemon default for ptpClockThreshold MaxOffsetThreshold
// (MinOffsetThreshold is the negative). GNR-D presets that use this value need no PtpConfig patch.
const DefaultPtpClockThresholdOffset = 100

// UsesGNRDHoldoverPresets reports whether holdover cases should use GNR-D QE-aligned values
// and matching ptpClockThreshold. OCPBUGS-111642 / CNF-30026 only affect HardwareConfig T-BC
// and dual T-BC, not T-TSC.
func UsesGNRDHoldoverPresets(profileInfo *ProfileInfo) bool {
	if profileInfo == nil || profileInfo.HardwareConfig == nil {
		return false
	}

	return profileInfo.ProfileType == ProfileTypeTBCReceiver ||
		profileInfo.ProfileType == ProfileTypeDualTBCReceiver
}

// HoldoverSettingsFor returns the requested WPC preset, or the GNR-D equivalent for HardwareConfig
// T-BC and dual T-BC profiles.
func HoldoverSettingsFor(profileInfo *ProfileInfo, requested HoldoverPluginSettings) HoldoverPluginSettings {
	if !UsesGNRDHoldoverPresets(profileInfo) {
		return requested
	}

	switch requested {
	case HoldoverPluginSettingsNoOutOfSpec:
		return HoldoverPluginSettingsGNRDNoOutOfSpec
	case HoldoverPluginSettingsWithOutOfSpec:
		return HoldoverPluginSettingsGNRDWithOutOfSpec
	default:
		return requested
	}
}

// TBCClockClasses returns the standard clock class values for T-BC tests.
func TBCClockClasses() HoldoverExpectedClockClasses {
	return HoldoverExpectedClockClasses{
		Locked:            metrics.ClockClass6,
		HoldoverInSpec:    metrics.ClockClass135,
		HoldoverOutOfSpec: metrics.ClockClass165,
		Freerun:           metrics.ClockClass248,
	}
}
