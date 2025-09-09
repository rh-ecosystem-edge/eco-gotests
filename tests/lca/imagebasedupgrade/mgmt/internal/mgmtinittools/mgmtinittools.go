package mgmtinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtconfig"
)

var (
	// APIClient provides API access to the cluster.
	APIClient *clients.Settings
	// MGMTConfig provides access to general configuration parameters.
	MGMTConfig *mgmtconfig.MGMTConfig
)

func init() {
	MGMTConfig = mgmtconfig.NewMGMTConfig()
	APIClient = inittools.APIClient
}
