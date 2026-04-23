package raninittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/ranconfig"
)

var (
	// HubAPIClient is the API client to the hub cluster (used for pull-secret, BMH, etc.).
	HubAPIClient *clients.Settings
	// RanConfig provides access to ran (CNF IBI) configuration parameters.
	RanConfig *ranconfig.RanConfig
)

//nolint:gochecknoinits // Package initialization pattern used throughout eco-gotests
func init() {
	RanConfig = ranconfig.NewRanConfig()

	if RanConfig != nil && RanConfig.HubKubeConfig != "" {
		HubAPIClient = clients.New(RanConfig.HubKubeConfig)
	}
}
