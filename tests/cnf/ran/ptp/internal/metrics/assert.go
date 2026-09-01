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

	return pollQueryAssertions(ctx, client, []QueryExpectation{Expect(query, expected)}, opts, "query")
}

// AssertThresholdsOption configures optional behavior for [AssertThresholds].
type AssertThresholdsOption func(*assertThresholdsConfig)

// assertThresholdsConfig holds configuration for [AssertThresholds].
type assertThresholdsConfig struct {
	keyNormalizer func(string) string
}

// WithKeyNormalizer sets a function that transforms the Prometheus profile label before it is used as a key in the
// actual thresholds map. This is useful when the label format differs from the expected map's keys, for example when
// the daemon uses qualified profile names (4.22+) but the expected map is keyed by unqualified spec-level names.
func WithKeyNormalizer(normalizer func(string) string) AssertThresholdsOption {
	if normalizer == nil {
		return func(c *assertThresholdsConfig) {}
	}

	return func(c *assertThresholdsConfig) {
		c.keyNormalizer = normalizer
	}
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
	expected map[string]ptpv1.PtpClockThreshold,
	opts ...AssertThresholdsOption) error {
	cfg := &assertThresholdsConfig{
		keyNormalizer: func(s string) string { return s },
	}

	for _, opt := range opts {
		opt(cfg)
	}

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

	actual, err := buildActualThresholds(result, cfg.keyNormalizer)
	if err != nil {
		return err
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

// buildActualThresholds converts Prometheus samples into a map of profile names to their observed thresholds. The
// keyNormalizer transforms the raw profile label before it is used as a map key.
func buildActualThresholds(
	result model.Vector, keyNormalizer func(string) string) (map[string]ptpv1.PtpClockThreshold, error) {
	actual := make(map[string]ptpv1.PtpClockThreshold)

	for _, sample := range result {
		if sample == nil {
			continue
		}

		profile, exists := sample.Metric[model.LabelName(KeyProfile)]
		if !exists {
			return nil, fmt.Errorf("failed to find profile label in sample: %s", sample)
		}

		threshold, exists := sample.Metric[model.LabelName(KeyThreshold)]
		if !exists {
			return nil, fmt.Errorf("failed to find threshold label in sample: %s", sample)
		}

		normalizedProfile := string(profile)
		if keyNormalizer != nil {
			normalizedProfile = keyNormalizer(normalizedProfile)
		}

		existing := actual[normalizedProfile]

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

		actual[normalizedProfile] = existing
	}

	return actual, nil
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
