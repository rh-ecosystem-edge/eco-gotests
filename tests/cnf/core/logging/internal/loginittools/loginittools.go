package loginittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// CoreConfig provides access to core configuration parameters.
	CoreConfig *coreconfig.CoreConfig
)

func init() {
	CoreConfig = coreconfig.NewCoreConfig()
	APIClient = inittools.APIClient
}
