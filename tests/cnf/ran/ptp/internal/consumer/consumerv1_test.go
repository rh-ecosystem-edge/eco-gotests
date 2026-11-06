//go:build unit_test

package consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefineSidecarContainerAPIVersionArg(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		ocpVersion string
		wantAPIArg bool
	}{
		{
			name:       "OCP 4.14 omits unsupported argument",
			ocpVersion: "4.14.18",
		},
		{
			name:       "OCP 4.15 omits unsupported argument",
			ocpVersion: "4.15.35",
		},
		{
			name:       "OCP 4.16 includes supported argument",
			ocpVersion: "4.16.0",
			wantAPIArg: true,
		},
		{
			name:       "OCP 4.16 pre-release includes supported argument",
			ocpVersion: "4.16.0-20240601.123456",
			wantAPIArg: true,
		},
		{
			name:       "unavailable OCP version preserves existing argument",
			wantAPIArg: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			container, err := defineSidecarContainer("worker-0.example.com", "proxy-image", testCase.ocpVersion)
			assert.NoError(t, err)
			assert.Equal(t, testCase.wantAPIArg, containsArg(container.Args, "--api-version=1.0"))
		})
	}
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}

	return false
}
