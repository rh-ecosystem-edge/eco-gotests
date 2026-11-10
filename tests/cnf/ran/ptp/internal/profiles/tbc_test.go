//go:build unit_test

package profiles

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/stretchr/testify/assert"
)

func TestHoldoverSettingsFor(t *testing.T) {
	t.Parallel()

	hardwareConfigTBC := &ProfileInfo{
		ProfileType:    ProfileTypeTBCReceiver,
		HardwareConfig: &ptp.HardwareConfigBuilder{},
	}
	hardwareConfigDualTBC := &ProfileInfo{
		ProfileType:    ProfileTypeDualTBCReceiver,
		HardwareConfig: &ptp.HardwareConfigBuilder{},
	}
	hardwareConfigTTSC := &ProfileInfo{
		ProfileType:    ProfileTypeTTSC,
		HardwareConfig: &ptp.HardwareConfigBuilder{},
	}
	pluginProfile := &ProfileInfo{ProfileType: ProfileTypeTBCReceiver}

	tests := []struct {
		name        string
		profileInfo *ProfileInfo
		requested   HoldoverPluginSettings
		want        HoldoverPluginSettings
	}{
		{
			name:      "nil profile keeps WPC no-out-of-spec",
			requested: HoldoverPluginSettingsNoOutOfSpec,
			want:      HoldoverPluginSettingsNoOutOfSpec,
		},
		{
			name:        "plugin path keeps WPC no-out-of-spec",
			profileInfo: pluginProfile,
			requested:   HoldoverPluginSettingsNoOutOfSpec,
			want:        HoldoverPluginSettingsNoOutOfSpec,
		},
		{
			name:        "plugin path keeps WPC with-out-of-spec",
			profileInfo: pluginProfile,
			requested:   HoldoverPluginSettingsWithOutOfSpec,
			want:        HoldoverPluginSettingsWithOutOfSpec,
		},
		{
			name:        "HardwareConfig T-BC remaps no-out-of-spec to GNR-D",
			profileInfo: hardwareConfigTBC,
			requested:   HoldoverPluginSettingsNoOutOfSpec,
			want:        HoldoverPluginSettingsGNRDNoOutOfSpec,
		},
		{
			name:        "HardwareConfig T-BC remaps with-out-of-spec to GNR-D",
			profileInfo: hardwareConfigTBC,
			requested:   HoldoverPluginSettingsWithOutOfSpec,
			want:        HoldoverPluginSettingsGNRDWithOutOfSpec,
		},
		{
			name:        "HardwareConfig dual T-BC remaps no-out-of-spec to GNR-D",
			profileInfo: hardwareConfigDualTBC,
			requested:   HoldoverPluginSettingsNoOutOfSpec,
			want:        HoldoverPluginSettingsGNRDNoOutOfSpec,
		},
		{
			name:        "HardwareConfig T-TSC keeps WPC stress presets",
			profileInfo: hardwareConfigTTSC,
			requested:   HoldoverPluginSettingsNoOutOfSpec,
			want:        HoldoverPluginSettingsNoOutOfSpec,
		},
		{
			name:        "HardwareConfig T-BC leaves custom settings unchanged",
			profileInfo: hardwareConfigTBC,
			requested:   HoldoverPluginSettings{LocalHoldoverTimeout: 10, MaxInSpecOffset: 20, LocalMaxHoldoverOffSet: 30},
			want:        HoldoverPluginSettings{LocalHoldoverTimeout: 10, MaxInSpecOffset: 20, LocalMaxHoldoverOffSet: 30},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, HoldoverSettingsFor(testCase.profileInfo, testCase.requested))
		})
	}
}
