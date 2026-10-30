//go:build unit_test

package metrics

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/stretchr/testify/assert"
)

func TestClockStateQueryInterfaceLabel(t *testing.T) {
	testCases := []struct {
		name     string
		query    ClockStateQuery
		expected string
	}{
		{
			name: "multiple aliases preserve balanced regex",
			query: ClockStateQuery{
				Interface: Includes(iface.Alias("ens1fx"), iface.Alias("ens2fx")),
			},
			expected: `=~"(ens1fx|ens2fx)"`,
		},
		{
			name: "canonical alias remains exact",
			query: ClockStateQuery{
				Interface: Equals(iface.Alias("ens1fx")),
			},
			expected: `="ens1fx"`,
		},
		{
			name: "clock realtime remains exact",
			query: ClockStateQuery{
				Interface: Equals(iface.ClockRealtime),
			},
			expected: `="CLOCK_REALTIME"`,
		},
		{
			name:     "unset interface excludes master",
			query:    ClockStateQuery{},
			expected: `!="master"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			label := testCase.query.ToMetricQuery().Labels[KeyInterface]

			assert.Equal(t, testCase.expected, label.String())
		})
	}
}
