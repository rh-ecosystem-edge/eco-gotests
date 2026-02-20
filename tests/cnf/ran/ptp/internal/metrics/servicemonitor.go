package metrics

import (
	"fmt"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/monitoring"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
)

// UpdatePtpServiceMonitorInterval pulls the PTP operator's ServiceMonitor, saves a deep copy of the original
// definition, updates all endpoint scrape intervals to the provided value, and returns the saved copy. The caller
// should use RestorePtpServiceMonitor with the returned value to restore the original state.
func UpdatePtpServiceMonitorInterval(
	client *clients.Settings, interval monv1.Duration) (*monv1.ServiceMonitor, error) {
	builder, err := monitoring.Pull(client, ranparam.PtpServiceMonitorName, ranparam.PtpOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PTP ServiceMonitor: %w", err)
	}

	saved := builder.Definition.DeepCopy()

	for i := range builder.Definition.Spec.Endpoints {
		builder.Definition.Spec.Endpoints[i].Interval = interval
	}

	_, err = builder.Update()
	if err != nil {
		return nil, fmt.Errorf("failed to update PTP ServiceMonitor interval: %w", err)
	}

	return saved, nil
}

// RestorePtpServiceMonitor restores the PTP operator's ServiceMonitor to the provided saved state. The saved
// ServiceMonitor should be the return value from UpdatePtpServiceMonitorInterval.
func RestorePtpServiceMonitor(client *clients.Settings, saved *monv1.ServiceMonitor) error {
	if saved == nil {
		return fmt.Errorf("cannot restore nil ServiceMonitor")
	}

	builder, err := monitoring.Pull(client, ranparam.PtpServiceMonitorName, ranparam.PtpOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull PTP ServiceMonitor for restoration: %w", err)
	}

	// Preserve the current resource version so the update succeeds, then restore everything else.
	saved.ResourceVersion = builder.Object.ResourceVersion
	builder.Definition = saved

	_, err = builder.Update()
	if err != nil {
		return fmt.Errorf("failed to restore PTP ServiceMonitor: %w", err)
	}

	return nil
}
