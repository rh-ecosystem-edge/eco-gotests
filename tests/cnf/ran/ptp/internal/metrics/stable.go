package metrics

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/constraints"
)

// StabilityOverTime asserts the queried metric holds within tolerance of its own first-observed value for at least
// requiredStability percent of samples over [start, end]. requiredStability=100 requires every sample to
// hold; 99.9 allows up to 0.1% of samples to deviate.
//
// For a discrete-valued metric (clock class, phase status, sync state), tolerance=0 means 'no change; flat slope'
// For a continuous-valued metric (offset, frequency adjustment), tolerance is a non-zero drift bound.
// For example while the clock is locked offset usually remains around -+0, but it could spike,
// As such it would be favourable to increase the tolerance to different value (e.g. 100),
// while maintaining the desired stability.
func StabilityOverTime[V constraints.Integer](
	ctx context.Context, client prometheusv1.API, query Query[V], start, end time.Time,
	tolerance V, requiredStability float64,
) error {
	if client == nil {
		return fmt.Errorf("cannot assert stability with nil client")
	}

	metricQuery := query.ToMetricQuery()
	metricQuery.Start = start
	metricQuery.End = end

	matrix, err := ExecuteQueryRange(ctx, client, metricQuery)
	if err != nil {
		return fmt.Errorf("failed to execute query range: %w", err)
	}

	for _, stream := range matrix {
		if len(stream.Values) == 0 {
			continue
		}

		expected := stream.Values[0].Value
		toleranceF := model.SampleValue(tolerance)

		goodCount := 0

		for _, sample := range stream.Values {
			if diff := sample.Value - expected; diff <= toleranceF && diff >= -toleranceF {
				goodCount++
			}
		}

		actualStability := float64(goodCount) / float64(len(stream.Values)) * 100
		if actualStability >= requiredStability {
			continue
		}

		return &OscillationError[V]{
			Metric:            stream.Metric,
			Expected:          V(expected),
			Tolerance:         tolerance,
			RequiredStability: requiredStability,
			ActualStability:   actualStability,
			GoodCount:         goodCount,
			TotalCount:        len(stream.Values),
			Runs:              buildMetricRuns[V](stream.Values),
		}
	}

	return nil
}

// MetricRun is a run of consecutive samples sharing the same value, collapsed for display.
type MetricRun[V constraints.Integer] struct {
	Value   V
	Count   int
	FirstAt time.Time
	LastAt  time.Time
}

// buildMetricRuns collapses consecutive samples sharing the same value into runs, in chronological order.
func buildMetricRuns[V constraints.Integer](values []model.SamplePair) []MetricRun[V] {
	var runs []MetricRun[V]

	for _, sample := range values {
		value := V(sample.Value)
		sampleTime := sample.Timestamp.Time()

		if len(runs) > 0 && runs[len(runs)-1].Value == value {
			runs[len(runs)-1].Count++
			runs[len(runs)-1].LastAt = sampleTime

			continue
		}

		runs = append(runs, MetricRun[V]{Value: value, Count: 1, FirstAt: sampleTime, LastAt: sampleTime})
	}

	return runs
}

// OscillationError reports Stable's own failure: the queried metric's actual stability over the window
// fell below the required percentage.
type OscillationError[V constraints.Integer] struct {
	Metric            model.Metric
	Expected          V
	Tolerance         V
	RequiredStability float64
	ActualStability   float64
	GoodCount         int
	TotalCount        int
	Runs              []MetricRun[V]
}

// Error renders a diff-style report as tab-aligned columns: the expected value/tolerance/stability
// target, then every chronological run of observed values with a repeat count, flagging the ones
// outside tolerance.
func (err *OscillationError[V]) Error() string {
	var b strings.Builder

	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "unexpected oscillation in metric %s\n", err.Metric)
	fmt.Fprintf(w, "Expected:\tmetric value %v (+/- %v), >= %.4g%% stable\n", err.Expected, err.Tolerance, err.RequiredStability)
	fmt.Fprintf(w, "Got:\t%.4g%% stable (%d/%d good)\n", err.ActualStability, err.GoodCount, err.TotalCount)

	for _, run := range err.Runs {
		flag := ""
		if absDiff(run.Value, err.Expected) > err.Tolerance {
			flag = "unexpected"
		}

		fmt.Fprintf(w, "\t%v\t%dx\t%s\n", run.Value, run.Count, flag)
	}

	w.Flush()

	return b.String()
}

// absDiff returns the absolute difference between a and b, safe for unsigned V.
func absDiff[V constraints.Integer](a, b V) V {
	if a > b {
		return a - b
	}

	return b - a
}
