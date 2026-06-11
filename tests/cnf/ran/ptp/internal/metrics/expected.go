package metrics

import (
	"fmt"
	"strings"
)

// ExpectedClockState represents a single expected openshift_ptp_clock_state metric series. It describes a
// (process, interface, node) tuple that must be present in Prometheus for the configuration to be considered
// healthy.
//
// The Interface field stores the exact iface label value as it appears in Prometheus. For ptp4l this is the
// raw interface name (e.g., "ens1f0"); for dpll, gnss, and other processes this is the NIC name (e.g.,
// "ens2fx"); for phc2sys this is "CLOCK_REALTIME".
type ExpectedClockState struct {
	Process   PtpProcess
	Interface string
	Node      string
}

// String returns a human-readable representation of the expected clock state for logging and error messages.
func (e ExpectedClockState) String() string {
	return fmt.Sprintf("{process=%s, iface=%s, node=%s}", e.Process, e.Interface, e.Node)
}

// toQuery builds a MetricQuery for the expected clock state. It constructs the query directly rather than
// going through ClockStateQuery to avoid the ensureNIC() conversion, since ptp4l metrics use raw interface
// names while other processes use NIC names.
func (e ExpectedClockState) toQuery() MetricQuery[PtpClockState] {
	return MetricQuery[PtpClockState]{
		Metric: MetricClockState,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyProcess:   Equals(e.Process).ToAny(),
			KeyInterface: Equals(e.Interface).ToAny(),
			KeyNode:      Equals(e.Node).ToAny(),
		},
	}
}

// FormatExpectedClockStates returns a multi-line summary of the expected clock states for logging.
func FormatExpectedClockStates(expected []ExpectedClockState) string {
	if len(expected) == 0 {
		return "(none)"
	}

	var sb strings.Builder

	for i, e := range expected {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString("  - ")
		sb.WriteString(e.String())
	}

	return sb.String()
}

// deduplicateExpectedClockStates removes duplicate entries from the expected clock states slice. Two entries
// are considered duplicates if they have the same process, interface, and node.
func deduplicateExpectedClockStates(expected []ExpectedClockState) []ExpectedClockState {
	type key struct {
		process   PtpProcess
		iface     string
		node      string
	}

	seen := make(map[key]struct{})
	result := make([]ExpectedClockState, 0, len(expected))

	for _, entry := range expected {
		k := key{process: entry.Process, iface: entry.Interface, node: entry.Node}
		if _, exists := seen[k]; exists {
			continue
		}

		seen[k] = struct{}{}
		result = append(result, entry)
	}

	return result
}
