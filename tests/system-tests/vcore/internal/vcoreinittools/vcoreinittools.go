package vcoreinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// VCoreConfig provides access to vCore system tests configuration parameters.
	VCoreConfig *vcoreconfig.VCoreConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	VCoreConfig = vcoreconfig.NewVCoreConfig()
	APIClient = inittools.APIClient
}
