package inittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
)

var (
	// HubAPIClient provides API access to hub cluster.
	HubAPIClient *clients.Settings
	// ZTPConfig provides access to general configuration parameters.
	ZTPConfig *ztpconfig.ZTPConfig
)

func init() {
	ZTPConfig = ztpinittools.ZTPConfig
	HubAPIClient = ztpinittools.HubAPIClient

	if HubAPIClient == nil {
		HubAPIClient = clients.New("")
	}
}
