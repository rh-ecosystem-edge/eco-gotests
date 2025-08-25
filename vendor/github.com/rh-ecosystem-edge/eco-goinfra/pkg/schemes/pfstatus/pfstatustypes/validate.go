package pfstatustypes

import (
	"fmt"
	"reflect"
)

// InterfaceUniqueness validates that interfaces do not overlap for daemon sets that share nodes.
func InterfaceUniqueness(pfMonitor *PFLACPMonitor, pfMonitorList *PFLACPMonitorList) error {
	for _, monitor := range pfMonitorList.Items {
		if pfMonitor.Name == monitor.Name {
			continue
		}

		if monitor.Status.Degraded {
			continue
		}

		if monitor.Spec.NodeSelector == nil || pfMonitor.Spec.NodeSelector == nil || nodeSelectorOverlaps(pfMonitor.Spec.NodeSelector, monitor.Spec.NodeSelector) {
			if !areInterfacesUnique(pfMonitor.Spec.Interfaces, monitor.Spec.Interfaces) {
				return fmt.Errorf("interfaces %s conflict with the ones from PFLACPMonitor %s", pfMonitor.Spec.Interfaces, monitor.Name)
			}
		}
	}

	return nil
}

// nodeSelectorOverlaps checks if two node selectors overlap.
func nodeSelectorOverlaps(nodeSelector1, nodeSelector2 map[string]string) bool {
	for key, value := range nodeSelector1 {
		if v, ok := nodeSelector2[key]; ok && reflect.DeepEqual(value, v) {
			return true
		}
	}
	return false
}

// areInterfacesUnique checks if two string slices contain any common elements.
func areInterfacesUnique(intfs1, intfs2 []string) bool {
	seen := make(map[string]struct{})
	for _, item := range intfs2 {
		seen[item] = struct{}{}
	}

	for _, item := range intfs1 {
		if _, ok := seen[item]; ok {
			return false
		}
	}
	return true
}
