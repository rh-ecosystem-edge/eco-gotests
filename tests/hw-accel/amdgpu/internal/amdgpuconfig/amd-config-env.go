package amdgpuconfig

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

// AMDConfig contains environment information related to amd tests.
type AMDConfig struct {
	AMDDriverVersion string `envconfig:"ECO_HWACCEL_AMD_DRIVER_VERSION"`
}

// NewAMDConfig returns instance of AMDConfig type.
func NewAMDConfig() *AMDConfig {
	log.Print("Creating new AMDConfig")

	AMDConfig := new(AMDConfig)

	err := envconfig.Process("eco_hwaccel_amd_", AMDConfig)
	if err != nil {
		log.Printf("failed to instantiate AMDConfig: %v", err)

		return nil
	}

	return AMDConfig
}
