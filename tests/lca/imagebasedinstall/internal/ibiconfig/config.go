package ibiconfig

import (
	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/internal/ibiparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaconfig"
)

// IBIConfig type contains imagebasedupgrade configuration.
type IBIConfig struct {
	*lcaconfig.LCAConfig
}

// NewIBIConfig returns instance of IBIConfig type.
func NewIBIConfig() *IBIConfig {
	glog.V(ibiparams.IBILogLevel).Info("Creating new IBIConfig struct")

	var ibiConfig IBIConfig
	ibiConfig.LCAConfig = lcaconfig.NewLCAConfig()

	return &ibiConfig
}
