package hwaccelconfig

import (
	"log"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
)

// HwAccelConfig contains Hardware Accelerators configuration.
type HwAccelConfig struct {
	*config.GeneralConfig
}

// NewHwAccelConfig returns instance of HwAccelConfig.
func NewHwAccelConfig() *HwAccelConfig {
	log.Print("Creating new HwAccelConfig struct")

	var hwaccelConfig HwAccelConfig

	hwaccelConfig.GeneralConfig = config.NewConfig()

	return &hwaccelConfig
}
