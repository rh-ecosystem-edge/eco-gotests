package helper

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
)

// GetCGUEvents lists CGU events in test namespace, optionally filtered by CGU name, sorted by event time.
func GetCGUEvents(cguName string) ([]*eventsv1.Event, error) {
	fieldSet := fields.Set{"regarding.kind": tsparams.CguRegardingKind}
	if cguName != "" {
		fieldSet["regarding.name"] = cguName
	}

	builders, err := events.ListEventV1s(HubAPIClient,
		runtimeclient.InNamespace(tsparams.TestNamespace),
		runtimeclient.MatchingFieldsSelector{Selector: fieldSet.AsSelector()})
	if err != nil {
		return nil, err
	}

	cguEvents := make([]*eventsv1.Event, 0, len(builders))

	for _, builder := range builders {
		if builder.Object == nil {
			continue
		}

		if cguName != "" && builder.Object.Regarding.Name != cguName {
			continue
		}

		cguEvents = append(cguEvents, builder.Object)
	}

	sort.Slice(cguEvents, func(i, j int) bool {
		return cguEvents[i].EventTime.Time.Before(cguEvents[j].EventTime.Time)
	})

	return cguEvents, nil
}

// ClearCGUEvents deletes all CGU events in test namespace via single DeleteCollection request.
func ClearCGUEvents() {
	if err := eventsv1.AddToScheme(HubAPIClient.Scheme()); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to attach the events/v1 scheme for clearing CGU events: %v", err)

		return
	}

	err := HubAPIClient.DeleteAllOf(context.TODO(), &eventsv1.Event{},
		runtimeclient.InNamespace(tsparams.TestNamespace),
		runtimeclient.MatchingFieldsSelector{Selector: fields.Set{"regarding.kind": tsparams.CguRegardingKind}.AsSelector()})
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to clear CGU events in the %s namespace: %v", tsparams.TestNamespace, err)

		return
	}

	klog.V(tsparams.LogLevel).Infof("Cleared CGU events in the %s namespace", tsparams.TestNamespace)
}

// EventMatcher defines expected event with reason, scope, and optional count.
type EventMatcher struct {
	Reason string // Event reason (e.g., CguStarted, CguSuccess)
	Scope  string // Event scope annotation (global, batch, cluster)
	Count  int    // Expected count: 0 = at least one, >0 = exact minimum
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
		if event.Reason == reason && event.Annotations[tsparams.CguEventScopeAnnotation] == scope {
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
// Returns true if all matchers appear in sequence; false otherwise.
func VerifyEventSequence(events []*eventsv1.Event, matchers []EventMatcher) bool {
	matcherIdx := 0
	matcherCounts := make(map[int]int) // Track count for each matcher

	for _, event := range events {
		if matcherIdx >= len(matchers) {
			break // All matchers satisfied
		}

		matcher := matchers[matcherIdx]
		eventScope := event.Annotations[tsparams.CguEventScopeAnnotation]

		// Check if event matches current matcher
		if event.Reason == matcher.Reason && eventScope == matcher.Scope {
			matcherCounts[matcherIdx]++

			// Move to next matcher if count requirement met
			if matcher.Count == 0 || matcherCounts[matcherIdx] >= matcher.Count {
				matcherIdx++
			}
		}
	}

	// All matchers must be satisfied
	return matcherIdx == len(matchers)
}

// cguDebugAnnotations lists annotation keys to include in debug output, ordered for stability.
var cguDebugAnnotations = []string{
	tsparams.CguMissingClustersAnnotation,
	tsparams.CguMissingClustersCountAnnotation,
	tsparams.CguMissingPoliciesAnnotation,
	tsparams.CguTimedoutClustersAnnotation,
}

// PrintCGUEvents logs all CGU events in test namespace for debugging (call from AfterEach).
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
		scope := event.Annotations[tsparams.CguEventScopeAnnotation]
		if scope == "" {
			scope = "-"
		}

		line := fmt.Sprintf("  %s  %-7s %-36s scope=%-7s regarding=%-24s",
			event.EventTime.Time.Format(time.RFC3339Nano), event.Type, event.Reason, scope, event.Regarding.Name)

		if annotations := formatCGUDebugAnnotations(event.Annotations); annotations != "" {
			line += " " + annotations
		}

		lines = append(lines, line+fmt.Sprintf(" note=%s", event.Note))
	}

	return strings.Join(lines, "\n")
}

// formatCGUDebugAnnotations renders present annotations as "annotations=[key=value, ...]" or empty string.
func formatCGUDebugAnnotations(annotations map[string]string) string {
	pairs := make([]string, 0, len(cguDebugAnnotations))

	for _, key := range cguDebugAnnotations {
		if value, ok := annotations[key]; ok && value != "" {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
	}

	if len(pairs) == 0 {
		return ""
	}

	return fmt.Sprintf("annotations=[%s]", strings.Join(pairs, ", "))
}

