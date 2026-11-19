//go:build unit_test

package profiles

import (
	"encoding/json"
	"testing"

	ptpv1 "github.com/k8snetworkplumbingwg/ptp-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func profileWithE810Pins(t *testing.T, pins map[string]map[string]string) *ptpv1.PtpProfile {
	t.Helper()

	plugin := ptp.IntelPlugin{Pins: pins}

	raw, err := json.Marshal(plugin)
	require.NoError(t, err)

	return &ptpv1.PtpProfile{
		Plugins: map[string]*apiextv1.JSON{
			string(ptp.PluginTypeE810): {Raw: raw},
		},
	}
}

func TestGetGmInterfaceToGPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile *ptpv1.PtpProfile
		want    iface.Iface
		wantErr bool
	}{
		{
			name: "multi-NIC GM uses TX pin",
			profile: profileWithE810Pins(t, map[string]map[string]string{
				"ens7f0": {"SMA1": "2 1", "SMA2": "0 2"},
				"ens2f0": {"SMA1": "0 1", "SMA2": "1 2"},
			}),
			want: "ens7f0",
		},
		{
			name: "single-NIC GM uses sole E810 interface",
			profile: profileWithE810Pins(t, map[string]map[string]string{
				"ens2f0": {
					"SMA1":  "0 1",
					"SMA2":  "0 2",
					"U.FL1": "0 1",
					"U.FL2": "0 2",
				},
			}),
			want: "ens2f0",
		},
		{
			name:    "nil profile",
			profile: nil,
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := GetGmInterfaceToGPS(testCase.profile, nil)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, testCase.want, got)
			}
		})
	}
}

func TestGetRxInterfaces(t *testing.T) {
	t.Parallel()

	profile := profileWithE810Pins(t, map[string]map[string]string{
		"ens7f0": {"SMA1": "0 1", "SMA2": "0 2"},
		"ens2f0": {"SMA1": "0 1", "SMA2": "1 2"},
	})

	got, err := GetRxInterfaces(profile)
	require.NoError(t, err)
	assert.Equal(t, []iface.Iface{"ens2f0"}, got)
}

func TestGetUpstreamPortsForProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile *ptpv1.PtpProfile
		want    []iface.Iface
		wantErr bool
	}{
		{
			name: "single upstreamPort",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "ens7f1"},
			},
			want: []iface.Iface{"ens7f1"},
		},
		{
			name: "dual comma-separated upstreamPort",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "ens7f1,ens7f3"},
			},
			want: []iface.Iface{"ens7f1", "ens7f3"},
		},
		{
			name: "dual upstreamPort with spaces",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "eno8503np2, eno8603np3"},
			},
			want: []iface.Iface{"eno8503np2", "eno8603np3"},
		},
		{
			name:    "missing upstreamPort and plugin",
			profile: &ptpv1.PtpProfile{},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := GetUpstreamPortsForProfile(testCase.profile)
			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestLocalHoldoverTimeoutDisabled(t *testing.T) {
	t.Parallel()

	e825Timeout := func(timeout *uint64) *ptpv1.PtpProfile {
		t.Helper()

		plugin := ptp.IntelPlugin{Devices: []string{"eno1np0"}}
		if timeout != nil {
			plugin.DpllSettings = map[string]uint64{"LocalHoldoverTimeout": *timeout}
		}

		raw, err := json.Marshal(plugin)
		require.NoError(t, err)

		return &ptpv1.PtpProfile{
			Plugins: map[string]*apiextv1.JSON{
				string(ptp.PluginTypeE825): {Raw: raw},
			},
		}
	}

	zero := uint64(0)
	fourHours := uint64(14400)

	e830Only, err := json.Marshal(ptp.IntelPlugin{Devices: []string{"enp108s0f0np0"}})
	require.NoError(t, err)

	e825Zero, err := json.Marshal(ptp.IntelPlugin{
		Devices:      []string{"eno1np0"},
		DpllSettings: map[string]uint64{"LocalHoldoverTimeout": 0},
	})
	require.NoError(t, err)

	e810Long, err := json.Marshal(ptp.IntelPlugin{
		DpllSettings: map[string]uint64{"LocalHoldoverTimeout": fourHours},
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		profile *ptpv1.PtpProfile
		want    bool
	}{
		{name: "nil profile", profile: nil, want: false},
		{name: "no plugins", profile: &ptpv1.PtpProfile{}, want: false},
		{name: "plugin without the setting", profile: e825Timeout(nil), want: false},
		{name: "timeout explicitly zero", profile: e825Timeout(&zero), want: true},
		{name: "timeout four hours", profile: e825Timeout(&fourHours), want: false},
		{
			name: "e825 zero is used when e830 has no setting",
			profile: &ptpv1.PtpProfile{Plugins: map[string]*apiextv1.JSON{
				string(ptp.PluginTypeE830): {Raw: e830Only},
				string(ptp.PluginTypeE825): {Raw: e825Zero},
			}},
			want: true,
		},
		{
			name: "e810 timeout takes precedence over e825 zero",
			profile: &ptpv1.PtpProfile{Plugins: map[string]*apiextv1.JSON{
				string(ptp.PluginTypeE810): {Raw: e810Long},
				string(ptp.PluginTypeE825): {Raw: e825Zero},
			}},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, LocalHoldoverTimeoutDisabled(testCase.profile))
		})
	}
}
