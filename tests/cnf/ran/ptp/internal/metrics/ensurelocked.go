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

// ensureLockedConfig holds the configuration for EnsureClocksAreLocked and EnsureClocksAreStable.
type ensureLockedConfig struct {
	expectedClockStates []ExpectedClockState
	assertOptions       []QueryAssertOption
}

// EnsureLockedOption configures optional behavior for EnsureClocksAreLocked and EnsureClocksAreStable.
type EnsureLockedOption func(*ensureLockedConfig)

// WithExpectedClockStates configures profile-aware presence checks. When provided, EnsureClocksAreLocked asserts
// each expected (process, iface, node) series is present and LOCKED via a single [AssertQuerySet] snapshot per
// poll tick. Missing series fail with "no samples returned" from [assertQueryAtTime].
func WithExpectedClockStates(expected []ExpectedClockState) EnsureLockedOption {
	return func(cfg *ensureLockedConfig) {
		cfg.expectedClockStates = expected
	}
}

// WithAssertOptions passes poll options through to the underlying [AssertQuerySet] call.
func WithAssertOptions(options ...QueryAssertOption) EnsureLockedOption {
	return func(cfg *ensureLockedConfig) {
		cfg.assertOptions = append(cfg.assertOptions, options...)
	}
}

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

// EnsureClocksAreLocked ensures that all PTP clocks are locked across all nodes covered by the Prometheus API client.
// It is designed to be used as a BeforeEach/AfterEach check to ensure the cluster is in a stable state.
//
// By default it uses the broad cluster-wide locked baseline ([LockedClockExpectations]). When
// [WithExpectedClockStates] is provided, it asserts each expected series is present and LOCKED.
//
// It ensures that clocks are locked for 10 seconds with a timeout of 5 minutes unless overridden by
// [WithAssertOptions].
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API, opts ...EnsureLockedOption) error {
	cfg := &ensureLockedConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	assertOpts := cfg.assertOptions
	if len(assertOpts) == 0 {
		assertOpts = []QueryAssertOption{
			AssertWithStableDuration(defaultLockedStableDuration),
			AssertWithTimeout(defaultLockedTimeout),
		}
	}

	var expectations []QueryExpectation

	if len(cfg.expectedClockStates) > 0 {
		deduped := deduplicateExpectedClockStates(cfg.expectedClockStates)

		klog.V(tsparams.LogLevel).Infof("Ensuring expected clock state metrics are present and locked:\n%s",
			FormatExpectedClockStates(deduped))

		expectations = LockedExpectationsFromExpected(deduped)
	} else {
		expectations = LockedClockExpectations()
	}

	err := AssertQuerySet(context.TODO(), prometheusAPI, expectations, assertOpts...)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked: %w", err)
	}

	return nil
}

// EnsureClocksAreStable ensures that all PTP clocks are locked across all nodes for a specific continuous duration.
// This is useful for waiting for plugins (e.g. DPLL) to build a sufficient history buffer.
//
// When [WithExpectedClockStates] is provided, each expected series must be present and LOCKED for the full
// stable duration.
func EnsureClocksAreStable(
	prometheusAPI prometheusv1.API, stableDuration time.Duration, opts ...EnsureLockedOption,
) error {
	cfg := &ensureLockedConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	assertOpts := cfg.assertOptions
	if len(assertOpts) == 0 {
		assertOpts = []QueryAssertOption{
			AssertWithStableDuration(stableDuration),
			AssertWithTimeout(stableDuration + 5*time.Minute),
		}
	}

	var expectations []QueryExpectation

	if len(cfg.expectedClockStates) > 0 {
		deduped := deduplicateExpectedClockStates(cfg.expectedClockStates)

		klog.V(tsparams.LogLevel).Infof("Ensuring expected clock state metrics are stable and locked:\n%s",
			FormatExpectedClockStates(deduped))

		expectations = LockedExpectationsFromExpected(deduped)
	} else {
		expectations = LockedClockExpectations()
	}

	err := AssertQuerySet(context.TODO(), prometheusAPI, expectations, assertOpts...)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are stable for %s: %w", stableDuration, err)
	}

	return nil
}
