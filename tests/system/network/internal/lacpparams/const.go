package lacpparams

import (
	"time"
)

const (
	// LabelLACPBondStability represents the Ginkgo label for LACP bond stability tests.
	LabelLACPBondStability = "lacp-bond-stability"

	// DefaultTimeout is the timeout used for waiting on LACP state convergence.
	DefaultTimeout = 5 * time.Minute

	// DefaultInterval is the polling interval for LACP state checks.
	DefaultInterval = 10 * time.Second

	// RebootTimeout is the timeout used for waiting on node reboot completion.
	RebootTimeout = 10 * time.Minute

	// RebootInterval is the polling interval during node reboot.
	RebootInterval = 30 * time.Second

	// LACPStateCollecting is the LACP port state bit for Collecting (bit 4).
	LACPStateCollecting = 0x10

	// LACPStateDistributing is the LACP port state bit for Distributing (bit 5).
	LACPStateDistributing = 0x20

	// MinLACPSlowTimeoutSeconds is the minimum acceptable timeout for slow LACP rate.
	MinLACPSlowTimeoutSeconds = 60

	// MinLACPFastTimeoutSeconds is the minimum acceptable timeout for fast LACP rate.
	MinLACPFastTimeoutSeconds = 1

	// DefaultBondInterfaceName is the fallback bond interface name.
	DefaultBondInterfaceName = "bond0"

	// EnvLACPBondInterface is the env var to override the bond interface name.
	EnvLACPBondInterface = "ECO_LACP_BOND_INTERFACE"

	// LogLevel configures logging level for LACP bond stability tests.
	LogLevel = 90
)

// Labels represents the range of labels for LACP bond stability test selection.
var Labels = []string{LabelLACPBondStability}
