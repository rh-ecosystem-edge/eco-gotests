package kmminittools

import "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmconfig"

var (
	// ModulesConfig provides access to general configuration parameters.
	ModulesConfig *kmmconfig.ModulesConfig
)

func init() {
	ModulesConfig = kmmconfig.NewModulesConfig()
}
