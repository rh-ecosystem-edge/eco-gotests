package ipcinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcconfig"
)

var (
	// TargetSNOAPIClient is the api client to the target sno cluster.
	TargetSNOAPIClient *clients.Settings
	// IPCConfig provides access to configuration parameters.
	IPCConfig *ipcconfig.IPCConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
//
//nolint:gochecknoinits // Package initialization pattern used throughout eco-gotests
func init() {
	IPCConfig = ipcconfig.NewIPCConfig()
	if IPCConfig == nil {
		IPCConfig = &ipcconfig.IPCConfig{}
		TargetSNOAPIClient = nil

		return
	}

	TargetSNOAPIClient = IPCConfig.GetTargetSNOAPIClient()
}
