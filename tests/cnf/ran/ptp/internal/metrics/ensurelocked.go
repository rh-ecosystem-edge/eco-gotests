package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// ensureLockedConfig holds the configuration for EnsureClocksAreLocked and EnsureClocksAreStable.
type ensureLockedConfig struct {
	expectedClockStates []ExpectedClockState
}

// EnsureLockedOption configures optional behavior for EnsureClocksAreLocked and EnsureClocksAreStable.
type EnsureLockedOption func(*ensureLockedConfig)

// WithExpectedClockStates configures the presence check phase. When provided, EnsureClocksAreLocked will first
// verify that all expected clock state metrics are present (at least one sample returned per entry) before
// proceeding to the locked assertion. This ensures that missing metrics (e.g., a dpll or gnss process that
// never started) are caught rather than silently passing.
func WithExpectedClockStates(expected []ExpectedClockState) EnsureLockedOption {
	return func(cfg *ensureLockedConfig) {
		cfg.expectedClockStates = expected
	}
}

// EnsureClocksAreLocked ensures that all PTP clocks are locked across all nodes covered by the Prometheus API client.
// It is designed to be used as a BeforeEach/AfterEach check to ensure the cluster is in a stable state.
//
// It ensures that clocks are locked for 10 seconds with a timeout of 5 minutes. It does not check the clock state of
// the chronyd process, as it will be FREERUN when PTP is working correctly.
//
// When WithExpectedClockStates is provided, the function first verifies that all expected metric series are present
// before asserting the locked state.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API, opts ...EnsureLockedOption) error {
	cfg := &ensureLockedConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if len(cfg.expectedClockStates) > 0 {
		deduped := deduplicateExpectedClockStates(cfg.expectedClockStates)

		klog.V(tsparams.LogLevel).Infof("Ensuring expected clock state metrics are present:\n%s",
			FormatExpectedClockStates(deduped))

		err := ensureClockStatesPresent(prometheusAPI, deduped, 5*time.Minute)
		if err != nil {
			return fmt.Errorf("failed presence check before asserting locked state: %w", err)
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
//
// When WithExpectedClockStates is provided, the function first verifies that all expected metric series are present
// before asserting the stable locked state.
func EnsureClocksAreStable(
	prometheusAPI prometheusv1.API, stableDuration time.Duration, opts ...EnsureLockedOption,
) error {
	cfg := &ensureLockedConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if len(cfg.expectedClockStates) > 0 {
		deduped := deduplicateExpectedClockStates(cfg.expectedClockStates)

		klog.V(tsparams.LogLevel).Infof("Ensuring expected clock state metrics are present:\n%s",
			FormatExpectedClockStates(deduped))

		err := ensureClockStatesPresent(prometheusAPI, deduped, 5*time.Minute)
		if err != nil {
			return fmt.Errorf("failed presence check before asserting stable state: %w", err)
		}
	}

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

// ensureClockStatesPresent polls Prometheus until all expected clock state metrics return at least one sample.
// It retries with a 5-second poll interval until the timeout is exceeded.
func ensureClockStatesPresent(
	prometheusAPI prometheusv1.API, expected []ExpectedClockState, timeout time.Duration) error {
	ctx := context.TODO()
	deadline := time.Now().Add(timeout)
	pollInterval := DefaultPollInterval

	for {
		var missing []ExpectedClockState

		for _, entry := range expected {
			query := entry.toQuery()

			result, err := ExecuteQuery(ctx, prometheusAPI, query)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Presence check query error for %s: %v", entry, err)

				missing = append(missing, entry)

				continue
			}

			if len(result) == 0 {
				missing = append(missing, entry)
			}
		}

		if len(missing) == 0 {
			klog.V(tsparams.LogLevel).Infof("All %d expected clock state metrics are present", len(expected))

			return nil
		}

		if time.Now().After(deadline) {
			var stringBuilder strings.Builder

			fmt.Fprintf(&stringBuilder, "timed out after %s waiting for %d/%d expected clock state metrics:\n",
				timeout, len(missing), len(expected))

			for _, m := range missing {
				stringBuilder.WriteString("  - missing: ")
				stringBuilder.WriteString(m.String())
				stringBuilder.WriteString("\n")
			}

			return fmt.Errorf("%s", stringBuilder.String())
		}

		klog.V(tsparams.LogLevel).Infof("Presence check: %d/%d metrics missing, retrying in %s",
			len(missing), len(expected), pollInterval)

		time.Sleep(pollInterval)
	}
}
