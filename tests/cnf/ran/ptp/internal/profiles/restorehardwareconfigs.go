package profiles

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
)

// HardwareConfigSnapshot is a saved copy of every HardwareConfig in the cluster, ready to be restored.
type HardwareConfigSnapshot []*ptp.HardwareConfigBuilder

// SaveHardwareConfigs returns a list of all HardwareConfigs in the cluster. Returns an empty list when
// the CRD is not installed (pre-4.22 clusters).
func SaveHardwareConfigs(client *clients.Settings) (HardwareConfigSnapshot, error) {
	hwConfigList, err := ptp.ListHardwareConfigs(client)
	if err != nil {
		return nil, fmt.Errorf("failed to list HardwareConfigs: %w", err)
	}

	return hwConfigList, nil
}

// RestoreHardwareConfigs restores the HardwareConfigs from the list to the cluster, updating only those
// whose spec differs from what's currently live. Returns the HardwareConfigs that were changed.
func RestoreHardwareConfigs(
	client *clients.Settings, hwConfigList []*ptp.HardwareConfigBuilder,
) ([]*ptp.HardwareConfigBuilder, error) {
	var (
		changed []*ptp.HardwareConfigBuilder
		errs    []error
	)

	for _, hwConfig := range hwConfigList {
		latest, err := hwConfig.Get()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get latest HardwareConfig %s in namespace %s: %w",
				hwConfig.Definition.Name, hwConfig.Definition.Namespace, err))

			continue
		}

		if reflect.DeepEqual(hwConfig.Definition.Spec, latest.Spec) {
			continue
		}

		_, err = hwConfig.Update()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to update HardwareConfig %s in namespace %s: %w",
				hwConfig.Definition.Name, hwConfig.Definition.Namespace, err))

			continue
		}

		changed = append(changed, hwConfig)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to restore HardwareConfigs: %w", errors.Join(errs...))
	}

	return changed, nil
}

// Restore reverts every changed HardwareConfig back to what was captured, waiting for the daemon to
// reload on each affected node only if anything actually changed.
func (snapshot HardwareConfigSnapshot) Restore(client *clients.Settings) error {
	startTime := time.Now()

	changed, err := RestoreHardwareConfigs(client, snapshot)
	if err != nil {
		return fmt.Errorf("failed to restore HardwareConfigs: %w", err)
	}

	nodeNames := make(map[string]struct{}, len(changed))

	for _, hwConfig := range changed {
		for _, matchedNode := range hwConfig.Definition.Status.MatchedNodes {
			nodeNames[matchedNode.NodeName] = struct{}{}
		}
	}

	for nodeName := range nodeNames {
		err := daemonlogs.WaitForHardwareConfigLoad(client, nodeName,
			daemonlogs.WithStartTime(startTime), daemonlogs.WithTimeout(5*time.Minute))
		if err != nil {
			return fmt.Errorf("failed to wait for HardwareConfig reload on node %s: %w", nodeName, err)
		}
	}

	return nil
}
