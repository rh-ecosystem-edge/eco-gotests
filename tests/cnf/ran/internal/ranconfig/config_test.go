//go:build unit_test

package ranconfig

import (
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTalmPreCachePolicies(t *testing.T) {
	var config RANConfig

	require.NoError(t, readFile(&config, PathToDefaultCnfRanParamsFile))
	require.NotEmpty(t, config.TalmPreCachePolicies)

	patterns := make([]*regexp.Regexp, 0, len(config.TalmPreCachePolicies))
	for _, expression := range config.TalmPreCachePolicies {
		pattern, err := regexp.Compile(expression)
		require.NoError(t, err, "Failed to compile TALM pre-cache policy pattern %q", expression)

		patterns = append(patterns, pattern)
	}

	policies := []string{
		"common-config-policy",
		"common-subscriptions-policy",
		"common-v4.19-config-policy",
		"common-v4.19-subscriptions-policy",
		"common-v5.0-config-policy",
		"common-v5.0-subscriptions-policy",
	}

	for _, policy := range policies {
		assert.True(t,
			slices.ContainsFunc(patterns, func(pattern *regexp.Regexp) bool { return pattern.MatchString(policy) }),
			"Expected TALM pre-cache policies to include a pattern matching %q", policy)
	}
}

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
