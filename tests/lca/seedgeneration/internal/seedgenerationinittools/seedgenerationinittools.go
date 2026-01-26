package seedgenerationinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/internal/seedgenerationconfig"
)

var (
	// TargetSNOAPIClient is the api client to the target sno cluster.
	TargetSNOAPIClient *clients.Settings
	// SeedGenerationConfig provides access to configuration parameters.
	SeedGenerationConfig *seedgenerationconfig.SeedGenerationConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
//
//nolint:gochecknoinits // Package initialization pattern used throughout eco-gotests
func init() {
	SeedGenerationConfig = seedgenerationconfig.NewSeedGenerationConfig()
	if SeedGenerationConfig == nil {
		SeedGenerationConfig = &seedgenerationconfig.SeedGenerationConfig{}
		TargetSNOAPIClient = nil

		return
	}

	TargetSNOAPIClient = SeedGenerationConfig.GetTargetSNOAPIClient()
}
