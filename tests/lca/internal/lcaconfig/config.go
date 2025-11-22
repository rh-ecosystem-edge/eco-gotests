package lcaconfig

import (
	"log"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
)

// LCAConfig type contains lifecycle agent configuration.
type LCAConfig struct {
	*config.GeneralConfig
}

// NewLCAConfig returns instance of LCAConfig type.
func NewLCAConfig() *LCAConfig {
	log.Print("Creating new LCAConfig struct")

	var lcaConfig LCAConfig

	lcaConfig.GeneralConfig = config.NewConfig()

	return &lcaConfig
}
