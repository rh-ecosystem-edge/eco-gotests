package raninittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/ranconfig"
)

var (
	// HubAPIClient provides API access to the first spoke cluster.
	HubAPIClient *clients.Settings
	// Spoke1APIClient provides API access to cluster.
	Spoke1APIClient *clients.Settings
	// Spoke2APIClient provides API access to the second spoke cluster.
	Spoke2APIClient *clients.Settings
	// RANConfig provides access to configuration.
	RANConfig *ranconfig.RANConfig
)

func init() {
	RANConfig = ranconfig.NewRANConfig()
	HubAPIClient = RANConfig.HubAPIClient
	Spoke1APIClient = RANConfig.Spoke1APIClient
	Spoke2APIClient = RANConfig.Spoke2APIClient
}
