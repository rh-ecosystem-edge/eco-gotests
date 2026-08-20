// Package client translates pure PTP domain values (clock.Clock, PtpClusterSnapshot) into the
// K8s/Prometheus API calls needed to observe and mutate them. cluster stays the functional core
// (discovers topology, no I/O of its own); client is the imperative shell around it -- mirrors
// metrics.ExecuteQuery's own shape (domain description + live client kept separate) and
// sigs.k8s.io/cluster-api/test/framework.ClusterProxy's own role.
package client

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/eventmetric"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
)

// PtpClient holds the live connections used to discover, observe, and mutate PTP cluster state.
// It's the translation layer between the OCP API & the PTP domain model.
type PtpClient struct {
	client        *clients.Settings
	prometheusAPI prometheusv1.API
}

var ptpClient *PtpClient

// Init registers the client's own K8s/Prometheus connections, once per suite run.
func Init(k8sClient *clients.Settings, prometheusAPI prometheusv1.API) {
	ptpClient = &PtpClient{client: k8sClient, prometheusAPI: prometheusAPI}
}

// fetchPtpResources fetches every cluster resource (CRs) which is relevant to PTP on OCP.
// (Node, PtpOperatorConfig, PtpConfig, HardwareConfig...).
func (client *PtpClient) fetchPtpResources() (cluster.Resources, error) {
	nodeList, err := nodes.List(client.client)
	if err != nil {
		return cluster.Resources{}, fmt.Errorf("failed to list Nodes: %w", err)
	}

	ptpConfigList, err := ptp.ListPtpConfigs(client.client)
	if err != nil {
		return cluster.Resources{}, fmt.Errorf("failed to list PtpConfigs: %w", err)
	}

	hwConfigList, err := ptp.ListHardwareConfigs(client.client)
	if err != nil {
		return cluster.Resources{}, fmt.Errorf("failed to list HardwareConfigs: %w", err)
	}

	return cluster.Resources{Nodes: nodeList, PtpConfigs: ptpConfigList, HwConfigs: hwConfigList}, nil
}

// DeriveAllClusterClocks derives all the PTP clocks which exist on the cluster.
func DeriveAllClusterClocks() (matches clock.Clocks, err error) {
	if ptpClient == nil {
		return nil, fmt.Errorf("client.Init was never called")
	}

	resources, err := ptpClient.fetchPtpResources()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PTP resources: %w", err)
	}

	clks, err := cluster.DeriveClocksFromResources(resources)
	if err != nil {
		return nil, fmt.Errorf("failed to build PTP nodes and clocks: %w", err)
	}

	return clks, nil
}

// WaitForLockedClocks asserts every clock in the cluster is currently LOCKED.
func WaitForLockedClocks() error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	clks, err := DeriveAllClusterClocks()
	if err != nil {
		return fmt.Errorf("failed to derive cluster clocks: %w", err)
	}

	since := time.Now()

	for _, clk := range clks {
		if err := WaitForLocked(clk, since, 5*time.Minute); err != nil {
			return fmt.Errorf("clock on node %s failed to reach locked: %w", clk.NodeName, err)
		}
	}

	return nil
}

// NewPtpClusterSnapshot creates a new PtpClusterSnapshot instance.
func NewPtpClusterSnapshot(
	clks clock.Clocks, ptpConfigs profiles.PtpConfigSnapshot, hwConfigs profiles.HardwareConfigSnapshot,
) *PtpClusterSnapshot {
	return &PtpClusterSnapshot{
		clocks:     clks,
		ptpConfigs: ptpConfigs,
		hwConfigs:  hwConfigs,
	}
}

// PtpClusterSnapshot is a saved copy of everything a holdover test can mutate: every PtpConfig and
// HardwareConfig in the cluster, and the given clocks' own upstream interfaces.
type PtpClusterSnapshot struct {
	clocks     clock.Clocks
	ptpConfigs profiles.PtpConfigSnapshot
	hwConfigs  profiles.HardwareConfigSnapshot
}

// Snapshot captures every PtpConfig and HardwareConfig in the cluster, and the given clocks, ready to
// be restored regardless of what a test changes.
func Snapshot() (*PtpClusterSnapshot, error) {
	if ptpClient == nil {
		return nil, fmt.Errorf("client.Init was never called")
	}

	clks, err := DeriveAllClusterClocks()
	if err != nil {
		return nil, err
	}

	ptpConfigs, err := profiles.SavePtpConfigs(ptpClient.client)
	if err != nil {
		return nil, fmt.Errorf("failed to save PtpConfigs: %w", err)
	}

	hwConfigs, err := profiles.SaveHardwareConfigs(ptpClient.client)
	if err != nil {
		return nil, fmt.Errorf("failed to save HardwareConfigs: %w", err)
	}

	snapshot := NewPtpClusterSnapshot(clks, ptpConfigs, hwConfigs)

	return snapshot, nil
}

// Restore reverts every PtpConfig and HardwareConfig to what the snapshot captured -- no diffing, the
// K8s API overwrites the current spec with what's sent -- and unconditionally brings every one of the
// snapshot's own clocks' upstream interfaces back up.
func Restore(snapshot *PtpClusterSnapshot) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	for _, clk := range snapshot.clocks {
		if err := SetUpstreamInterfaces(clk, iface.InterfaceStateUp); err != nil {
			return fmt.Errorf("failed to restore upstream interfaces on node %s: %w", clk.NodeName, err)
		}
	}

	if _, err := profiles.RestorePtpConfigs(ptpClient.client, snapshot.ptpConfigs); err != nil {
		return fmt.Errorf("failed to restore PtpConfigs: %w", err)
	}

	if _, err := profiles.RestoreHardwareConfigs(ptpClient.client, snapshot.hwConfigs); err != nil {
		return fmt.Errorf("failed to restore HardwareConfigs: %w", err)
	}

	return nil
}

// ChangeHoldoverParameters applies the given holdover parameters to the clock's own receiver profile.
func ChangeHoldoverParameters(clk *clock.Clock, desired clock.HoldoverParameters, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	ref := clk.ReceivingProfileRef

	settings := profiles.HoldoverPluginSettings{
		LocalHoldoverTimeout:   desired.LocalHoldoverTimeout,
		LocalMaxHoldoverOffSet: desired.LocalMaxHoldoverOffSet,
		MaxInSpecOffset:        desired.MaxInSpecOffset,
	}

	switch clk.HoldoverSource {
	case clock.HoldoverSourcePlugin:
		ptpConfig, err := ptp.PullPtpConfig(ptpClient.client, ref.Name.Name, ref.Name.Namespace)
		if err != nil {
			return fmt.Errorf("failed to pull PtpConfig %s: %w", ref.Name, err)
		}

		if err := profiles.SetHoldoverPluginSettings(
			&ptpConfig.Definition.Spec.Profile[ref.ProfileIndex],
			settings); err != nil {
			return fmt.Errorf("failed to set holdover plugin settings on PtpConfig %s: %w", ref.Name, err)
		}

		if _, err := ptpConfig.Update(); err != nil {
			return fmt.Errorf("failed to update PtpConfig %s: %w", ref.Name, err)
		}

		return nil
	case clock.HoldoverSourceHardwareConfig:
		hwConfigRef := clk.HoldoverHardwareConfigRef.Name

		hwConfig, err := ptp.PullHardwareConfig(ptpClient.client, hwConfigRef.Name, hwConfigRef.Namespace)
		if err != nil {
			return fmt.Errorf("failed to pull HardwareConfig %s: %w", hwConfigRef, err)
		}

		if err := profiles.SetHoldoverHardwareConfigSettings(hwConfig, settings); err != nil {
			return fmt.Errorf("failed to set holdover hardware config settings on HardwareConfig %s: %w", hwConfigRef, err)
		}

		return nil
	case clock.HoldoverSourceNone:
		return fmt.Errorf("clock %s has no holdover source", clk.NodeName)
	default:
		panic(fmt.Errorf("clock %s has an undefined holldover resource", clk.NodeName))
	}
}

// InterfaceStatusSetter is the capability SetUpstreamInterfaces needs -- a seam so a test doesn't need
// real pod exec against a PTP daemon pod, mirroring kubectl's own RemoteExecutor (pkg/cmd/exec/exec.go).
// Var, not const, so tests can substitute a fake.
var InterfaceStatusSetter = iface.SetInterfaceStatus

// SetUpstreamInterfaces sets every one of the clock's own upstream (time-receiver) interfaces to the given state.
func SetUpstreamInterfaces(clk *clock.Clock, status iface.InterfaceState) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	for _, receiverIface := range clk.TimeReceiverIfaces() {
		if err := InterfaceStatusSetter(ptpClient.client, clk.NodeName, receiverIface.Name, status); err != nil {
			return fmt.Errorf("failed to set interface %s to %s on node %s: %w",
				receiverIface.Name, status, clk.NodeName, err)
		}
	}

	return nil
}

// ClockClassSettleDuration bounds how long to wait, after a WaitFor* call's own event+metric assertion
// first succeeds, before sampling the clock class for a no-flip-flop check. Var, not const, so tests can
// shrink it.
var ClockClassSettleDuration = 5 * time.Second

// confirmClockClassStable waits ClockClassSettleDuration, then verifies clk's own clock-class metric
// held with no flip-flop for that whole, now-elapsed window.
func confirmClockClassStable(ctx context.Context, clk *clock.Clock) error {
	reachedAt := time.Now()

	select {
	case <-time.After(ClockClassSettleDuration):
	case <-ctx.Done():
		return ctx.Err()
	}

	settledAt := time.Now()

	return metrics.StabilityOverTime(ctx, ptpClient.prometheusAPI,
		metrics.ClockClassQuery{Node: metrics.Equals(clk.NodeName)}, reachedAt, settledAt, 0, 100)
}

// WaitForLocked verifies the clock reached LOCKED, at the clock class its own ExpectedClockClass
// predicts, and holds it with no flip-flop for ClockClassSettleDuration. If the clock is
// holdover-enabled, it also verifies the DPLL reported LOCKED_HO_ACQ, not just LOCKED.
func WaitForLocked(clk *clock.Clock, since time.Time, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	ctx := context.TODO()

	if err := lockedWaitStrategyFor(clk).wait(ctx, ptpClient.prometheusAPI, ptpClient.client, clk, since, timeout); err != nil {
		return err
	}

	return confirmClockClassStable(ctx, clk)
}

// WaitForHoldover verifies the clock reached HOLDOVER-IN-SPEC, at the clock class its own
// ExpectedClockClass predicts, and holds it with no flip-flop for ClockClassSettleDuration.
func WaitForHoldover(clk *clock.Clock, since time.Time, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	ctx := context.TODO()

	err := eventmetric.NewAssertion(
		ptpClient.prometheusAPI,
		metrics.ClockClassQuery{Node: metrics.Equals(clk.NodeName)},
		clk.ExpectedClockClass(clock.ClockClassPhaseHoldoverInSpec),
		events.All(
			events.IsType(eventptp.PtpStateChange),
			events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnNode(clk.NodeName)),
		),
	).ForNode(ptpClient.client, clk.NodeName).WithStartTime(since).WithTimeout(timeout).
		ExecuteAssertion(ctx)
	if err != nil {
		return err
	}

	return confirmClockClassStable(ctx, clk)
}

// WaitForHoldoverOutOfSpec verifies the clock reached HOLDOVER-OUT-OF-SPEC, at the clock class its own
// ExpectedClockClass predicts, and holds it with no flip-flop for ClockClassSettleDuration. Sync state
// stays HOLDOVER for this transition -- only clock class changes.
func WaitForHoldoverOutOfSpec(clk *clock.Clock, since time.Time, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	ctx := context.TODO()
	expectedClass := clk.ExpectedClockClass(clock.ClockClassPhaseHoldoverOutOfSpec)

	err := eventmetric.NewAssertion(
		ptpClient.prometheusAPI,
		metrics.ClockClassQuery{Node: metrics.Equals(clk.NodeName)},
		expectedClass,
		events.All(
			events.IsType(eventptp.PtpClockClassChange),
			events.HasValue(events.WithMetric(int64(expectedClass)), events.OnNode(clk.NodeName)),
		),
	).ForNode(ptpClient.client, clk.NodeName).WithStartTime(since).WithTimeout(timeout).
		ExecuteAssertion(ctx)
	if err != nil {
		return err
	}

	return confirmClockClassStable(ctx, clk)
}

// WaitForFreerun verifies the clock reached FREERUN, at the clock class its own ExpectedClockClass
// predicts, and holds it with no flip-flop for ClockClassSettleDuration.
func WaitForFreerun(clk *clock.Clock, since time.Time, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	ctx := context.TODO()

	err := eventmetric.NewAssertion(
		ptpClient.prometheusAPI,
		metrics.ClockClassQuery{Node: metrics.Equals(clk.NodeName)},
		clk.ExpectedClockClass(clock.ClockClassPhaseFreerun),
		events.All(
			events.IsType(eventptp.PtpStateChange),
			events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnNode(clk.NodeName)),
		),
	).ForNode(ptpClient.client, clk.NodeName).WithStartTime(since).WithTimeout(timeout).
		ExecuteAssertion(ctx)
	if err != nil {
		return err
	}

	return confirmClockClassStable(ctx, clk)
}

// WaitForLockedFromHoldover verifies the clock transitions directly from HOLDOVER to LOCKED -- any other
// sync state observed in between (e.g. FREERUN) fails the wait, catching a bracketed flip-flop that
// WaitForLocked's own "eventually LOCKED" check alone would miss.
func WaitForLockedFromHoldover(clk *clock.Clock, since time.Time, timeout time.Duration) error {
	if ptpClient == nil {
		return fmt.Errorf("client.Init was never called")
	}

	eventsEnabled, err := consumer.AreEventsEnabled(ptpClient.client)
	if err != nil {
		return fmt.Errorf("failed to check if events are enabled: %w", err)
	}

	if eventsEnabled {
		eventPod, err := consumer.GetConsumerPodforNode(ptpClient.client, clk.NodeName)
		if err != nil {
			return fmt.Errorf("failed to get consumer pod for node %s: %w", clk.NodeName, err)
		}

		filter := events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.OnNode(clk.NodeName)))

		if _, err := events.WaitForEventTransitioned(eventPod, since, timeout, filter,
			events.WithSyncState(eventptp.HOLDOVER), events.WithSyncState(eventptp.LOCKED)); err != nil {
			return fmt.Errorf("clock %s did not transition cleanly from HOLDOVER to LOCKED: %w", clk.NodeName, err)
		}
	}

	return WaitForLocked(clk, since, timeout)
}
