package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	ginkgotypes "github.com/onsi/ginkgo/v2/types"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

// clockStateNames maps each PtpClockState value to a short, human-readable label for log output.
var clockStateNames = map[PtpClockState]string{
	ClockStateFreerun:  "FREERUN",
	ClockStateLocked:   "LOCKED",
	ClockStateHoldover: "HOLDOVER",
}

// clockStateRangeStep is the resolution used for the range query in DumpClockStateRangeIfFailed. It matches the PTP
// suite's own runtime override of the ServiceMonitor's scrape interval (see UpdatePtpServiceMonitorInterval, called
// from this package's BeforeSuite) -- a finer step would just repeat the last real sample, since Prometheus cannot
// report data it never scraped.
const clockStateRangeStep = time.Second

// DumpClockStateRangeIfFailed queries openshift_ptp_clock_state as a single Prometheus range query spanning the
// entire duration of the given (already-completed) spec, and logs every returned series as a timestamped history.
// It is a no-op unless the spec failed, and a best-effort helper: any error just gets logged, it never fails the
// calling test itself.
//
// This exists to make PTP clock-state investigations reproducible directly from the archived CI log, without needing
// live cluster/Thanos access after the fact. Today, a stuck-vs-flipping clock_state value during a failure can only
// be inferred indirectly, by cross-referencing the daemon's own raw process logs against the individual
// pass/fail lines AssertQuery already emits one instant-query at a time. A single query_range call covering the
// spec's own StartTime/EndTime gives the same information directly, as one readable artifact. See CNF-26457.
func DumpClockStateRangeIfFailed(specReport ginkgotypes.SpecReport, prometheusAPI prometheusv1.API) {
	if !specReport.Failed() {
		return
	}

	if prometheusAPI == nil {
		klog.V(tsparams.LogLevel).Info("Skipping clock state range dump: no Prometheus API client available")

		return
	}

	rangeQuery := MetricQuery[PtpClockState]{
		Metric: MetricClockState,
		Start:  specReport.StartTime,
		End:    specReport.EndTime,
		Step:   clockStateRangeStep,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyProcess: Includes(ProcessPTP4L, ProcessPHC2SYS).ToAny(),
		},
	}

	matrix, err := ExecuteQueryRange(context.Background(), prometheusAPI, rangeQuery)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to dump clock state range for failed spec %q: %v", specReport.FullText(), err)

		return
	}

	if len(matrix) == 0 {
		klog.V(tsparams.LogLevel).Infof(
			"Clock state range dump for failed spec %q returned no series between %s and %s",
			specReport.FullText(), specReport.StartTime.Format(time.RFC3339), specReport.EndTime.Format(time.RFC3339))

		return
	}

	klog.V(tsparams.LogLevel).Info(formatClockStateMatrix(specReport, matrix))
}

// formatClockStateMatrix renders a clock_state range query result as a readable, timestamped, per-series history.
func formatClockStateMatrix(specReport ginkgotypes.SpecReport, matrix model.Matrix) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "Clock state history for failed spec %q (%s to %s):\n",
		specReport.FullText(), specReport.StartTime.Format(time.RFC3339), specReport.EndTime.Format(time.RFC3339))

	for _, stream := range matrix {
		fmt.Fprintf(&builder, "  %s\n", stream.Metric.String())

		for _, sample := range stream.Values {
			state := PtpClockState(int(sample.Value))

			name, ok := clockStateNames[state]
			if !ok {
				name = fmt.Sprintf("UNKNOWN(%s)", sample.Value.String())
			}

			fmt.Fprintf(&builder, "    %s => %s\n", sample.Timestamp.Time().Format(time.RFC3339), name)
		}
	}

	return builder.String()
}
