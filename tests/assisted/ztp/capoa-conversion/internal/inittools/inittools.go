package inittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
)

var (
	// HubAPIClient provides API access to hub cluster via KUBECONFIG env var.
	HubAPIClient *clients.Settings
	// GeneralConfig provides access to general configuration parameters.
	GeneralConfig *config.GeneralConfig
)

func init() {
	HubAPIClient = clients.New("")
	GeneralConfig = config.NewConfig()
}
