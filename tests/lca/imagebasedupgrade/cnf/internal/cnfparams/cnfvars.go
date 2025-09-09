package cnfparams

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/ibuparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{ibuparams.Label, Label}
	// WorkloadReadyTimeout represents the timeout for the workload to be ready.
	WorkloadReadyTimeout = 5 * time.Minute
)
