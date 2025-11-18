package ocpsriovinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	sriovconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// SriovOcpConfig provides access to general configuration parameters.
	SriovOcpConfig *sriovconfig.SriovOcpConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	SriovOcpConfig = sriovconfig.NewSriovOcpConfig()
	APIClient = inittools.APIClient
}
