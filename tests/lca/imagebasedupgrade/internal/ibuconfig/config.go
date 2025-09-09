package ibuconfig

import (
	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/ibuparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaconfig"
)

// IBUConfig type contains imagebasedupgrade configuration.
type IBUConfig struct {
	*lcaconfig.LCAConfig
}

// NewIBUConfig returns instance of IBUConfig type.
func NewIBUConfig() *IBUConfig {
	glog.V(ibuparams.IBULogLevel).Info("Creating new IBUConfig struct")

	var ibuConfig IBUConfig
	ibuConfig.LCAConfig = lcaconfig.NewLCAConfig()

	return &ibuConfig
}
