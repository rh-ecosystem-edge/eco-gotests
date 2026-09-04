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
	UpstreamIfaces []iface.Name
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
	// HoldoverPluginSettingsNoOutOfSpec are the holdover settings without out-of-spec.
	HoldoverPluginSettingsNoOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        14401,
		LocalMaxHoldoverOffSet: 14400,
	}
	// HoldoverPluginSettingsWithOutOfSpec are the holdover settings with out-of-spec.
	HoldoverPluginSettingsWithOutOfSpec = HoldoverPluginSettings{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        1800,
		LocalMaxHoldoverOffSet: 14400,
	}
)

// TBCClockClasses returns the standard clock class values for T-BC tests.
func TBCClockClasses() HoldoverExpectedClockClasses {
	return HoldoverExpectedClockClasses{
		Locked:            metrics.ClockClass6,
		HoldoverInSpec:    metrics.ClockClass135,
		HoldoverOutOfSpec: metrics.ClockClass165,
		Freerun:           metrics.ClockClass248,
	}
}
