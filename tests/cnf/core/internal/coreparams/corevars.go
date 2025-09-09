package coreparams

import "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/cnfparams"

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{cnfparams.Label, Label}
	// OvnExternalBridge represents the name of OVN node external bridge.
	OvnExternalBridge = "br-ex"
)
