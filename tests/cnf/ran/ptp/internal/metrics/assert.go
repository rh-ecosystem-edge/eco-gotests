package metrics

import (
	"context"
	"fmt"
	"math"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"golang.org/x/exp/constraints"
	"k8s.io/klog/v2"
)

const (
	// DefaultPollInterval is the poll interval used for a query assert when a timeout is specified but no poll
	// interval is provided.
	DefaultPollInterval = 5 * time.Second
)

// queryAssertOptions is a struct that holds the options for the AssertQuery function. It is unexported since the
// QueryAssertOption functions should be used to configure it.
type queryAssertOptions struct {
	timeout        time.Duration
	pollInterval   time.Duration
	stableDuration time.Duration
	startTime      time.Time
}

// newQueryAssertOptions creates a new queryAssertOptions struct with default values. This function should always be
// used instead of creating a new struct directly to ensure that the default values are set correctly.
func newQueryAssertOptions() *queryAssertOptions {
	return &queryAssertOptions{
		timeout:        0,
		pollInterval:   DefaultPollInterval,
		stableDuration: 0,
		startTime:      time.Now(),
	}
}

// QueryAssertOption is a function that configures assertions for the AssertQuery function.
type QueryAssertOption func(*queryAssertOptions)

// noopQueryAssertOption is a QueryAssertOption that does nothing. It is used when the value provided to a function
// returning a QueryAssertOption is invalid.
func noopQueryAssertOption(options *queryAssertOptions) {}

// AssertWithTimeout sets the timeout for the assertion. If the timeout is less than or equal to zero, it does nothing.
// Similarly, the timeout cannot be set to less than the stable duration. This upholds the invariant that timeout =
// max(timeout, stableDuration).
func AssertWithTimeout(timeout time.Duration) QueryAssertOption {
	if timeout <= 0 {
		return noopQueryAssertOption
	}

	return func(options *queryAssertOptions) {
		if options.stableDuration > timeout {
			return
		}

		options.timeout = timeout
	}
}

// AssertWithPollInterval sets the poll interval for the assertion. If the poll interval is less than or equal to zero,
// it does nothing. Note that if the poll interval is set to longer than the timeout, the assertion will only run once.
func AssertWithPollInterval(pollInterval time.Duration) QueryAssertOption {
	if pollInterval <= 0 {
		return noopQueryAssertOption
	}

	return func(options *queryAssertOptions) {
		options.pollInterval = pollInterval
	}
}

// AssertWithStableDuration sets the stable duration for the assertion. If the stable duration is less than or equal to
// zero, it does nothing. If the stable duration is set to longer than the timeout, the timeout is updated to be the
// stable duration. This upholds the invariant that timeout = max(timeout, stableDuration).
func AssertWithStableDuration(stableDuration time.Duration) QueryAssertOption {
	if stableDuration <= 0 {
		return noopQueryAssertOption
	}

	return func(options *queryAssertOptions) {
		if options.timeout <= stableDuration {
			options.timeout = stableDuration
		}

		options.stableDuration = stableDuration
	}
}

// AssertWithStartTime sets the start time for the assertion. If the start time is zero or in the future, it does
// nothing.
func AssertWithStartTime(startTime time.Time) QueryAssertOption {
	if startTime.IsZero() {
		return noopQueryAssertOption
	}

	if startTime.After(time.Now()) {
		return noopQueryAssertOption
	}

	return func(options *queryAssertOptions) {
		options.startTime = startTime
	}
}

// AssertQuery executes the provided MetricQuery and compares all values in the result vector to the expected value. In
// the base case, the query is executed once and all values in the result vector are compared to the expected value,
// after both the expected and actual values are converted to int64.
//
// Options can be provided to specify a timeout, poll interval, stable duration, and start time. In cases where the
// start time is provided alone, the query will be executed immediately at start time and return the result based on
// just the start time, which defaults to the current time.
//
// Timeout is equal to max(timeout, stableDuration) if at least one of them is provided. The behavior then is to ensure
// that the assertion succeeds at least once within the period between the start time and call time plus timeout and if
// stableDuration is provided, the query must succeed for polls over the entire stable duration. If the assertion fails,
// the running stable duration is reset.
//
// Type parameter V is the expected type of the query result, but is only used for strongly typing since both actual and
// expected values are converted before comparison.
//
// SECURITY: This function does not perform any sort of sanitization on the query. It should only be used with trusted
// queries.
func AssertQuery[V constraints.Integer](
	ctx context.Context, client prometheusv1.API, query Query[V], expected V, options ...QueryAssertOption) error {
	if client == nil {
		return fmt.Errorf("cannot assert query with nil client")
	}

	opts := newQueryAssertOptions()

	for _, option := range options {
		option(opts)
	}

	// queryTime is the time at which each query is executed. It begins as the start time and will be incremented by
	// the poll interval until the timeout is reached.
	queryTime := opts.startTime
	// stableTime is the first time at which a query succeeded, reset after a failure. The time in between the
	// stableTime and the queryTime is the running stable duration. For the query to be considered stable, the
	// running stable duration must be greater than or equal to the stable duration.
	stableTime := queryTime
	// lastTime is the time at which the query will stop being executed. It is the current time plus the timeout. If
	// queryTime is lastTime or later, the query is considered to have timed out.
	lastTime := time.Now().Add(opts.timeout)

	// Since Before is a strict comparison and the loop should run if the queryTime is earlier or equal to the
	// lastTime, the second condition is necessary. This is what allows the query to be executed exactly once if the
	// timeout is zero.
	for queryTime.Before(lastTime) || queryTime.Equal(lastTime) {
		select {
		// Wait until the queryTime is no longer in the future. If the queryTime is in the past, it is executed
		// immediately.
		case <-time.After(time.Until(queryTime)):
			// Actually execute the query at the queryTime. Since the metrics are saved to Prometheus, the
			// queryTime is allowed to be in the past and in fact always should be to avoid it being in the
			// future.
			err := assertQueryAtTime(ctx, client, query, expected, queryTime)
			// If the query succeeds and there is no stable duration, or the query has been stable for the
			// stable duration, return nil. This indicates the assertion has succeeded.
			if err == nil && (opts.stableDuration == 0 || queryTime.Sub(stableTime) >= opts.stableDuration) {
				return nil
			} else if err == nil {
				// If the query succeeds but the stable duration has not been reached, continue without
				// updating the stableTime. This allows the stableTime to be the earlier of the last
				// success or the queryTime. Query time must still be updated when we will query again.
				queryTime = queryTime.Add(opts.pollInterval)

				continue
			}

			klog.V(tsparams.LogLevel).Infof("Query assert failed at time %s: %v", queryTime, err)

			// After a failure, update the queryTime to the next queryTime by adding the poll interval.
			queryTime = queryTime.Add(opts.pollInterval)
			// Since the query failed, the earliest it can start being stable is the next queryTime, so
			// update queryTime first then set stableTime to the queryTime.
			stableTime = queryTime
		// If the context is done, return an error and consider the assertion to have failed. This allows the
		// caller to cancel or set their own timeout.
		case <-ctx.Done():
			return fmt.Errorf("failed to assert query eventually: context finished: %w", ctx.Err())
		}
	}

	return fmt.Errorf("failed to assert query eventually: timeout of %s exceeded", opts.timeout)
}

// AssertThresholds asserts that the expected thresholds, a map between profile names and their expected thresholds, are
// met at the current time. It uses the query to get the thresholds, ignoring the profile and threshold type labels
// (only using the node label if included). Profile names are expected to be unique.
//
// The assertion works by getting all the thresholds and building a map of the profile names to their actual thresholds.
// Then, it checks every entry of the expected map against the actual map. If any expected entry is not found in the
// actual map or it is found but with a different value, the assertion fails. Values are compared per key, with zero
// values in the expected PtpClockThreshold being ignored.
//
// SECURITY: This function does not perform any sort of sanitization on the query. It should only be used with trusted
// queries.
func AssertThresholds(
	ctx context.Context,
	client prometheusv1.API,
	query ThresholdQuery,
	expected map[string]ptpv1.PtpClockThreshold) error {
	if client == nil {
		return fmt.Errorf("cannot assert thresholds with nil client")
	}

	// Since only one query is executed, do not constrain the profile or threshold type labels. These will be
	// processed as part of the assertion.
	query.Profile = MetricLabel[string]{}
	query.ThresholdType = MetricLabel[PtpThresholdType]{}

	result, err := ExecuteQuery(ctx, client, query)
	if err != nil {
		return fmt.Errorf("failed to execute query to assert thresholds: %w", err)
	}

	actual := make(map[string]ptpv1.PtpClockThreshold)

	for _, sample := range result {
		if sample == nil {
			continue
		}

		profile, exists := sample.Metric[model.LabelName(KeyProfile)]
		if !exists {
			return fmt.Errorf("failed to find profile label in sample: %s", sample)
		}

		threshold, exists := sample.Metric[model.LabelName(KeyThreshold)]
		if !exists {
			return fmt.Errorf("failed to find threshold label in sample: %s", sample)
		}

		// Take the existing threshold if it exists. If it does not exist, existing defaults to a zero value.
		existing := actual[string(profile)]

		switch PtpThresholdType(threshold) {
		case ThresholdHoldoverTimeout:
			existing.HoldOverTimeout = convertSampleValueToInt64(sample.Value)
		case ThresholdMaxOffset:
			existing.MaxOffsetThreshold = convertSampleValueToInt64(sample.Value)
		case ThresholdMinOffset:
			existing.MinOffsetThreshold = convertSampleValueToInt64(sample.Value)
		default:
			klog.V(tsparams.LogLevel).Infof("Ignoring unknown threshold type %s", threshold)

			continue
		}

		// Update the actual map with modified thresholds. If the profile did not already exist, this adds it.
		actual[string(profile)] = existing
	}

	for profile, expectedThreshold := range expected {
		actualThreshold, ok := actual[profile]
		if !ok {
			return fmt.Errorf("expected threshold profile %s not found in actual thresholds", profile)
		}

		if expectedThreshold.HoldOverTimeout != 0 && actualThreshold.HoldOverTimeout != expectedThreshold.HoldOverTimeout {
			return fmt.Errorf("expected holdover timeout for profile %s to be %d, but got %d",
				profile, expectedThreshold.HoldOverTimeout, actualThreshold.HoldOverTimeout)
		}

		if expectedThreshold.MaxOffsetThreshold != 0 &&
			actualThreshold.MaxOffsetThreshold != expectedThreshold.MaxOffsetThreshold {
			return fmt.Errorf("expected max offset threshold for profile %s to be %d, but got %d",
				profile, expectedThreshold.MaxOffsetThreshold, actualThreshold.MaxOffsetThreshold)
		}

		if expectedThreshold.MinOffsetThreshold != 0 &&
			actualThreshold.MinOffsetThreshold != expectedThreshold.MinOffsetThreshold {
			return fmt.Errorf("expected min offset threshold for profile %s to be %d, but got %d",
				profile, expectedThreshold.MinOffsetThreshold, actualThreshold.MinOffsetThreshold)
		}
	}

	return nil
}

// assertQueryAtTime executes the provided MetricQuery and compares all values in the result vector to the expected
// value after converting the actual values to int64. It is used by AssertQuery to execute the query at a specific time.
// When AssertQuery is called with no options, this function is called once with the current time. Otherwise, the more
// complex logic in AssertQuery is used to poll with this function.
func assertQueryAtTime[V constraints.Integer](
	ctx context.Context, client prometheusv1.API, query Query[V], expected V, assertTime time.Time) error {
	metricQuery := query.ToMetricQuery()
	// Since this function is only called with non-zero assertTimes in the past, we can set the queryTime to be the
	// assertTime knowing it is valid. Setting the query time is done by setting the end time when using
	// ExecuteQuery.
	metricQuery.End = assertTime

	result, err := ExecuteQuery(ctx, client, metricQuery)
	if err != nil {
		return fmt.Errorf("failed to execute query %#v at time %s: %w", metricQuery, assertTime, err)
	}

	if len(result) == 0 {
		return fmt.Errorf("query assert error at time %s: no samples returned", assertTime)
	}

	for _, sample := range result {
		if sample == nil {
			continue
		}

		roundedValue := convertSampleValueToInt64(sample.Value)
		if roundedValue != int64(expected) {
			return fmt.Errorf("query assert error at time %s: expected %d, got %d\nquery: %s\nsample: %s",
				assertTime, int64(expected), roundedValue, metricQuery.String(), sample)
		}
	}

	klog.V(tsparams.LogLevel).Infof("Query assert passed at time %s: expected %d, got %#v\nquery: %s",
		assertTime, int64(expected), result, metricQuery.String())

	return nil
}

// convertSampleValueToInt64 converts a SampleValue to an int64 by rounding it to the nearest integer. This is intended
// as a safeguard against floating point precision issues, although they should not occur in practice.
func convertSampleValueToInt64(sampleValue model.SampleValue) int64 {
	return int64(math.Round(float64(sampleValue)))
}
