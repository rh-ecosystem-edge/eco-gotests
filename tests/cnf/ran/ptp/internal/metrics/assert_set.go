package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"golang.org/x/exp/constraints"
	"k8s.io/klog/v2"
)

// QueryExpectation pairs a query with its expected value for use in [AssertQuerySet].
// Construct expectations with [Expect] to preserve compile-time type safety at the call site.
type QueryExpectation struct {
	// query is stored as MetricQuery[int64] for type erasure so heterogeneous expectations can share one slice.
	// PTP metric values are integers; erasure is safe when built via [Expect].
	query    MetricQuery[int64]
	expected int64
}

func (expectation QueryExpectation) description() string {
	return fmt.Sprintf("%s (expected %d)", expectation.query.String(), expectation.expected)
}

// Set wraps one or more expectations for [AssertQuerySet]. Use for single-metric checks as well as
// multi-metric snapshots so call sites can add expectations without changing assertion machinery.
func Set(expectations ...QueryExpectation) []QueryExpectation {
	return expectations
}

// ExpectLocked returns an expectation that clock_state equals [ClockStateLocked].
func ExpectLocked(query ClockStateQuery) QueryExpectation {
	return Expect(query, ClockStateLocked)
}

// ExpectFromMetricQuery returns a [QueryExpectation] for a raw [MetricQuery]. Use this when exact iface labels
// must be preserved (e.g. ptp4l on OC profiles) without [ClockStateQuery]'s ensureNIC() conversion.
func ExpectFromMetricQuery(query MetricQuery[PtpClockState], expected PtpClockState) QueryExpectation {
	return Expect(query, expected)
}

// Expect returns a [QueryExpectation] for the given query and expected value.
func Expect[V constraints.Integer](query Query[V], expected V) QueryExpectation {
	return QueryExpectation{
		query:    MetricQuery[int64](query.ToMetricQuery()),
		expected: int64(expected),
	}
}

// AssertQuerySet polls like [AssertQuery], but at each tick evaluates all expectations at the same queryTime
// before deciding pass or fail. This captures a consistent snapshot of multiple metrics.
//
// Options can be provided to specify a timeout, poll interval, stable duration, and start time. Timeout is equal to
// max(timeout, stableDuration) if at least one of them is provided. If any expectation fails at a given queryTime, the
// whole tick fails and the running stable duration is reset.
//
// SECURITY: This function does not perform any sort of sanitization on the query. It should only be used with trusted
// queries.
func AssertQuerySet(
	ctx context.Context,
	client prometheusv1.API,
	expectations []QueryExpectation,
	options ...QueryAssertOption,
) error {
	return assertQueryExpectations(ctx, client, expectations, "query set", options...)
}

func assertQueryExpectations(
	ctx context.Context,
	client prometheusv1.API,
	expectations []QueryExpectation,
	assertionKind string,
	options ...QueryAssertOption,
) error {
	if client == nil {
		return fmt.Errorf("cannot assert %s with nil client", assertionKind)
	}

	if len(expectations) == 0 {
		return fmt.Errorf("cannot assert %s with no expectations", assertionKind)
	}

	opts := newQueryAssertOptions()

	for _, option := range options {
		option(opts)
	}

	return pollQueryAssertions(ctx, client, expectations, opts, assertionKind)
}

// pollQueryAssertions runs the poll loop shared by [AssertQuery] and [AssertQuerySet].
func pollQueryAssertions(
	ctx context.Context,
	client prometheusv1.API,
	expectations []QueryExpectation,
	opts *queryAssertOptions,
	assertionKind string,
) error {
	queryTime := opts.startTime
	stableTime := queryTime
	lastTime := time.Now().Add(opts.timeout)

	for queryTime.Before(lastTime) || queryTime.Equal(lastTime) {
		select {
		case <-time.After(time.Until(queryTime)):
			err := assertQueriesAtTime(ctx, client, expectations, queryTime)
			if err == nil && (opts.stableDuration == 0 || queryTime.Sub(stableTime) >= opts.stableDuration) {
				return nil
			} else if err == nil {
				queryTime = queryTime.Add(opts.pollInterval)

				continue
			}

			klog.V(tsparams.LogLevel).Infof("Query set assert failed at time %s: %v", queryTime, err)

			queryTime = queryTime.Add(opts.pollInterval)
			stableTime = queryTime
		case <-ctx.Done():
			return fmt.Errorf("failed to assert %s eventually: context finished: %w", assertionKind, ctx.Err())
		}
	}

	return fmt.Errorf("failed to assert %s eventually: timeout of %s exceeded", assertionKind, opts.timeout)
}

// assertQueriesAtTime evaluates all expectations at the same assertTime and returns the first failure.
func assertQueriesAtTime(
	ctx context.Context,
	client prometheusv1.API,
	expectations []QueryExpectation,
	assertTime time.Time,
) error {
	for _, expectation := range expectations {
		if err := assertQueryAtTime(ctx, client, expectation.query, expectation.expected, assertTime); err != nil {
			return fmt.Errorf("expectation %q failed: %w", expectation.description(), err)
		}
	}

	return nil
}
