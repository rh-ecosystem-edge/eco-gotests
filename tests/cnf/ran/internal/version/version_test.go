package version

// The version package imports cluster (via version.go), which pulls inittools. For local unit tests of
// IsVersionStringInRange only, run: UNIT_TEST=true go test ./tests/cnf/ran/internal/version/... -run TestIsVersionStringInRange

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:funlen
func TestIsVersionStringInRange(t *testing.T) {
	testCases := []struct {
		version          string
		minimum          string
		maximum          string
		expectedResult   bool
		wantErrSubstring string // if non-empty, err must contain this substring; if empty, err must be nil
	}{
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "",
			expectedResult: true,
		},
		{
			version:        "4.16.0",
			minimum:        "",
			maximum:        "4.20",
			expectedResult: true,
		},
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "4.20",
			expectedResult: true,
		},
		{
			version:        "4.16.0",
			minimum:        "4.20.0-0",
			maximum:        "",
			expectedResult: false,
		},
		{
			version:        "4.16.0",
			minimum:        "",
			maximum:        "4.10",
			expectedResult: false,
		},
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "4.15",
			expectedResult: false,
		},
		{
			version:        "4.16.0",
			minimum:        "4.0.0-0",
			maximum:        "5.0",
			expectedResult: true,
		},
		{
			version:        "4.16.0",
			minimum:        "3.0.0-0",
			maximum:        "4.0",
			expectedResult: false,
		},
		{
			version:          "4.16.0",
			minimum:          "invalid minimum",
			maximum:          "",
			expectedResult: false,
			wantErrSubstring: "invalid minimum provided: 'invalid minimum'",
		},
		{
			version:          "4.16.0",
			minimum:          "",
			maximum:          "invalid maximum",
			expectedResult: false,
			wantErrSubstring: "invalid maximum provided: 'invalid maximum'",
		},
		{
			version:          "4.16.0",
			minimum:          "",
			maximum:          "4.15-rc.1",
			expectedResult: false,
			wantErrSubstring: "invalid maximum provided: '4.15-rc.1'",
		},
		{
			version:        "",
			minimum:        "3.0.0-0",
			maximum:        "4.0",
			expectedResult: false,
		},
		{
			version:        "",
			minimum:        "3.0.0-0",
			maximum:        "",
			expectedResult: true,
		},
		{
			version:        "4.20.0-20251212.151256",
			minimum:        "4.20.0",
			maximum:        "",
			expectedResult: false,
		},
		{
			version:        "4.20.0-20251212.151256",
			minimum:        "4.20.0-0",
			maximum:        "",
			expectedResult: true,
		},
		{
			version:        "v4.16.5",
			minimum:        "4.16.0-0",
			maximum:        "",
			expectedResult: true,
		},
		// Explicit exclusive maximum: v < 4.18.0-0 (4.18.0 release is out).
		{
			version:        "4.17.5",
			minimum:        "",
			maximum:        "4.18.0-0",
			expectedResult: true,
		},
		{
			version:        "4.18.0",
			minimum:        "",
			maximum:        "4.18.0-0",
			expectedResult: false,
		},
		{
			version:        "4.18.5",
			minimum:        "",
			maximum:        "4.18.0-0",
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		result, err := IsVersionStringInRange(testCase.version, testCase.minimum, testCase.maximum)

		assert.Equal(t, testCase.expectedResult, result)
		if testCase.wantErrSubstring != "" {
			assert.Error(t, err)
			assert.ErrorContains(t, err, testCase.wantErrSubstring)
		} else {
			assert.NoError(t, err)
		}
	}
}
