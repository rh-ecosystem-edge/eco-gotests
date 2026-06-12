//go:build unit_test

package iface

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitNICNaming(t *testing.T) {
	testCases := []struct {
		name       string
		ptpVersion string
		wantLegacy bool
	}{
		{
			name:       "PTP 4.19 uses legacy naming",
			ptpVersion: "4.19.0",
			wantLegacy: true,
		},
		{
			name:       "PTP 4.20 uses modern naming",
			ptpVersion: "4.20.0",
			wantLegacy: false,
		},
		{
			name:       "PTP 4.20 pre-release uses modern naming",
			ptpVersion: "4.20.0-20251212.151256",
			wantLegacy: false,
		},
		{
			name:       "unparsable version uses modern naming by default",
			ptpVersion: "not-a-version",
			wantLegacy: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := InitNICNaming(testCase.ptpVersion)
			assert.NoError(t, err)
			assert.Equal(t, testCase.wantLegacy, useLegacyNICNaming)
		})
	}
}

//nolint:funlen // long due to number of test cases
func TestNameGetNIC(t *testing.T) {
	testCases := []struct {
		name       string
		ptpVersion string
		iface      Name
		wantNIC    NICName
	}{
		// Modern naming (PTP >= 4.20)
		{
			name:       "modern replaces trailing digit sequence",
			ptpVersion: "4.20.0",
			iface:      "eth1",
			wantNIC:    "ethx",
		},
		{
			name:       "modern handles SR-IOV interface",
			ptpVersion: "4.20.0",
			iface:      "ens2f0",
			wantNIC:    "ens2fx",
		},
		{
			name:       "modern strips np suffix before deriving NIC",
			ptpVersion: "4.20.0",
			iface:      "eth0np0",
			wantNIC:    "ethx",
		},
		{
			name:       "modern handles VF with np suffix",
			ptpVersion: "4.20.0",
			iface:      "ens2f0np0",
			wantNIC:    "ens2fx",
		},
		{
			name:       "modern preserves VLAN",
			ptpVersion: "4.20.0",
			iface:      "ens2f0.100",
			wantNIC:    "ens2fx.100",
		},
		{
			name:       "modern handles PCI-style interface name",
			ptpVersion: "4.20.0",
			iface:      "enp0s2f0",
			wantNIC:    "enp0s2fx",
		},
		// Legacy naming (PTP <= 4.19)
		{
			name:       "legacy replaces last character including np suffix",
			ptpVersion: "4.19.0",
			iface:      "eth0np0",
			wantNIC:    "eth0npx",
		},
		{
			name:       "legacy handles SR-IOV interface",
			ptpVersion: "4.19.0",
			iface:      "ens2f0",
			wantNIC:    "ens2fx",
		},
		{
			name:       "legacy handles VF with np suffix differently from modern",
			ptpVersion: "4.19.0",
			iface:      "ens2f0np0",
			wantNIC:    "ens2f0npx",
		},
		{
			name:       "legacy preserves VLAN",
			ptpVersion: "4.19.0",
			iface:      "ens2f0.100",
			wantNIC:    "ens2fx.100",
		},
		// Special and invalid names (naming system independent)
		{
			name:       "clock realtime is unchanged",
			ptpVersion: "4.19.0",
			iface:      "CLOCK_REALTIME",
			wantNIC:    ClockRealtime,
		},
		{
			name:       "master is unchanged",
			ptpVersion: "4.20.0",
			iface:      "master",
			wantNIC:    Master,
		},
		{
			name:       "single character interface is invalid",
			ptpVersion: "4.19.0",
			iface:      "a",
			wantNIC:    "",
		},
		{
			name:       "empty interface is invalid",
			ptpVersion: "4.19.0",
			iface:      "",
			wantNIC:    "",
		},
		{
			name:       "unrecognized format is invalid",
			ptpVersion: "4.20.0",
			iface:      "invalid",
			wantNIC:    "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := InitNICNaming(testCase.ptpVersion)
			assert.NoError(t, err)
			assert.Equal(t, testCase.wantNIC, testCase.iface.GetNIC())
		})
	}
}
