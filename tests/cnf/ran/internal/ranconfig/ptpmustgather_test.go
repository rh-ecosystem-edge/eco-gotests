//go:build unit_test

package ranconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePTPMustGatherImageUsesConfiguredImage(t *testing.T) {
	config := RANConfig{PTPMustGatherImage: "registry.example.com/ptp-must-gather:latest"}

	image, err := config.ResolvePTPMustGatherImage(nil)

	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/ptp-must-gather:latest", image)
}

func TestPTPMustGatherImageForVersion(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		expectedImage string
		expectError   bool
	}{
		{
			name:          "RHEL 8 image before PTP operator 4.16",
			version:       "4.15.37",
			expectedImage: "registry.redhat.io/openshift4/ptp-must-gather-rhel8:v4.15",
		},
		{
			name:          "RHEL 9 image at PTP operator 4.16",
			version:       "4.16.0",
			expectedImage: "registry.redhat.io/openshift4/ptp-must-gather-rhel9:v4.16",
		},
		{
			name:          "RHEL 9 image after PTP operator 4.16",
			version:       "4.22.3-rc.1",
			expectedImage: "registry.redhat.io/openshift4/ptp-must-gather-rhel9:v4.22",
		},
		{
			name:          "openshift5 RHEL 9 image at PTP operator 5.0",
			version:       "5.0.0",
			expectedImage: "registry.redhat.io/openshift5/ptp-must-gather-rhel9:v5.0",
		},
		{
			name:        "invalid version",
			version:     "invalid",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			image, err := formatPTPMustGatherImageForVersion(test.version)
			if test.expectError {
				require.Error(t, err)
				assert.Empty(t, image)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expectedImage, image)
		})
	}
}
