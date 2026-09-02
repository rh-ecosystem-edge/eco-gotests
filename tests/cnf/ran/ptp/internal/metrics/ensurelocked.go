package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

const (
	defaultLockedStableDuration = 10 * time.Second
	defaultLockedTimeout        = 5 * time.Minute
)

// LockedClockExpectations returns expectations for the standard cluster-wide locked baseline.
// Chronyd is excluded: on 4.20+ it is stopped outside NTP fallback; when running pre-4.20 it
// stays FREERUN while PTP is the sync source.
func LockedClockExpectations() []QueryExpectation {
	return Set(ExpectLocked(ClockStateQuery{
		Process: DoesNotEqual(ProcessChronyd),
	}))
}

// AssertAllClocksLocked asserts [LockedClockExpectations] with caller-supplied poll options.
func AssertAllClocksLocked(prometheusAPI prometheusv1.API, options ...QueryAssertOption) error {
	err := AssertQuerySet(context.TODO(), prometheusAPI, LockedClockExpectations(), options...)
	if err != nil {
		return fmt.Errorf("failed to assert all clocks are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreLocked ensures that all PTP clocks are locked across all nodes covered by the Prometheus API client.
// It is designed to be used as a BeforeEach/AfterEach check to ensure the cluster is in a stable state.
//
// It ensures that clocks are locked for 10 seconds with a timeout of 5 minutes.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API) error {
	err := AssertAllClocksLocked(prometheusAPI,
		AssertWithStableDuration(defaultLockedStableDuration),
		AssertWithTimeout(defaultLockedTimeout))
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreStable ensures that all PTP clocks are locked across all nodes for a specific continuous duration.
// This is useful for waiting for plugins (e.g. DPLL) to build a sufficient history buffer.
func EnsureClocksAreStable(prometheusAPI prometheusv1.API, stableDuration time.Duration) error {
	err := AssertAllClocksLocked(prometheusAPI,
		AssertWithStableDuration(stableDuration),
		AssertWithTimeout(stableDuration+5*time.Minute))
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are stable for %s: %w", stableDuration, err)
	}

	return nil
}
