package rdscoreinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// RDSCoreConfig provides access to RDS Core tests configuration parameters.
	RDSCoreConfig *rdscoreconfig.CoreConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	RDSCoreConfig = rdscoreconfig.NewCoreConfig()
	APIClient = inittools.APIClient
}
