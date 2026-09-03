package metrics

// Run unit tests with:
// UNIT_TEST=true go test ./tests/cnf/ran/ptp/internal/metrics/...

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePrometheusAPI struct {
	mu sync.Mutex

	queryCalls []queryCall
	values     map[string]model.SampleValue
	// tickHandler, when set, is called at the start of each poll tick (before per-query responses).
	// It receives the number of completed ticks so far and returns values keyed by metric name substring.
	tickHandler func(tick int) map[string]model.SampleValue
	tick        int
}

type queryCall struct {
	query string
	ts    time.Time
}

func (fake *fakePrometheusAPI) Query(
	_ context.Context, query string, ts time.Time, _ ...prometheusv1.Option,
) (model.Value, prometheusv1.Warnings, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.queryCalls) == 0 || !fake.queryCalls[len(fake.queryCalls)-1].ts.Equal(ts) {
		fake.tick++
		if fake.tickHandler != nil {
			fake.values = fake.tickHandler(fake.tick)
		}
	}

	fake.queryCalls = append(fake.queryCalls, queryCall{query: query, ts: ts})

	value, ok := fake.valueForQuery(query)
	if !ok {
		return model.Vector{}, nil, nil
	}

	return model.Vector{
		&model.Sample{
			Metric: model.Metric{},
			Value:  value,
		},
	}, nil, nil
}

func (fake *fakePrometheusAPI) valueForQuery(query string) (model.SampleValue, bool) {
	for metric, value := range fake.values {
		if strings.Contains(query, metric) {
			return value, true
		}
	}

	return 0, false
}

func (fake *fakePrometheusAPI) Alerts(context.Context) (prometheusv1.AlertsResult, error) {
	panic("unexpected Alerts call")
}

func (fake *fakePrometheusAPI) AlertManagers(context.Context) (prometheusv1.AlertManagersResult, error) {
	panic("unexpected AlertManagers call")
}

func (fake *fakePrometheusAPI) CleanTombstones(context.Context) error {
	panic("unexpected CleanTombstones call")
}

func (fake *fakePrometheusAPI) Config(context.Context) (prometheusv1.ConfigResult, error) {
	panic("unexpected Config call")
}

func (fake *fakePrometheusAPI) DeleteSeries(context.Context, []string, time.Time, time.Time) error {
	panic("unexpected DeleteSeries call")
}

func (fake *fakePrometheusAPI) Flags(context.Context) (prometheusv1.FlagsResult, error) {
	panic("unexpected Flags call")
}

func (fake *fakePrometheusAPI) LabelNames(
	context.Context, []string, time.Time, time.Time, ...prometheusv1.Option,
) ([]string, prometheusv1.Warnings, error) {
	panic("unexpected LabelNames call")
}

func (fake *fakePrometheusAPI) LabelValues(
	context.Context, string, []string, time.Time, time.Time, ...prometheusv1.Option,
) (model.LabelValues, prometheusv1.Warnings, error) {
	panic("unexpected LabelValues call")
}

func (fake *fakePrometheusAPI) QueryRange(
	context.Context, string, prometheusv1.Range, ...prometheusv1.Option,
) (model.Value, prometheusv1.Warnings, error) {
	panic("unexpected QueryRange call")
}

func (fake *fakePrometheusAPI) QueryExemplars(
	context.Context, string, time.Time, time.Time,
) ([]prometheusv1.ExemplarQueryResult, error) {
	panic("unexpected QueryExemplars call")
}

func (fake *fakePrometheusAPI) Buildinfo(context.Context) (prometheusv1.BuildinfoResult, error) {
	panic("unexpected Buildinfo call")
}

func (fake *fakePrometheusAPI) Runtimeinfo(context.Context) (prometheusv1.RuntimeinfoResult, error) {
	panic("unexpected Runtimeinfo call")
}

func (fake *fakePrometheusAPI) Series(
	context.Context, []string, time.Time, time.Time, ...prometheusv1.Option,
) ([]model.LabelSet, prometheusv1.Warnings, error) {
	panic("unexpected Series call")
}

func (fake *fakePrometheusAPI) Snapshot(context.Context, bool) (prometheusv1.SnapshotResult, error) {
	panic("unexpected Snapshot call")
}

func (fake *fakePrometheusAPI) Rules(context.Context) (prometheusv1.RulesResult, error) {
	panic("unexpected Rules call")
}

func (fake *fakePrometheusAPI) Targets(context.Context) (prometheusv1.TargetsResult, error) {
	panic("unexpected Targets call")
}

func (fake *fakePrometheusAPI) TargetsMetadata(context.Context, string, string, string) ([]prometheusv1.MetricMetadata, error) {
	panic("unexpected TargetsMetadata call")
}

func (fake *fakePrometheusAPI) Metadata(context.Context, string, string) (map[string][]prometheusv1.Metadata, error) {
	panic("unexpected Metadata call")
}

func (fake *fakePrometheusAPI) TSDB(context.Context, ...prometheusv1.Option) (prometheusv1.TSDBResult, error) {
	panic("unexpected TSDB call")
}

func (fake *fakePrometheusAPI) WalReplay(context.Context) (prometheusv1.WalReplayStatus, error) {
	panic("unexpected WalReplay call")
}

func clockStateQueryForNode(node string) ClockStateQuery {
	return ClockStateQuery{
		Node:    Equals(node),
		Process: Equals(ProcessTBC),
	}
}

func clockClassQueryForNode(node string) ClockClassQuery {
	return ClockClassQuery{
		Node:    Equals(node),
		Process: Equals(ProcessPTP4L),
	}
}

func TestAssertQuerySetEmptyExpectations(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{}
	err := AssertQuerySet(context.Background(), fake, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no expectations")
}

func TestAssertQuerySetNilClient(t *testing.T) {
	t.Parallel()

	err := AssertQuerySet(
		context.Background(),
		nil,
		[]QueryExpectation{Expect(clockStateQueryForNode("worker-0"), ClockStateLocked)},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil client")
}

func TestAssertQuerySetUsesSameTimestampPerTick(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateLocked),
			string(MetricClockClass): model.SampleValue(ClockClass6),
		},
	}

	err := AssertQuerySet(
		context.Background(),
		fake,
		[]QueryExpectation{
			Expect(clockStateQueryForNode("worker-0"), ClockStateLocked),
			Expect(clockClassQueryForNode("worker-0"), ClockClass6),
		},
	)
	require.NoError(t, err)
	require.Len(t, fake.queryCalls, 2)

	assert.Equal(t, fake.queryCalls[0].ts, fake.queryCalls[1].ts)
}

func TestAssertQueriesAtTimeReturnsFirstFailure(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateLocked),
			string(MetricClockClass): model.SampleValue(ClockClass7),
		},
	}

	err := assertQueriesAtTime(
		context.Background(),
		fake,
		[]QueryExpectation{
			Expect(clockStateQueryForNode("worker-0"), ClockStateLocked),
			Expect(clockClassQueryForNode("worker-0"), ClockClass6),
		},
		time.Now(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expectation")
}

func TestAssertQuerySetFailsWhenAnyExpectationFails(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateLocked),
			string(MetricClockClass): model.SampleValue(ClockClass7),
		},
	}

	err := AssertQuerySet(
		context.Background(),
		fake,
		[]QueryExpectation{
			Expect(clockStateQueryForNode("worker-0"), ClockStateLocked),
			Expect(clockClassQueryForNode("worker-0"), ClockClass6),
		},
		AssertWithTimeout(10*time.Millisecond),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestAssertQuerySetStableDuration(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		tickHandler: func(tick int) map[string]model.SampleValue {
			if tick == 1 {
				return map[string]model.SampleValue{
					string(MetricClockState): model.SampleValue(ClockStateFreerun),
					string(MetricClockClass): model.SampleValue(ClockClass6),
				}
			}

			return map[string]model.SampleValue{
				string(MetricClockState): model.SampleValue(ClockStateLocked),
				string(MetricClockClass): model.SampleValue(ClockClass6),
			}
		},
	}

	err := AssertQuerySet(
		context.Background(),
		fake,
		[]QueryExpectation{
			Expect(clockStateQueryForNode("worker-0"), ClockStateLocked),
			Expect(clockClassQueryForNode("worker-0"), ClockClass6),
		},
		AssertWithStableDuration(30*time.Millisecond),
		AssertWithPollInterval(40*time.Millisecond),
		AssertWithTimeout(500*time.Millisecond),
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fake.tick, 2)
}

func TestAssertQuerySetStableDurationResetsOnFailure(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		tickHandler: func(tick int) map[string]model.SampleValue {
			clockState := ClockStateLocked
			if tick%2 == 0 {
				clockState = ClockStateFreerun
			}

			return map[string]model.SampleValue{
				string(MetricClockState): model.SampleValue(clockState),
				string(MetricClockClass): model.SampleValue(ClockClass6),
			}
		},
	}

	err := AssertQuerySet(
		context.Background(),
		fake,
		[]QueryExpectation{
			Expect(clockStateQueryForNode("worker-0"), ClockStateLocked),
			Expect(clockClassQueryForNode("worker-0"), ClockClass6),
		},
		AssertWithStableDuration(50*time.Millisecond),
		AssertWithPollInterval(20*time.Millisecond),
		AssertWithTimeout(200*time.Millisecond),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestAssertQueryDelegatesToPollLoop(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateLocked),
		},
	}

	err := AssertQuery(
		context.Background(),
		fake,
		clockStateQueryForNode("worker-0"),
		ClockStateLocked,
	)
	require.NoError(t, err)
	require.Len(t, fake.queryCalls, 1)
}

func TestAssertQueryPreservesTimeoutErrorMessage(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateFreerun),
		},
	}

	err := AssertQuery(
		context.Background(),
		fake,
		clockStateQueryForNode("worker-0"),
		ClockStateLocked,
		AssertWithTimeout(10*time.Millisecond),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to assert query eventually")
}

func TestLockedExpectationsFromExpectedFailsOnMissingSeries(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{},
	}

	expectations := LockedExpectationsFromExpected([]ExpectedClockState{
		{
			Process:   ProcessPTP4L,
			Interface: "ens1f0",
			Node:      "worker-0",
		},
	})

	err := assertQueriesAtTime(
		context.Background(),
		fake,
		expectations,
		time.Now(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no samples returned")
}

func TestLockedExpectationsFromExpectedPassesWhenPresent(t *testing.T) {
	t.Parallel()

	fake := &fakePrometheusAPI{
		values: map[string]model.SampleValue{
			string(MetricClockState): model.SampleValue(ClockStateLocked),
		},
	}

	expectations := LockedExpectationsFromExpected([]ExpectedClockState{
		{
			Process:   ProcessPTP4L,
			Interface: "ens1f0",
			Node:      "worker-0",
		},
	})

	err := AssertQuerySet(context.Background(), fake, expectations)
	require.NoError(t, err)
}
