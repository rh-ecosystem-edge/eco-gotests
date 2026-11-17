// Package tsparams contains DRA lifecycle test parameters.
package tsparams

import "time"

const (
	// ReconcileTimeout allows node reconfiguration and DaemonSet rollout to complete.
	ReconcileTimeout = 15 * time.Minute
	// PollInterval is the interval between resource observations.
	PollInterval = 5 * time.Second
	// InitialVFs is the initial policy VF count.
	InitialVFs = 2
	// UpdatedVFs is the in-place policy update VF count.
	UpdatedVFs = 4
)
