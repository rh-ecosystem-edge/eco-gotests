//go:build unit_test

package o2imstest

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
)

func TestAsStringKeyedMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     any
		converted bool
		expected  map[string]string
	}{
		{
			name:      "map string any",
			value:     map[string]any{"cores": "96", "architecture": "x86_64"},
			converted: true,
			expected:  map[string]string{"cores": "96", "architecture": "x86_64"},
		},
		{
			name:      "map string string",
			value:     map[string]string{"GiB": "256"},
			converted: true,
			expected:  map[string]string{"GiB": "256"},
		},
		{
			name:      "nested non-string values",
			value:     map[string]any{"cores": 96},
			converted: true,
			expected:  map[string]string{"cores": "96"},
		},
		{name: "empty map", value: map[string]any{}, converted: true, expected: map[string]string{}},
		{name: "not a map", value: "not-a-map", converted: false},
		{name: "nil", value: nil, converted: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, converted := AsStringKeyedMap(testCase.value)

			assert.Equal(t, testCase.converted, converted)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestExtensionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		extensions map[string]any
		key, want  string
	}{
		{name: "nil extensions", key: tsparams.ClusterModelExtension},
		{
			name:       "present string",
			extensions: map[string]any{tsparams.ClusterModelExtension: tsparams.ClusterModelHubCluster},
			key:        tsparams.ClusterModelExtension,
			want:       tsparams.ClusterModelHubCluster,
		},
		{
			name:       "missing key",
			extensions: map[string]any{tsparams.ClusterModelExtension: tsparams.ClusterModelHubCluster},
			key:        "missing",
		},
		{
			name:       "nil value",
			extensions: map[string]any{tsparams.ClusterModelExtension: nil},
			key:        tsparams.ClusterModelExtension,
		},
		{
			name:       "non-string value",
			extensions: map[string]any{tsparams.ClusterModelExtension: 4},
			key:        tsparams.ClusterModelExtension,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, ExtensionString(testCase.extensions, testCase.key))
		})
	}
}
