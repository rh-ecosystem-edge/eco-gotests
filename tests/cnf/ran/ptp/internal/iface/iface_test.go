//go:build unit_test

package iface

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitIfaceAliasing(t *testing.T) {
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
			err := InitIfaceAliasingByParsing(testCase.ptpVersion)
			assert.NoError(t, err)
			assert.Equal(t, testCase.wantLegacy, useLegacyNICNaming)
		})
	}
}

//nolint:funlen // long due to number of test cases
func TestIfaceGetAlias(t *testing.T) {
	testCases := []struct {
		name       string
		ptpVersion string
		iface      Iface
		wantAlias  Alias
	}{
		// Modern naming (PTP >= 4.20)
		{
			name:       "modern replaces trailing digit sequence",
			ptpVersion: "4.20.0",
			iface:      "eth1",
			wantAlias:  "ethx",
		},
		{
			name:       "modern handles SR-IOV interface",
			ptpVersion: "4.20.0",
			iface:      "ens2f0",
			wantAlias:  "ens2fx",
		},
		{
			name:       "modern strips np suffix before deriving NIC",
			ptpVersion: "4.20.0",
			iface:      "eth0np0",
			wantAlias:  "ethx",
		},
		{
			name:       "modern handles VF with np suffix",
			ptpVersion: "4.20.0",
			iface:      "ens2f0np0",
			wantAlias:  "ens2fx",
		},
		{
			name:       "modern preserves VLAN",
			ptpVersion: "4.20.0",
			iface:      "ens2f0.100",
			wantAlias:  "ens2fx.100",
		},
		{
			name:       "modern handles PCI-style interface name",
			ptpVersion: "4.20.0",
			iface:      "enp0s2f0",
			wantAlias:  "enp0s2fx",
		},
		// Legacy naming (PTP <= 4.19)
		{
			name:       "legacy replaces last character including np suffix",
			ptpVersion: "4.19.0",
			iface:      "eth0np0",
			wantAlias:  "eth0npx",
		},
		{
			name:       "legacy handles SR-IOV interface",
			ptpVersion: "4.19.0",
			iface:      "ens2f0",
			wantAlias:  "ens2fx",
		},
		{
			name:       "legacy handles VF with np suffix differently from modern",
			ptpVersion: "4.19.0",
			iface:      "ens2f0np0",
			wantAlias:  "ens2f0npx",
		},
		{
			name:       "legacy preserves VLAN",
			ptpVersion: "4.19.0",
			iface:      "ens2f0.100",
			wantAlias:  "ens2fx.100",
		},
		// Special and invalid names (naming system independent)
		{
			name:       "clock realtime is unchanged",
			ptpVersion: "4.19.0",
			iface:      "CLOCK_REALTIME",
			wantAlias:  ClockRealtime,
		},
		{
			name:       "master is unchanged",
			ptpVersion: "4.20.0",
			iface:      "master",
			wantAlias:  Master,
		},
		{
			name:       "single character interface is invalid",
			ptpVersion: "4.19.0",
			iface:      "a",
			wantAlias:  "",
		},
		{
			name:       "empty interface is invalid",
			ptpVersion: "4.19.0",
			iface:      "",
			wantAlias:  "",
		},
		{
			name:       "unrecognized format is invalid",
			ptpVersion: "4.20.0",
			iface:      "invalid",
			wantAlias:  "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := InitIfaceAliasingByParsing(testCase.ptpVersion)
			assert.NoError(t, err)
			assert.Equal(t, testCase.wantAlias, testCase.iface.GetAlias())
		})
	}
}
