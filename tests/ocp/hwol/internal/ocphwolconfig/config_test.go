//go:build unit_test

package ocphwolconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadEnvOvsDoca(t *testing.T) {
	t.Setenv("ECO_OCP_HWOL_OVS_DOCA", "true")
	var config HwolOcpConfig
	require.NoError(t, readEnv(&config))
	require.True(t, config.OvsDocaEnabled)
}

func TestReadEnvCleanupRecovery(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		value := "false"
		if enabled {
			value = "true"
		}
		t.Run(value, func(t *testing.T) {
			t.Setenv("ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE", value)
			var config HwolOcpConfig
			require.NoError(t, readEnv(&config))
			require.Equal(t, enabled, config.RecoverOnCleanupFailure)
		})
	}
}
