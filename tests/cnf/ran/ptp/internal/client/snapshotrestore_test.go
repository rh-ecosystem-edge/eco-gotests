//go:build unit_test

package client_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/client"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/stretchr/testify/require"
)

// interfaceStatusCall records one InterfaceStatusSetter invocation.
type interfaceStatusCall struct {
	nodeName  string
	ifaceName iface.Name
	status    iface.InterfaceState
}

// fakeInterfaceStatusSetter installs a no-op InterfaceStatusSetter for the duration of the test, so
// Restore's own SetUpstreamInterfaces call doesn't need a real PTP daemon pod.
func fakeInterfaceStatusSetter(t *testing.T) *[]interfaceStatusCall {
	t.Helper()

	var calls []interfaceStatusCall

	original := client.InterfaceStatusSetter
	t.Cleanup(func() { client.InterfaceStatusSetter = original })

	client.InterfaceStatusSetter = func(
		_ *clients.Settings, nodeName string, ifaceName iface.Name, status iface.InterfaceState,
	) error {
		calls = append(calls, interfaceStatusCall{nodeName: nodeName, ifaceName: ifaceName, status: status})

		return nil
	}

	return &calls
}

// TestSnapshotAndRestore_MultiClock_TTSCAndTBC covers a cluster with two distinct clocks discovered on
// the same node: T-TSC WPC single-card (standalone profile) and T-BC GNR-D multi-card (paired
// transmitter+receiver, HardwareConfig-backed holdover) -- the same fixtures already used by
// TestDeriveAllClusterClocks_TTSC_WPC_SingleNIC_HoldoverCapable and
// TestDeriveAllClusterClocks_TBC_GNRD_NACDualCarterFlat_HoldoverCapable.
func TestSnapshotAndRestore_MultiClock_TTSCAndTBC(t *testing.T) {
	ttscConfig := loadPtpConfig(t, "PtpConfigTTSCWpcSingleNICHoldoverCapable.yaml")
	tbcConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	hwConfig := loadHardwareConfig(t, "HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")

	initFakePtpClient(t, masterNode(), ttscConfig, tbcConfig, hwConfig)

	snapshot, err := client.Snapshot()
	require.NoError(t, err)

	calls := fakeInterfaceStatusSetter(t)

	require.NoError(t, client.Restore(snapshot))

	require.Len(t, *calls, 2, "expected one upstream-interface restore per clock (T-TSC + T-BC)")

	calledIfaces := make(map[iface.Name]bool, len(*calls))

	for _, call := range *calls {
		require.Equal(t, "master-0", call.nodeName)
		require.Equal(t, iface.InterfaceStateUp, call.status)

		calledIfaces[call.ifaceName] = true
	}

	require.Len(t, calledIfaces, 2, "expected two distinct receiver interfaces, one per clock")
}

// TestSnapshotAndRestore_MultiClock_RevertsHoldoverParameters is RED against today's client.Restore: it
// only reverts upstream interfaces, never the PtpConfig/HardwareConfig content ChangeHoldoverParameters
// mutated. Snapshot captures the original PtpConfig/HardwareConfig specs before mutation; Restore is
// expected to write them back, for both a plugin-backed clock (T-TSC) and a HardwareConfig-backed one
// (T-BC GNR-D) in the same restore call.
func TestSnapshotAndRestore_MultiClock_RevertsHoldoverParameters(t *testing.T) {
	ttscConfig := loadPtpConfig(t, "PtpConfigTTSCWpcSingleNICHoldoverCapable.yaml")
	tbcConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	hwConfig := loadHardwareConfig(t, "HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")

	fakeClient := initFakePtpClientPlainTracker(t, masterNode(), ttscConfig, tbcConfig, hwConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 2)

	ttscClks, ok := clks.OfType(clock.ClockTypeTTSC)
	require.True(t, ok)
	tbcClks, ok := clks.OfType(clock.ClockTypeTBC)
	require.True(t, ok)

	originalPtpConfig, err := ptp.PullPtpConfig(fakeClient, ttscConfig.Name, ttscConfig.Namespace)
	require.NoError(t, err)
	originalHwConfig, err := ptp.PullHardwareConfig(fakeClient, hwConfig.Name, hwConfig.Namespace)
	require.NoError(t, err)

	// Snapshot before any mutation -- the point being tested is whether Restore can get back to this.
	snapshot, err := client.Snapshot()
	require.NoError(t, err)

	mutated := clock.HoldoverParameters{LocalHoldoverTimeout: 9999, LocalMaxHoldoverOffSet: 9999, MaxInSpecOffset: 9999}
	require.NoError(t, client.ChangeHoldoverParameters(ttscClks[0], mutated, time.Minute))
	require.NoError(t, client.ChangeHoldoverParameters(tbcClks[0], mutated, time.Minute))

	// Sanity: confirm the mutation actually took effect before asking whether Restore can undo it.
	afterMutatePtpConfig, err := ptp.PullPtpConfig(fakeClient, ttscConfig.Name, ttscConfig.Namespace)
	require.NoError(t, err)
	require.NotEmpty(t, cmp.Diff(originalPtpConfig.Definition.Spec, afterMutatePtpConfig.Definition.Spec, pluginJSONAsMap),
		"ChangeHoldoverParameters should have changed the PtpConfig spec")

	afterMutateHwConfig, err := ptp.PullHardwareConfig(fakeClient, hwConfig.Name, hwConfig.Namespace)
	require.NoError(t, err)
	require.NotEmpty(t, cmp.Diff(originalHwConfig.Definition.Spec, afterMutateHwConfig.Definition.Spec),
		"ChangeHoldoverParameters should have changed the HardwareConfig spec")

	fakeInterfaceStatusSetter(t)

	require.NoError(t, client.Restore(snapshot))

	restoredPtpConfig, err := ptp.PullPtpConfig(fakeClient, ttscConfig.Name, ttscConfig.Namespace)
	require.NoError(t, err)
	if diff := cmp.Diff(originalPtpConfig.Definition.Spec, restoredPtpConfig.Definition.Spec, pluginJSONAsMap); diff != "" {
		t.Errorf("PtpConfig spec not restored to its pre-mutation shape (-original +restored):\n%s", diff)
	}

	restoredHwConfig, err := ptp.PullHardwareConfig(fakeClient, hwConfig.Name, hwConfig.Namespace)
	require.NoError(t, err)
	if diff := cmp.Diff(originalHwConfig.Definition.Spec, restoredHwConfig.Definition.Spec); diff != "" {
		t.Errorf("HardwareConfig spec not restored to its pre-mutation shape (-original +restored):\n%s", diff)
	}
}
