package assistedconfig

import (
	"log"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
)

// AssistedConfig type contains assisted installer configuration.
type AssistedConfig struct {
	*config.GeneralConfig
}

// NewAssistedConfig returns instance of AssistedConfig type.
func NewAssistedConfig() *AssistedConfig {
	log.Print("Creating new AssistedConfig struct")

	var assistedConfig AssistedConfig

	assistedConfig.GeneralConfig = config.NewConfig()

	return &assistedConfig
}
