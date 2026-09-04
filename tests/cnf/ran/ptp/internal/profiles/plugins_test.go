//go:build unit_test

package profiles

import (
	"encoding/json"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
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
		want    iface.Name
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

			got, err := GetGmInterfaceToGPS(testCase.profile)
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
	assert.Equal(t, []iface.Name{"ens2f0"}, got)
}

func TestGetDpllInterfaces(t *testing.T) {
	t.Parallel()

	t.Run("E810 uses RX pins", func(t *testing.T) {
		t.Parallel()

		profile := profileWithE810Pins(t, map[string]map[string]string{
			"ens7f0": {"SMA1": "0 1", "SMA2": "0 2"},
			"ens2f0": {"SMA1": "0 1", "SMA2": "1 2"},
		})

		got, err := GetDpllInterfaces(profile)
		require.NoError(t, err)
		assert.Equal(t, []iface.Name{"ens2f0"}, got)
	})

	t.Run("nil profile", func(t *testing.T) {
		t.Parallel()

		_, err := GetDpllInterfaces(nil)
		assert.Error(t, err)
	})
}

func TestGetUpstreamPortsForProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile *ptpv1.PtpProfile
		want    []iface.Name
		wantErr bool
	}{
		{
			name: "single upstreamPort",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "ens7f1"},
			},
			want: []iface.Name{"ens7f1"},
		},
		{
			name: "dual comma-separated upstreamPort",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "ens7f1,ens7f3"},
			},
			want: []iface.Name{"ens7f1", "ens7f3"},
		},
		{
			name: "dual upstreamPort with spaces",
			profile: &ptpv1.PtpProfile{
				PtpSettings: map[string]string{"upstreamPort": "eno8503np2, eno8603np3"},
			},
			want: []iface.Name{"eno8503np2", "eno8603np3"},
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
