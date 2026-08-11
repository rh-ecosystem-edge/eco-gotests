package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// EnsureClocksAreLocked ensures that all PTP clocks are locked across all nodes covered by the Prometheus API client.
// It is designed to be used as a BeforeEach/AfterEach check to ensure the cluster is in a stable state.
//
// It ensures that clocks are locked for 10 seconds with a timeout of 5 minutes. Chronyd is excluded because this check
// establishes the healthy baseline where GNSS/PTP is the sync source. On versions before 4.20, chronyd remains FREERUN
// in that baseline; on 4.20+ it is stopped outside of NTP fallback. Chronyd becoming LOCKED indicates NTP fallback,
// which is tested separately, not the steady state this function validates.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API) error {
	query := ClockStateQuery{
		Process: DoesNotEqual(ProcessChronyd),
	}

	err := AssertQuery(context.TODO(), prometheusAPI, query, ClockStateLocked,
		AssertWithStableDuration(10*time.Second),
		AssertWithTimeout(5*time.Minute))
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreStable ensures that all PTP clocks are locked across all nodes for a specific continuous duration.
// This is useful for waiting for plugins (e.g. DPLL) to build a sufficient history buffer.
func EnsureClocksAreStable(prometheusAPI prometheusv1.API, stableDuration time.Duration) error {
	query := ClockStateQuery{
		Process: DoesNotEqual(ProcessChronyd),
	}

	err := AssertQuery(context.TODO(), prometheusAPI, query, ClockStateLocked,
		AssertWithStableDuration(stableDuration),
		AssertWithTimeout(stableDuration+5*time.Minute))
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are stable for %s: %w", stableDuration, err)
	}

	return nil
}
