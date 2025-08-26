package systemtestsinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// SystemTestsTestConfig provides access to system tests configuration parameters.
	SystemTestsTestConfig *systemtestsconfig.SystemTestsConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	SystemTestsTestConfig = systemtestsconfig.NewSystemTestsConfig()
	APIClient = inittools.APIClient
}
