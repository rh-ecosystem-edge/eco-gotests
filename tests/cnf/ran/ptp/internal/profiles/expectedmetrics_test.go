//go:build unit_test

package profiles

import (
	"encoding/json"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func profileInfoWithClientIfaces(profileType PtpProfileType, clientIfaces ...iface.Name) *ProfileInfo {
	interfaces := make(map[iface.Name]*InterfaceInfo, len(clientIfaces))
	for _, ifName := range clientIfaces {
		interfaces[ifName] = &InterfaceInfo{
			Name:      ifName,
			ClockType: ClockTypeClient,
		}
	}

	return &ProfileInfo{
		ProfileType: profileType,
		Interfaces:  interfaces,
		Reference: ProfileReference{
			ProfileName: "test-profile",
		},
	}
}

func profileWithDpllSettings(t *testing.T) *ptpv1.PtpProfile {
	t.Helper()

	plugin := ptp.IntelPlugin{
		DpllSettings: map[string]uint64{
			"LocalHoldoverTimeout":   300,
			"LocalMaxHoldoverOffSet": 1000,
			"MaxInSpecOffset":        100,
		},
		Pins: map[string]map[string]string{
			"ens2f0": {"SMA1": "0 1", "SMA2": "1 2"},
		},
	}

	raw, err := json.Marshal(plugin)
	require.NoError(t, err)

	return &ptpv1.PtpProfile{
		Plugins: map[string]*apiextv1.JSON{
			string(ptp.PluginTypeE810): {Raw: raw},
		},
	}
}

func TestPtp4lUsesNICName(t *testing.T) {
	t.Parallel()

	assert.True(t, ptp4lUsesNICName(ProfileTypeBC))
	assert.True(t, ptp4lUsesNICName(ProfileTypeTBCReceiver))
	assert.False(t, ptp4lUsesNICName(ProfileTypeOC))
	assert.False(t, ptp4lUsesNICName(ProfileTypeTBCTransmitter))
}

func TestIsGMProfile(t *testing.T) {
	t.Parallel()

	assert.True(t, isGMProfile(ProfileTypeGM))
	assert.True(t, isGMProfile(ProfileTypeMultiNICGM))
	assert.True(t, isGMProfile(ProfileTypeNTPFallback))
	assert.False(t, isGMProfile(ProfileTypeBC))
}

func TestGetExpectedForProfileOCSlaveUsesRawIface(t *testing.T) {
	t.Parallel()

	profileInfo := profileInfoWithClientIfaces(ProfileTypeOC, "ens1f0")
	got := getExpectedForProfile(nil, "worker-0", profileInfo)

	require.Len(t, got, 2)
	assert.Equal(t, metrics.ExpectedClockState{
		Process:   metrics.ProcessPHC2SYS,
		Interface: string(iface.ClockRealtime),
		Node:      "worker-0",
	}, got[0])
	assert.Equal(t, metrics.ExpectedClockState{
		Process:   metrics.ProcessPTP4L,
		Interface: "ens1f0",
		Node:      "worker-0",
	}, got[1])
}

func TestGetExpectedForProfileBCUsesNICIface(t *testing.T) {
	t.Parallel()

	profileInfo := profileInfoWithClientIfaces(ProfileTypeBC, "ens3f2")
	got := getExpectedForProfile(nil, "worker-0", profileInfo)

	require.Len(t, got, 2)
	assert.Equal(t, metrics.ExpectedClockState{
		Process:   metrics.ProcessPTP4L,
		Interface: "ens3fx",
		Node:      "worker-0",
	}, got[1])
}

func TestGetExpectedForProfileTBCTransmitterExcludesPhc2sys(t *testing.T) {
	t.Parallel()

	profileInfo := profileInfoWithClientIfaces(ProfileTypeTBCTransmitter)
	got := getExpectedForProfile(nil, "worker-0", profileInfo)

	for _, entry := range got {
		assert.NotEqual(t, metrics.ProcessPHC2SYS, entry.Process)
	}
}

func TestGetExpectedDpllFromPluginRX(t *testing.T) {
	t.Parallel()

	profileInfo := profileInfoWithClientIfaces(ProfileTypeGM)
	rawProfile := profileWithDpllSettings(t)

	got := getExpectedDpll("worker-0", profileInfo, rawProfile)
	require.Len(t, got, 1)
	assert.Equal(t, metrics.ExpectedClockState{
		Process:   metrics.ProcessDPLL,
		Interface: "ens2fx",
		Node:      "worker-0",
	}, got[0])
}

func TestGetExpectedGnssFromTXPin(t *testing.T) {
	t.Parallel()

	profileInfo := profileInfoWithClientIfaces(ProfileTypeGM)
	rawProfile := profileWithE810Pins(t, map[string]map[string]string{
		"ens7f0": {"SMA1": "2 1", "SMA2": "0 2"},
	})

	got := getExpectedGnss("worker-0", profileInfo, rawProfile)
	require.Len(t, got, 1)
	assert.Equal(t, metrics.ExpectedClockState{
		Process:   metrics.ProcessGNSS,
		Interface: "ens7fx",
		Node:      "worker-0",
	}, got[0])
}

func TestGetExpectedClockStatesAggregatesProfiles(t *testing.T) {
	t.Parallel()

	nodeInfoMap := map[string]*NodeInfo{
		"worker-0": {
			Profiles: []*ProfileInfo{
				profileInfoWithClientIfaces(ProfileTypeOC, "ens1f0"),
			},
		},
	}

	got, err := GetExpectedClockStates(nil, nodeInfoMap)
	require.NoError(t, err)
	require.NotEmpty(t, got)

	foundPtp4l := false
	for _, entry := range got {
		if entry.Process == metrics.ProcessPTP4L && entry.Interface == "ens1f0" {
			foundPtp4l = true
		}
	}

	assert.True(t, foundPtp4l)
}
