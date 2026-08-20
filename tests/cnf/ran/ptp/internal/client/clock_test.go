//go:build unit_test

package client_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/client"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// fakePrometheusAPI dispatches by metric name embedded in the query string, since one fake handles
// both clock-class and phase-status queries within the same test. clockClassAfter, if set, simulates
// the clock class having changed by the time QueryRange's own end of window is reached.
type fakePrometheusAPI struct {
	prometheusv1.API
	clockClass      metrics.PtpClockClass
	clockClassAfter *metrics.PtpClockClass
	phaseStatus     metrics.PtpPhaseStatus
}

func (f *fakePrometheusAPI) Query(
	_ context.Context, query string, _ time.Time, _ ...prometheusv1.Option,
) (model.Value, prometheusv1.Warnings, error) {
	switch {
	case strings.Contains(query, string(metrics.MetricClockClass)):
		return vectorWithValue(metrics.MetricClockClass, float64(f.clockClass)), nil, nil
	case strings.Contains(query, string(metrics.MetricPhaseStatus)):
		return vectorWithValue(metrics.MetricPhaseStatus, float64(f.phaseStatus)), nil, nil
	default:
		return nil, nil, fmt.Errorf("fakePrometheusAPI: unexpected query %q", query)
	}
}

// QueryRange returns a sample at r's own start holding f's configured clock class, and a sample at r's
// own end holding clockClassAfter if set (else the same clock class) -- simulating a real emitter whose
// value changed sometime between the start and end of the sampled window.
func (f *fakePrometheusAPI) QueryRange(
	_ context.Context, query string, r prometheusv1.Range, _ ...prometheusv1.Option,
) (model.Value, prometheusv1.Warnings, error) {
	if !strings.Contains(query, string(metrics.MetricClockClass)) {
		return nil, nil, fmt.Errorf("fakePrometheusAPI: unexpected range query %q", query)
	}

	endClass := f.clockClass
	if f.clockClassAfter != nil {
		endClass = *f.clockClassAfter
	}

	return model.Matrix{{
		Metric: model.Metric{"__name__": model.LabelValue(metrics.MetricClockClass)},
		Values: []model.SamplePair{
			{Timestamp: model.TimeFromUnix(r.Start.Unix()), Value: model.SampleValue(f.clockClass)},
			{Timestamp: model.TimeFromUnix(r.End.Unix()), Value: model.SampleValue(endClass)},
		},
	}}, nil, nil
}

func vectorWithValue(metric metrics.PtpMetric, value float64) model.Vector {
	return model.Vector{
		&model.Sample{
			Metric: model.Metric{"__name__": model.LabelValue(metric)},
			Value:  model.SampleValue(value),
		},
	}
}

// initFakeClusterNoEvents wires a fake cluster client with a PtpOperatorConfig that has no
// EventConfig -- consumer.AreEventsEnabled returns false, so eventmetric skips the consumer-pod/event
// path entirely. No httptest.Server needed for this increment.
func initFakeClusterNoEvents(t *testing.T) *clients.Settings {
	t.Helper()

	ptpOperatorConfig := &ptpv1.PtpOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-ptp"},
	}

	return clients.GetTestClients(clients.TestClientParams{
		K8sMockObjects:  []runtime.Object{ptpOperatorConfig},
		SchemeAttachers: []clients.SchemeAttacher{ptpv1.AddToScheme},
	})
}

func init() {
	// Real tests don't need to wait out the real settle window.
	client.ClockClassSettleDuration = time.Millisecond
}

func lockedTestClock(holdoverEnabled bool) *clock.Clock {
	return &clock.Clock{
		NodeName: "master-0",
		Type:     clock.ClockTypeTBC,
		Interfaces: map[iface.Name]*clock.Interface{
			"ens7f0np0": {Name: "ens7f0np0", Role: profiles.InterfaceRoleClient},
		},
		HoldoverEnabled: holdoverEnabled,
	}
}

func TestWaitForLocked_NoHoldover_PassesOnClockClassAlone(t *testing.T) {
	clk := lockedTestClock(false)

	clusterClient := initFakeClusterNoEvents(t)
	client.Init(clusterClient, &fakePrometheusAPI{clockClass: metrics.ClockClass6})

	require.NoError(t, client.WaitForLocked(clk, time.Now(), time.Second))
}

func TestWaitForLocked_Holdover_PhaseStatusAcquired_Passes(t *testing.T) {
	clk := lockedTestClock(true)

	clusterClient := initFakeClusterNoEvents(t)
	client.Init(clusterClient, &fakePrometheusAPI{
		clockClass:  metrics.ClockClass6,
		phaseStatus: metrics.PhaseStatusLockedHoldoverAcquired,
	})

	require.NoError(t, client.WaitForLocked(clk, time.Now(), time.Second))
}

func TestWaitForLocked_Holdover_PhaseStatusNotAcquired_Fails(t *testing.T) {
	clk := lockedTestClock(true)

	clusterClient := initFakeClusterNoEvents(t)
	client.Init(clusterClient, &fakePrometheusAPI{
		clockClass:  metrics.ClockClass6,
		phaseStatus: metrics.PhaseStatusLocked, // 2, not 3 -- the exact real risk
	})

	require.Error(t, client.WaitForLocked(clk, time.Now(), time.Second))
}

// TestWaitForLocked_ClockClassFlipsDuringSettle_Fails verifies confirmClockClassStable's own post-hoc
// check: the clock class is LOCKED (6) when first confirmed, then FREERUN's class (248) by the time the
// settle window's own end is sampled -- a real emitter change between the start and end of the sampled
// window, which WaitForLocked must still catch.
func TestWaitForLocked_ClockClassFlipsDuringSettle_Fails(t *testing.T) {
	clk := lockedTestClock(false)
	flippedTo := metrics.ClockClass248

	clusterClient := initFakeClusterNoEvents(t)
	client.Init(clusterClient, &fakePrometheusAPI{clockClass: metrics.ClockClass6, clockClassAfter: &flippedTo})

	err := client.WaitForLocked(clk, time.Now(), time.Second)

	var oscillation *metrics.OscillationError[metrics.PtpClockClass]
	require.ErrorAs(t, err, &oscillation)
}
