package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
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

// LockedExpectationsFromExpected builds one LOCKED expectation per [ExpectedClockState], using exact iface
// labels from [ExpectedClockState.toQuery] rather than [ClockStateQuery] (which applies ensureNIC conversion).
func LockedExpectationsFromExpected(states []ExpectedClockState) []QueryExpectation {
	deduped := deduplicateExpectedClockStates(states)
	expectations := make([]QueryExpectation, 0, len(deduped))

	for _, state := range deduped {
		expectations = append(expectations, ExpectFromMetricQuery(state.toQuery(), ClockStateLocked))
	}

	return expectations
}

// AssertAllClocksLocked asserts [LockedClockExpectations] with caller-supplied poll options.
func AssertAllClocksLocked(prometheusAPI prometheusv1.API, options ...QueryAssertOption) error {
	err := AssertQuerySet(context.TODO(), prometheusAPI, LockedClockExpectations(), options...)
	if err != nil {
		return fmt.Errorf("failed to assert all clocks are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreLocked ensures that all reported PTP clock state metrics (except chronyd) are LOCKED across
// the cluster. It is designed for BeforeEach/AfterEach checks when a broad baseline is sufficient.
//
// Clocks must remain LOCKED for 10 seconds with a timeout of 5 minutes.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API) error {
	assertOpts := []QueryAssertOption{
		AssertWithStableDuration(defaultLockedStableDuration),
		AssertWithTimeout(defaultLockedTimeout),
	}

	err := AssertQuerySet(context.TODO(), prometheusAPI, LockedClockExpectations(), assertOpts...)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked: %w", err)
	}

	return nil
}

// EnsureExpectedClockStatesAreLocked asserts each expected (process, iface, node) openshift_ptp_clock_state
// series is present and LOCKED via a single [AssertQuerySet] snapshot per poll tick. Missing series fail with
// "no samples returned" from [assertQueryAtTime].
//
// By default clocks must remain LOCKED for 10 seconds with a timeout of 5 minutes. Pass [QueryAssertOption]
// values to override stable duration or timeout.
func EnsureExpectedClockStatesAreLocked(
	prometheusAPI prometheusv1.API,
	expected []ExpectedClockState,
	options ...QueryAssertOption,
) error {
	assertOpts := options
	if len(assertOpts) == 0 {
		assertOpts = []QueryAssertOption{
			AssertWithStableDuration(defaultLockedStableDuration),
			AssertWithTimeout(defaultLockedTimeout),
		}
	}

	deduped := deduplicateExpectedClockStates(expected)

	klog.V(tsparams.LogLevel).Infof("Ensuring expected clock state metrics are present and locked:\n%s",
		FormatExpectedClockStates(deduped))

	err := AssertQuerySet(context.TODO(), prometheusAPI, LockedExpectationsFromExpected(deduped), assertOpts...)
	if err != nil {
		return fmt.Errorf("failed to ensure expected clock states are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreStable ensures that all PTP clocks are locked across all nodes for a specific continuous duration.
// This is useful for waiting for plugins (e.g. DPLL) to build a sufficient history buffer.
func EnsureClocksAreStable(prometheusAPI prometheusv1.API, stableDuration time.Duration) error {
	assertOpts := []QueryAssertOption{
		AssertWithStableDuration(stableDuration),
		AssertWithTimeout(stableDuration + 5*time.Minute),
	}

	err := AssertQuerySet(context.TODO(), prometheusAPI, LockedClockExpectations(), assertOpts...)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are stable for %s: %w", stableDuration, err)
	}

	return nil
}
