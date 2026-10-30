//go:build unit_test

package tests

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	"github.com/stretchr/testify/require"
)

func TestNetworkStatusUsesExactTestNetwork(t *testing.T) {
	oldNamespace := tsparams.TestNamespaceName
	tsparams.TestNamespaceName = "hwol-tests-1234"
	t.Cleanup(func() { tsparams.TestNamespaceName = oldNamespace })
	status := `[
	  {"name":"other/hwol-ovsnet","ips":["10.1.1.1/24"]},
	  {"name":"hwol-tests-1234/hwol-ovsnet-extra","ips":["10.2.2.2/24"]},
	  {"name":"hwol-tests-1234/hwol-ovsnet","ips":["10.3.3.3/24"],"device-info":{"pci":{"pci-address":"0000:4b:00.4"}}}
	]`
	ip, pci, err := networkStatusIPAndPCI(status, "hwol-ovsnet", true)
	require.NoError(t, err)
	require.Equal(t, "10.3.3.3", ip)
	require.Equal(t, "0000:4b:00.4", pci)

	_, _, err = networkStatusIPAndPCI(`[{"name":"other/hwol-ovsnet","ips":["10.1.1.1"]}]`, "hwol-ovsnet", false)
	require.ErrorContains(t, err, "no entry")
	require.False(t, matchesNetworkName("hwol-tests-1234/hwol-ovsnet", ""))
	require.True(t, matchesNetworkName("hwol-tests-1234/hwol-ovsnet", "hwol-tests-1234/hwol-ovsnet"))
	require.False(t, matchesNetworkName("hwol-tests-old/hwol-ovsnet", "hwol-ovsnet"))
}
