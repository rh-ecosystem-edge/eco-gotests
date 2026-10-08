package nodehealthinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	APIClient = inittools.APIClient
}
