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
// Readiness is asserted in two stages. The ptp4l and phc2sys process statuses are checked first, because a daemon that
// is midway through a config reload has torn those processes down, and the clock state it last reported describes the
// processes that no longer exist. Only once the processes are up is clock state asserted.
//
// This establishes that the daemon is healthy now. It cannot establish that the daemon has observed every PtpConfig
// write: a reload that the daemon has not yet picked up is invisible to both metrics, so callers that have just
// modified a PtpConfig must additionally wait on the daemon reload barrier. See [profiles.RestoreAndSettle].
//
// It ensures the baseline holds for 10 seconds with a timeout of 5 minutes. Chronyd is excluded because this check
// establishes the healthy baseline where GNSS/PTP is the sync source. On versions before 4.20, chronyd remains FREERUN
// in that baseline; on 4.20+ it is stopped outside of NTP fallback. Chronyd becoming LOCKED indicates NTP fallback,
// which is tested separately, not the steady state this function validates.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API) error {
	// Each process is asserted with its own query rather than one regex alternation over both. An alternation
	// returns a single result set, and the assertion only fails on a sample that is not up, so a result holding
	// only ptp4l samples satisfies it while phc2sys is absent entirely. Querying separately forces each process
	// to return samples of its own.
	for _, process := range []PtpProcess{ProcessPTP4L, ProcessPHC2SYS} {
		processQuery := ProcessStatusQuery{
			Process: Equals(process),
		}

		err := AssertQuery(context.TODO(), prometheusAPI, processQuery, ProcessStatusUp,
			AssertWithStableDuration(10*time.Second),
			AssertWithTimeout(5*time.Minute))
		if err != nil {
			return fmt.Errorf("failed to ensure %s processes are up: %w", process, err)
		}
	}

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
