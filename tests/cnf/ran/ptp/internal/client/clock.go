package client

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/eventmetric"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

// lockedWaitStrategy dispatches how WaitForLocked waits for LOCKED, based on HoldoverEnabled.
type lockedWaitStrategy interface {
	wait(ctx context.Context, prometheusAPI prometheusv1.API, clusterClient *clients.Settings,
		clk *clock.Clock, since time.Time, timeout time.Duration) error
}

// lockedWaitStrategyFor dispatches on clk.HoldoverEnabled.
func lockedWaitStrategyFor(clk *clock.Clock) lockedWaitStrategy {
	if clk.HoldoverEnabled {
		return holdoverCapableLockedWait{}
	}

	return basicLockedWait{}
}

// basicLockedWait waits for the LOCKED event (if events are enabled on the cluster) and clock-class
// metric only -- no DPLL phase-status check, since a non-holdover clock has no holdover source to
// report one.
type basicLockedWait struct{}

func (basicLockedWait) wait(
	ctx context.Context, prometheusAPI prometheusv1.API, clusterClient *clients.Settings,
	clk *clock.Clock, since time.Time, timeout time.Duration,
) error {
	return eventmetric.NewAssertion(
		prometheusAPI,
		metrics.ClockClassQuery{Node: metrics.Equals(clk.NodeName)},
		clk.ExpectedClockClass(clock.ClockClassPhaseLocked),
		events.All(
			events.IsType(eventptp.PtpStateChange),
			events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnNode(clk.NodeName)),
		),
	).ForNode(clusterClient, clk.NodeName).WithStartTime(since).WithTimeout(timeout).ExecuteAssertion(ctx)
}

// holdoverCapableLockedWait also requires the DPLL to report LOCKED_HO_ACQ (3), confirming a
// holdover reference is ready, per https://issues.redhat.com/browse/CNF-26069.
type holdoverCapableLockedWait struct{}

func (holdoverCapableLockedWait) wait(
	ctx context.Context, prometheusAPI prometheusv1.API, clusterClient *clients.Settings,
	clk *clock.Clock, since time.Time, timeout time.Duration,
) error {
	if err := (basicLockedWait{}).wait(ctx, prometheusAPI, clusterClient, clk, since, timeout); err != nil {
		return err
	}

	receiverIface, timeRxIfaceOk := clk.TimeReceiverIface()
	if !timeRxIfaceOk {
		return fmt.Errorf("clock %s has no receiver interface", clk.NodeName)
	}

	return metrics.AssertQuery(ctx, prometheusAPI,
		metrics.PhaseStatusQuery{
			From:      metrics.Equals(metrics.ProcessDPLL),
			Process:   metrics.Equals(metrics.ProcessDPLL),
			Node:      metrics.Equals(clk.NodeName),
			Interface: metrics.Equals(receiverIface.Name.GetNIC()),
		},
		metrics.PhaseStatusLockedHoldoverAcquired,
		metrics.AssertWithStartTime(since), metrics.AssertWithTimeout(timeout))
}
