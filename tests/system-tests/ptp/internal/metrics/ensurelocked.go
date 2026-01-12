package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// EnsureClocksAreLocked ensures that all PTP clocks are locked across all nodes covered by the Prometheus API client.
// It is designed to be used as a BeforeEach/AfterEach check to ensure the cluster is in a stable state.
//
// It ensures that clocks are locked for 10 seconds with a timeout of 5 minutes. It does not check the clock state of
// the chronyd process, as it will be FREERUN when PTP is working correctly.
func EnsureClocksAreLocked(prometheusAPI prometheusv1.API) error {
	query := ClockStateQuery{
		Process: DoesNotEqual(ProcessChronyd),
	}

	err := AssertQuery(context.TODO(), prometheusAPI, query, ClockStateLocked,
		AssertWithStableDuration(10*time.Second),
		AssertWithTimeout(5*time.Minute))
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked: %w", err)
	}

	return nil
}
