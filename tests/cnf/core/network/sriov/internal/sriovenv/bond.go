package sriovenv

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
)

// CreateBondNAD creates a Bond CNI NAD in the SR-IOV test namespace.
// This follows the same pattern used by sriov/tests/qinq.go.
func CreateBondNAD(
	nadName string,
	mode string,
	mtu int,
	slaveCount int,
	ipamType string,
	vlanInContainer *uint16,
) (*nad.Builder, error) {
	if slaveCount < 2 {
		return nil, fmt.Errorf("slaveCount must be >= 2, got %d", slaveCount)
	}

	var links []nad.Link
	for i := 1; i <= slaveCount; i++ {
		links = append(links, nad.Link{Name: fmt.Sprintf("net%d", i)})
	}

	plugin := nad.NewMasterBondPlugin(nadName, mode).
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks(links).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(&nad.IPAM{Type: ipamType})

	if vlanInContainer != nil {
		plugin = plugin.WithVLANInContainer(*vlanInContainer)
	}

	masterPlugin, err := plugin.GetMasterPluginConfig()
	if err != nil {
		return nil, err
	}

	if mtu > 0 {
		masterPlugin.Mtu = mtu
	}

	createdNAD, err := nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPlugin).
		Create()
	if err != nil {
		return nil, err
	}

	return createdNAD, nil
}

// CreateBondNADStaticIPAM creates a two-slave bond NAD with static IPAM, no in-container VLAN,
// and unset bond MTU (0). This matches the common default used by tests such as allmulti.
func CreateBondNADStaticIPAM(nadName, mode string) (*nad.Builder, error) {
	return CreateBondNAD(nadName, mode, 0, 2, "static", nil)
}
