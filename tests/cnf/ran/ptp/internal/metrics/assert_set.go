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

// QueryExpectation pairs a typed query with its expected value for use in [AssertQuerySet].
// Construct expectations with [Expect] to preserve compile-time type safety.
type QueryExpectation struct {
	description string
	assert      func(ctx context.Context, client prometheusv1.API, assertTime time.Time) error
}

// Expect returns a [QueryExpectation] for the given query and expected value.
func Expect[V constraints.Integer](query Query[V], expected V) QueryExpectation {
	metricQuery := query.ToMetricQuery()

	return QueryExpectation{
		description: fmt.Sprintf("%s (expected %d)", metricQuery.String(), int64(expected)),
		assert: func(ctx context.Context, client prometheusv1.API, assertTime time.Time) error {
			return assertQueryAtTime(ctx, client, query, expected, assertTime)
		},
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
	if client == nil {
		return fmt.Errorf("cannot assert query set with nil client")
	}

	if len(expectations) == 0 {
		return fmt.Errorf("cannot assert query set with no expectations")
	}

	opts := newQueryAssertOptions()

	for _, option := range options {
		option(opts)
	}

	return pollQueryAssertions(ctx, client, expectations, opts, "query set")
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
		if err := expectation.assert(ctx, client, assertTime); err != nil {
			if expectation.description != "" {
				return fmt.Errorf("expectation %q failed: %w", expectation.description, err)
			}

			return err
		}
	}

	return nil
}
