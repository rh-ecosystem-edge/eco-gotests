package raninittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/ranconfig"
)

var (
	// HubAPIClient is the API client to the hub cluster (used for pull-secret, BMH, etc.).
	HubAPIClient *clients.Settings
	// RANConfig provides access to ran (CNF IBI) configuration parameters.
	RANConfig *ranconfig.RANConfig
)

//nolint:gochecknoinits // Package initialization pattern used throughout eco-gotests
func init() {
	RANConfig = ranconfig.NewRANConfig()

	if RANConfig != nil && RANConfig.HubKubeConfig != "" {
		HubAPIClient = clients.New(RANConfig.HubKubeConfig)
	}
}
