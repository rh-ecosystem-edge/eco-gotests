//go:build unit_test

package processes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPtp4lConfigSupport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ptpVersion  string
		wantSupport bool
		wantConfig  string
	}{
		{name: "version before config labels", ptpVersion: "4.20.0-0", wantSupport: false, wantConfig: ""},
		{name: "version with config labels", ptpVersion: "4.21.0-0", wantSupport: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotSupport, err := isPtp4lConfigSupported(testCase.ptpVersion)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantSupport, gotSupport)

			if !gotSupport {
				gotConfig, err := GetPtp4lConfigByRelatedProcessIfSupported(nil, "node", testCase.ptpVersion, Ts2phc)
				require.NoError(t, err)
				assert.Equal(t, testCase.wantConfig, gotConfig)
			}
		})
	}
}
