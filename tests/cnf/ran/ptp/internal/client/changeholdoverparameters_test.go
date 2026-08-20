//go:build unit_test

package client_test

import (
	"testing"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/client"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/stretchr/testify/require"
)

func TestChangeHoldoverParameters_TTSC_WPC_SingleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTTSCWpcSingleNICHoldoverCapable.yaml")
	fakeClient := initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	desired := clock.HoldoverParameters{
		LocalHoldoverTimeout:   360,
		LocalMaxHoldoverOffSet: 14400,
		MaxInSpecOffset:        1800,
	}
	require.NoError(t, client.ChangeHoldoverParameters(clks[0], desired, time.Minute))

	updated, err := ptp.PullPtpConfig(fakeClient, ptpConfig.Name, ptpConfig.Namespace)
	require.NoError(t, err)

	assertGoldenPtpConfigSpec(t, "PtpConfigTTSCWpcSingleNICHoldoverCapableGolden.yaml", updated.Definition)
}

// Proves ReceivingProfileRef resolves to the receiver profile (tbc-tr), not the transmitter's.
func TestChangeHoldoverParameters_TBC_WPC_SingleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCWpcSingleNICHoldoverCapable.yaml")
	fakeClient := initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	desired := clock.HoldoverParameters{
		LocalHoldoverTimeout:   720,
		LocalMaxHoldoverOffSet: 28800,
		MaxInSpecOffset:        28801,
	}
	require.NoError(t, client.ChangeHoldoverParameters(clks[0], desired, time.Minute))

	updated, err := ptp.PullPtpConfig(fakeClient, ptpConfig.Name, ptpConfig.Namespace)
	require.NoError(t, err)

	assertGoldenPtpConfigSpec(t, "PtpConfigTBCWpcSingleNICHoldoverCapableGolden.yaml", updated.Definition)
}

// Proves ProfileIndex resolves by real position -- the receiver profile here is index 1, not 0.
func TestChangeHoldoverParameters_TBC_WPC_TripleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCWpcTripleNICHoldoverCapable.yaml")
	fakeClient := initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	desired := clock.HoldoverParameters{
		LocalHoldoverTimeout:   180,
		LocalMaxHoldoverOffSet: 7200,
		MaxInSpecOffset:        50,
	}
	require.NoError(t, client.ChangeHoldoverParameters(clks[0], desired, time.Minute))

	updated, err := ptp.PullPtpConfig(fakeClient, ptpConfig.Name, ptpConfig.Namespace)
	require.NoError(t, err)

	assertGoldenPtpConfigSpec(t, "PtpConfigTBCWpcTripleNICHoldoverCapableGolden.yaml", updated.Definition)
}

// Proves ChangeHoldoverParameters mutates the HardwareConfig CR, not a PtpConfig plugin.
func TestChangeHoldoverParameters_TBC_GNRD_NACDualCarterFlat_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	hwConfig := loadHardwareConfig(t, "HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	fakeClient := initFakePtpClientPlainTracker(t, masterNode(), ptpConfig, hwConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	desired := clock.HoldoverParameters{
		LocalHoldoverTimeout:   7200,
		LocalMaxHoldoverOffSet: 750,
		MaxInSpecOffset:        50,
	}
	require.NoError(t, client.ChangeHoldoverParameters(clks[0], desired, time.Minute))

	updated, err := ptp.PullHardwareConfig(fakeClient, hwConfig.Name, hwConfig.Namespace)
	require.NoError(t, err)

	assertGoldenHardwareConfigSpec(t, "HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapableGolden.yaml", updated.Definition)
}

// No HoldoverSource exists (plugin-only, no HardwareConfig) -- proves ChangeHoldoverParameters errors
// instead of silently no-op'ing or mutating the wrong thing.
func TestChangeHoldoverParameters_TBC_GNRD_NACDualCarterFlat_HoldoverIncapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverIncapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	desired := clock.HoldoverParameters{
		LocalHoldoverTimeout:   7200,
		LocalMaxHoldoverOffSet: 750,
		MaxInSpecOffset:        50,
	}
	require.Error(t, client.ChangeHoldoverParameters(clks[0], desired, time.Minute))
}
