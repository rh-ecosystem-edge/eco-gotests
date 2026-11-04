//go:build unit_test

package hwolenv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountOffloadedTrafficPackets(t *testing.T) {
	const (
		client    = "3e:f5:01:0e:d5:61"
		server    = "1a:77:44:80:66:20"
		unrelated = "recirc_id(0),in_port(ovn-k8s-mp0),eth(src=00:00:00:00:00:00,dst=0a:58:0a:81:02:03),eth_type(0x0800), packets:5194, actions:ct()"                                   //nolint:lll
		forward   = "recirc_id(0),in_port(ens3f0np0_1),eth(src=3e:f5:01:0e:d5:61,dst=1a:77:44:80:66:20),eth_type(0x0800),ipv4(frag=no), packets:100, bytes:150000, actions:ens3f0np0_2" //nolint:lll
		reverse   = "recirc_id(0),in_port(ens3f0np0_2),eth(src=1a:77:44:80:66:20,dst=3e:f5:01:0e:d5:61),eth_type(0x0800),ipv4(frag=no), packets:5, bytes:500, actions:ens3f0np0_1"      //nolint:lll
		arp       = "recirc_id(0),eth(src=1a:77:44:80:66:20,dst=3e:f5:01:0e:d5:61),eth_type(0x0806), packets:99"
	)

	count, err := countOffloadedTrafficPackets(unrelated+"\n"+forward+"\n"+reverse+"\n"+arp, client, server)
	require.NoError(t, err)
	require.EqualValues(t, 105, count)

	count, err = countOffloadedTrafficPackets(unrelated, client, server)
	require.NoError(t, err)
	require.Zero(t, count)

	_, err = countOffloadedTrafficPackets("eth(src="+client+",dst="+server+"),eth_type(0x0800)", client, server)
	require.ErrorContains(t, err, "no packet counter")

	_, err = countOffloadedTrafficPackets(forward, client, client)
	require.ErrorContains(t, err, "duplicate")

	_, err = countOffloadedTrafficPackets(forward, "invalid", server)
	require.ErrorContains(t, err, "invalid first workload MAC")
}

func TestSwitchdevBridgeConfig(t *testing.T) {
	standard := switchdevBridgeConfig(false)
	require.NotNil(t, standard.OVS)
	require.Empty(t, standard.OVS.Bridge.DatapathType)
	require.Empty(t, standard.OVS.Uplink.Interface.Type)

	doca := switchdevBridgeConfig(true)
	require.NotNil(t, doca.OVS)
	require.Equal(t, "doca", doca.OVS.Bridge.DatapathType)
	require.Equal(t, "doca", doca.OVS.Uplink.Interface.Type)
}

func TestSelectOvsDatapath(t *testing.T) {
	output := "system@ovs-system\ndoca@ovs-doca\n"
	datapath, err := selectOvsDatapath(output, "doca")
	require.NoError(t, err)
	require.Equal(t, "doca@ovs-doca", datapath)

	datapath, err = selectOvsDatapath(output, "system")
	require.NoError(t, err)
	require.Equal(t, "system@ovs-system", datapath)

	_, err = selectOvsDatapath(output, "netdev")
	require.ErrorContains(t, err, "no netdev datapath")

	_, err = selectOvsDatapath("doca@first\ndoca@second\n", "doca")
	require.ErrorContains(t, err, "multiple doca datapaths")
}
