//go:build unit_test

package metrics_test

import (
	"context"
	"testing"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/stretchr/testify/require"
)

// fakeRangeAPI returns a fixed matrix from QueryRange, regardless of the query string.
type fakeRangeAPI struct {
	prometheusv1.API
	matrix model.Matrix
}

func (f *fakeRangeAPI) QueryRange(
	_ context.Context, _ string, _ prometheusv1.Range, _ ...prometheusv1.Option,
) (model.Value, prometheusv1.Warnings, error) {
	return f.matrix, nil, nil
}

// samplesAt builds a SamplePair series one second apart starting at base, one entry per value given.
func samplesAt(base time.Time, values ...float64) []model.SamplePair {
	samples := make([]model.SamplePair, 0, len(values))
	for i, value := range values {
		samples = append(samples, model.SamplePair{
			Timestamp: model.TimeFromUnix(base.Add(time.Duration(i) * time.Second).Unix()),
			Value:     model.SampleValue(value),
		})
	}

	return samples
}

func TestStabilityOverTime_AllSamplesMatch_Passes(t *testing.T) {
	base := time.Now()
	client := &fakeRangeAPI{matrix: model.Matrix{{
		Metric: model.Metric{"__name__": "openshift_ptp_clock_class"},
		Values: samplesAt(base, 135, 135, 135, 135),
	}}}

	err := metrics.StabilityOverTime[metrics.PtpClockClass](
		context.TODO(), client, metrics.ClockClassQuery{}, base, base.Add(4*time.Second), 0, 100)
	require.NoError(t, err)
}

// TestStabilityOverTime_OneBlip_FailsAt100PercentButPassesAt89Percent grounds requiredStability's own
// SLO semantics in a real regression's own pattern: HOLDOVER (135) sustained, one spurious excursion to
// FREERUN's clock class (248), then back to 135 -- self-recovers, matching the ticket's own log.
// Jira: https://redhat.atlassian.net/browse/OCPBUGS-90101
func TestStabilityOverTime_OneBlip_FailsAt100PercentButPassesAt89Percent(t *testing.T) {
	base := time.Now()
	client := &fakeRangeAPI{matrix: model.Matrix{{
		Metric: model.Metric{"__name__": "openshift_ptp_clock_class"},
		Values: samplesAt(base, 135, 135, 135, 135, 248, 135, 135, 135, 135, 135),
	}}}

	err := metrics.StabilityOverTime[metrics.PtpClockClass](
		context.TODO(), client, metrics.ClockClassQuery{}, base, base.Add(10*time.Second), 0, 100)
	require.Error(t, err)

	var oscillation *metrics.OscillationError[metrics.PtpClockClass]
	require.ErrorAs(t, err, &oscillation)
	require.Equal(t, metrics.PtpClockClass(135), oscillation.Expected)
	require.InDelta(t, 90.0, oscillation.ActualStability, 0.01)
	require.Equal(t, 9, oscillation.GoodCount)
	require.Equal(t, 10, oscillation.TotalCount)
	require.Equal(t,
		"unexpected oscillation in metric openshift_ptp_clock_class\n"+
			"Expected:  metric value 135 (+/- 0), >= 100% stable\n"+
			"Got:       90% stable (9/10 good)\n"+
			"           135  4x  \n"+
			"           248  1x  unexpected\n"+
			"           135  5x  \n",
		err.Error())

	// 90% actual stability clears a 89% SLO -- the same data, a looser requirement, now passes.
	err = metrics.StabilityOverTime[metrics.PtpClockClass](
		context.TODO(), client, metrics.ClockClassQuery{}, base, base.Add(10*time.Second), 0, 89)
	require.NoError(t, err)
}

func TestStabilityOverTime_ContinuousMetric_ToleratesJitterWithinBound(t *testing.T) {
	base := time.Now()
	client := &fakeRangeAPI{matrix: model.Matrix{{
		Metric: model.Metric{"__name__": "openshift_ptp_offset_ns"},
		Values: samplesAt(base, 0, 1, -1, 2, 0, -2),
	}}}

	err := metrics.StabilityOverTime[metrics.PtpClockClass](
		context.TODO(), client, metrics.ClockClassQuery{}, base, base.Add(6*time.Second), 2, 100)
	require.NoError(t, err)
}

func TestStabilityOverTime_NilClient_Errors(t *testing.T) {
	err := metrics.StabilityOverTime[metrics.PtpClockClass](
		context.TODO(), nil, metrics.ClockClassQuery{}, time.Now(), time.Now(), 0, 100)
	require.Error(t, err)
}
