package version

// The version package imports cluster (via version.go), which pulls inittools. For local unit tests of
// IsVersionStringInRange only, run: UNIT_TEST=true go test ./tests/cnf/ran/internal/version/... -run TestIsVersionStringInRange

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:funlen
func TestIsVersionStringInRange(t *testing.T) {
	testCases := []struct {
		version        string
		minimum        string
		maximum        string
		expectedResult bool
		expectedError  error
	}{
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "",
			maximum:        "4.20",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "4.20",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "4.20.0-0",
			maximum:        "",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "",
			maximum:        "4.10",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "4.10.0-0",
			maximum:        "4.15",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "4.0.0-0",
			maximum:        "5.0",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "3.0.0-0",
			maximum:        "4.0",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "4.16.0",
			minimum:        "invalid minimum",
			maximum:        "",
			expectedResult: false,
			expectedError:  fmt.Errorf("invalid minimum provided: 'invalid minimum'"),
		},
		{
			version:        "4.16.0",
			minimum:        "",
			maximum:        "invalid maximum",
			expectedResult: false,
			expectedError:  fmt.Errorf("invalid maximum provided: 'invalid maximum'"),
		},
		{
			version:        "",
			minimum:        "3.0.0-0",
			maximum:        "4.0",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "",
			minimum:        "3.0.0-0",
			maximum:        "",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "4.20.0-20251212.151256",
			minimum:        "4.20.0",
			maximum:        "",
			expectedResult: false,
			expectedError:  nil,
		},
		{
			version:        "4.20.0-20251212.151256",
			minimum:        "4.20.0-0",
			maximum:        "",
			expectedResult: true,
			expectedError:  nil,
		},
		{
			version:        "v4.16.5",
			minimum:        "4.16.0-0",
			maximum:        "",
			expectedResult: true,
			expectedError:  nil,
		},
	}

	for _, testCase := range testCases {
		result, err := IsVersionStringInRange(testCase.version, testCase.minimum, testCase.maximum)

		assert.Equal(t, testCase.expectedResult, result)
		assert.Equal(t, testCase.expectedError, err)
	}
}
