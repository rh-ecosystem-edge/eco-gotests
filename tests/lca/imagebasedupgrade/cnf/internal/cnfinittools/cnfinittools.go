package cnfinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfconfig"
)

var (
	// TargetHubAPIClient is the api client to the target hub cluster.
	TargetHubAPIClient *clients.Settings
	// TargetSNOAPIClient is the api client to the target sno cluster.
	TargetSNOAPIClient *clients.Settings
	// CNFConfig provides access to general configuration parameters.
	CNFConfig *cnfconfig.CNFConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	CNFConfig = cnfconfig.NewCNFConfig()

	if CNFConfig.TargetHubKubeConfig != "" {
		TargetHubAPIClient = clients.New(CNFConfig.TargetHubKubeConfig)
	}

	if CNFConfig.TargetSNOKubeConfig != "" {
		TargetSNOAPIClient = clients.New(CNFConfig.TargetSNOKubeConfig)
	}
}
