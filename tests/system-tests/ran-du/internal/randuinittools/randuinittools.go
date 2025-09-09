package randuinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// RanDuTestConfig provides access to RAN DU system tests configuration parameters.
	RanDuTestConfig *randuconfig.RanDuConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	RanDuTestConfig = randuconfig.NewRanDuConfig()
	APIClient = inittools.APIClient
}
