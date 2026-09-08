package helper

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
)

// EventMatcher defines expected event with reason, scope, and optional count.
type EventMatcher struct {
	Reason string // Event reason (e.g., CguStarted, CguSuccess)
	Scope  string // Event scope annotation (global, batch, cluster)
	Count  int    // Expected count: 0 = at least one, >0 = exact minimum
	Strict bool   // When true with Count>1, all occurrences must precede the next matcher
}

// cguAnnotations lists annotation keys to include in debug output, ordered for stability.
var cguAnnotations = []string{
	tsparams.CguMissingClustersAnnotation,
	tsparams.CguMissingClustersCountAnnotation,
	tsparams.CguMissingPoliciesAnnotation,
	tsparams.CguTimedoutClustersAnnotation,
}

// GetCGUEvents lists CGU events in test namespace, optionally filtered by CGU name, sorted by event time.
// Filtering by regarding.kind and regarding.name is done client-side because events.k8s.io/v1 does not
// register regarding.* as server-selectable fields on all apiserver versions.
func GetCGUEvents(cguName string) ([]*eventsv1.Event, error) {
	builders, err := events.ListEventV1s(HubAPIClient,
		runtimeclient.InNamespace(tsparams.TestNamespace))
	if err != nil {
		return nil, err
	}

	cguEvents := make([]*eventsv1.Event, 0, len(builders))

	for _, builder := range builders {
		if builder.Object == nil {
			continue
		}

		if builder.Object.Regarding.Kind != tsparams.CguRegardingKind {
			continue
		}

		if cguName != "" && builder.Object.Regarding.Name != cguName {
			continue
		}

		cguEvents = append(cguEvents, builder.Object)
	}

	sort.SliceStable(cguEvents, func(i, j int) bool {
		if !cguEvents[i].EventTime.Time.Equal(cguEvents[j].EventTime.Time) {
			return cguEvents[i].EventTime.Time.Before(cguEvents[j].EventTime.Time)
		}

		if cguEvents[i].Reason != cguEvents[j].Reason {
			return cguEvents[i].Reason < cguEvents[j].Reason
		}

		return cguEvents[i].Name < cguEvents[j].Name
	})

	return cguEvents, nil
}

// ClearCGUEvents deletes all CGU events in test namespace by listing and deleting each event individually.
// Uses client-side filtering to avoid depending on regarding.* server-side field selectors.
func ClearCGUEvents() error {
	builders, err := events.ListEventV1s(HubAPIClient,
		runtimeclient.InNamespace(tsparams.TestNamespace))
	if err != nil {
		return fmt.Errorf("failed to list events in the %s namespace: %w", tsparams.TestNamespace, err)
	}

	deleted := 0

	for _, builder := range builders {
		if builder.Object == nil {
			continue
		}

		if builder.Object.Regarding.Kind != tsparams.CguRegardingKind {
			continue
		}

		if err := builder.Delete(); err != nil {
			return fmt.Errorf("failed to delete CGU event %s in the %s namespace: %w",
				builder.Object.Name, tsparams.TestNamespace, err)
		}

		deleted++
	}

	klog.V(tsparams.LogLevel).Infof("Cleared %d CGU events in the %s namespace", deleted, tsparams.TestNamespace)

	return nil
}

// FindEventsByReason filters events by reason, returning all matching events.
func FindEventsByReason(events []*eventsv1.Event, reason string) []*eventsv1.Event {
	matches := make([]*eventsv1.Event, 0)

	for _, event := range events {
		if event.Reason == reason {
			matches = append(matches, event)
		}
	}

	return matches
}

// FindEventsByReasonAndScope filters events by both reason AND scope annotation.
func FindEventsByReasonAndScope(events []*eventsv1.Event, reason, scope string) []*eventsv1.Event {
	matches := make([]*eventsv1.Event, 0)

	for _, event := range events {
		if event.Reason == reason && event.Annotations[tsparams.CguEventTypeAnnotation] == scope {
			matches = append(matches, event)
		}
	}

	return matches
}

// HasEventWithAnnotation checks if any event has the specified annotation key present.
func HasEventWithAnnotation(events []*eventsv1.Event, annotationKey string) bool {
	for _, event := range events {
		if _, exists := event.Annotations[annotationKey]; exists {
			return true
		}
	}

	return false
}

// GetEventAnnotation retrieves annotation value from first event matching reason, returns value and exists flag.
func GetEventAnnotation(events []*eventsv1.Event, reason, annotationKey string) (string, bool) {
	for _, event := range events {
		if event.Reason == reason {
			if value, exists := event.Annotations[annotationKey]; exists {
				return value, true
			}
		}
	}

	return "", false
}

// CountEventsByReasonAndScope counts events matching both reason and scope.
func CountEventsByReasonAndScope(events []*eventsv1.Event, reason, scope string) int {
	return len(FindEventsByReasonAndScope(events, reason, scope))
}

// VerifyEventSequence checks that events appear in expected order (allows gaps and extras).
// For each matcher, all remaining events from the current position are scanned to count total
// occurrences of the (reason, scope) pair. This allows interleaved events from concurrent
// clusters while still enforcing cross-phase ordering between different matchers.
//
// Position advancement after a satisfied matcher depends on the Strict field:
//   - Strict=false (default): advances past the first match, allowing later occurrences to
//     interleave with events matched by subsequent matchers (e.g., concurrent cluster events).
//   - Strict=true: advances past the last match, ensuring all counted occurrences precede
//     any event matched by the next matcher.
//
// Returns (true, nil) if all matchers are satisfied, or (false, error) describing which
// matcher failed and how many occurrences were found.
func VerifyEventSequence(events []*eventsv1.Event, matchers []EventMatcher) (bool, error) {
	pos := 0

	for idx, matcher := range matchers {
		requiredCount := matcher.Count
		if requiredCount == 0 {
			requiredCount = 1
		}

		found := 0
		firstMatch := -1
		lastMatch := -1

		for i := pos; i < len(events); i++ {
			eventScope := events[i].Annotations[tsparams.CguEventTypeAnnotation]

			if events[i].Reason == matcher.Reason && eventScope == matcher.Scope {
				if firstMatch == -1 {
					firstMatch = i
				}

				lastMatch = i
				found++
			}
		}

		if found < requiredCount {
			return false, fmt.Errorf("matcher %d (%s/scope=%s) failed: found %d, expected at least %d (from event position %d)",
				idx, matcher.Reason, matcher.Scope, found, requiredCount, pos)
		}

		if matcher.Strict {
			pos = lastMatch + 1
		} else {
			pos = firstMatch + 1
		}
	}

	return true, nil
}

// PrintCGUEvents logs all CGU events in test namespace (call from AfterEach).
func PrintCGUEvents() {
	cguEvents, err := GetCGUEvents("")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to get CGU events in the %s namespace: %v", tsparams.TestNamespace, err)

		return
	}

	klog.V(tsparams.LogLevel).Infof(
		"CGU events in the %s namespace:\n%s", tsparams.TestNamespace, formatCGUEvents(cguEvents))
}

// formatCGUEvents renders events as multi-line summary with timestamps, types, reasons, and annotations.
func formatCGUEvents(cguEvents []*eventsv1.Event) string {
	if len(cguEvents) == 0 {
		return "  (none)"
	}

	lines := make([]string, 0, len(cguEvents))

	for _, event := range cguEvents {
		scope := event.Annotations[tsparams.CguEventTypeAnnotation]
		if scope == "" {
			scope = "-"
		}

		line := fmt.Sprintf("  %s  %-7s %-36s scope=%-7s regarding=%-24s",
			event.EventTime.Time.Format(time.RFC3339Nano), event.Type, event.Reason, scope, event.Regarding.Name)

		if annotations := formatCGUAnnotations(event.Annotations); annotations != "" {
			line += " " + annotations
		}

		lines = append(lines, line+fmt.Sprintf(" note=%s", event.Note))
	}

	return strings.Join(lines, "\n")
}

// formatCGUAnnotations renders present annotations as "annotations=[key=value, ...]" or empty string.
func formatCGUAnnotations(annotations map[string]string) string {
	pairs := make([]string, 0, len(cguAnnotations))

	for _, key := range cguAnnotations {
		if value, ok := annotations[key]; ok && value != "" {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
	}

	if len(pairs) == 0 {
		return ""
	}

	return fmt.Sprintf("annotations=[%s]", strings.Join(pairs, ", "))
}
