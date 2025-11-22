package ibuconfig

import (
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/ibuparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaconfig"
	"k8s.io/klog/v2"
)

// IBUConfig type contains imagebasedupgrade configuration.
type IBUConfig struct {
	*lcaconfig.LCAConfig
}

// NewIBUConfig returns instance of IBUConfig type.
func NewIBUConfig() *IBUConfig {
	klog.V(ibuparams.IBULogLevel).Info("Creating new IBUConfig struct")

	var ibuConfig IBUConfig

	ibuConfig.LCAConfig = lcaconfig.NewLCAConfig()

	return &ibuConfig
}
