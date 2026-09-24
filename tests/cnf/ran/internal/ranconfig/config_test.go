package ranconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestO2IMSMustGatherImageConfiguration(t *testing.T) {
	var config RANConfig

	require.NoError(t, readFile(&config, PathToDefaultCnfRanParamsFile))
	assert.Equal(t,
		"quay.io/openshift-kni/oran-o2ims-operator-must-gather:latest",
		config.O2IMSMustGatherImage)

	t.Setenv("ECO_CNF_RAN_O2IMS_MUST_GATHER_IMAGE", "registry.example.com/o2ims-must-gather:4.22")
	require.NoError(t, readEnv(&config))
	assert.Equal(t, "registry.example.com/o2ims-must-gather:4.22", config.O2IMSMustGatherImage)
}
