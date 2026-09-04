package helper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
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

// =============================================================================
// SECTION 1: Production Event Helpers (PERMANENT)
// Uses constants from tsparams package
// =============================================================================

// GetCGUEvents lists CGU events in test namespace, optionally filtered by CGU name, sorted by creation timestamp.
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

// =============================================================================
// SECTION 2: DEBUG HELPERS - Remove after test verification (Phase 8)
// TODO: Delete entire section once formal assertions validated
// =============================================================================

// Debug-only constants (not moved to tsparams - will be deleted)
const (
	// DefaultCGUEventCheckpointDelaySeconds is default wait before listing events in checkpoint.
	DefaultCGUEventCheckpointDelaySeconds = 10
	// talmCguCheckpointLogTag is log marker for checkpoint summary lines.
	talmCguCheckpointLogTag = "TALM_CGU_CHECKPOINT"
	// talmCguEventLogTag is log marker for checkpoint event lines.
	talmCguEventLogTag = "TALM_CGU_EVENT"
)

// cguDebugAnnotations lists annotation keys to check on events, ordered for stable debug output.
var cguDebugAnnotations = []string{
	tsparams.CguMissingClustersAnnotation,
	tsparams.CguMissingClustersCountAnnotation,
	tsparams.CguMissingPoliciesAnnotation,
	tsparams.CguTimedoutClustersAnnotation,
}

// cguDebugAnnotationFields maps annotations to logfmt-friendly field names.
var cguDebugAnnotationFields = []struct {
	annotationKey string
	logfmtField   string
}{
	{tsparams.CguMissingClustersAnnotation, "missing_clusters"},
	{tsparams.CguMissingClustersCountAnnotation, "missing_clusters_count"},
	{tsparams.CguMissingPoliciesAnnotation, "missing_policies"},
	{tsparams.CguTimedoutClustersAnnotation, "timedout_clusters"},
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

// PrintCGUEventsCheckpoint logs CGU events at test milestone with tcID, checkpoint label, and expected events.
func PrintCGUEventsCheckpoint(tcID, checkpoint, cguName string, expected ...string) {
	time.Sleep(DefaultCGUEventCheckpointDelaySeconds * time.Second)

	cguEvents, err := GetCGUEvents(cguName)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("%s tc=%s checkpoint=%s cgu=%s error=%s",
			talmCguCheckpointLogTag, tcID, logfmtQuote(checkpoint), cguName, logfmtQuote(err.Error()))

		return
	}

	klog.V(tsparams.LogLevel).Infof("%s tc=%s checkpoint=%s cgu=%s expected=%s event_count=%d",
		talmCguCheckpointLogTag, tcID, logfmtQuote(checkpoint), cguName,
		logfmtQuote(strings.Join(expected, "; ")), len(cguEvents))

	for _, event := range cguEvents {
		klog.V(tsparams.LogLevel).Infof("%s", formatCGUEventLogfmt(tcID, checkpoint, cguName, event))
	}
}

// formatCGUEventLogfmt renders event as TALM_CGU_EVENT logfmt line with checkpoint context.
func formatCGUEventLogfmt(tcID, checkpoint, cguName string, event *eventsv1.Event) string {
	scope := event.Annotations[tsparams.CguEventScopeAnnotation]
	if scope == "" {
		scope = "-"
	}

	fields := []string{
		talmCguEventLogTag,
		"tc=" + tcID,
		"checkpoint=" + logfmtQuote(checkpoint),
		"cgu=" + cguName,
		"ts=" + event.EventTime.Time.Format(time.RFC3339Nano),
		"type=" + event.Type,
		"reason=" + event.Reason,
		"scope=" + scope,
	}

	for _, annotation := range cguDebugAnnotationFields {
		if value, ok := event.Annotations[annotation.annotationKey]; ok && value != "" {
			fields = append(fields, annotation.logfmtField+"="+logfmtQuote(value))
		}
	}

	return strings.Join(append(fields, "note="+logfmtQuote(event.Note)), " ")
}

// logfmtQuote double-quotes and escapes value for logfmt output.
func logfmtQuote(value string) string {
	return strconv.Quote(value)
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

// =============================================================================
// SECTION 3: DEPRECATED FALLBACK - oc command implementations (Phase 8 removal)
// TODO: Remove if not needed after validation
// Merged from eventsdebug.go as safety net only
// =============================================================================

// fallbackOcPaths lists oc binary locations checked when not found in PATH.
var fallbackOcPaths = []string{"/clusterconfigs/oc", "/usr/local/bin/oc", "/usr/bin/oc"}

// ClearCGUEventsViaOc deletes CGU events via oc command (deprecated fallback).
func ClearCGUEventsViaOc() {
	output, err := runOcCommand("delete", "event.v1.events.k8s.io",
		"-n", tsparams.TestNamespace,
		"--field-selector", "regarding.kind==ClusterGroupUpgrade",
		"--ignore-not-found")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to clear CGU events in the %s namespace: %v\noutput: %s", tsparams.TestNamespace, err, output)
	} else {
		klog.V(tsparams.LogLevel).Infof("Cleared CGU events in the %s namespace", tsparams.TestNamespace)
	}
}

// PrintCGUEventsViaOc prints CGU events via oc command (deprecated fallback).
func PrintCGUEventsViaOc() {
	output, err := runOcCommand("get", "event.v1.events.k8s.io",
		"-n", tsparams.TestNamespace,
		"--field-selector", "regarding.kind==ClusterGroupUpgrade",
		"--sort-by", "{.metadata.creationTimestamp}")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to get CGU events in the %s namespace: %v\noutput: %s", tsparams.TestNamespace, err, output)
	} else {
		klog.V(tsparams.LogLevel).Infof("CGU events in the %s namespace:\n%s", tsparams.TestNamespace, output)
	}
}

// runOcCommand executes oc against hub cluster and returns combined output.
func runOcCommand(args ...string) ([]byte, error) {
	ocPath := resolveOcPath()

	hubKubeconfig := RANConfig.HubKubeconfig
	if _, statErr := os.Stat(hubKubeconfig); statErr != nil {
		klog.V(tsparams.LogLevel).Infof("Hub kubeconfig %q not found or inaccessible: %v", hubKubeconfig, statErr)
	}

	cmd := exec.Command(ocPath, args...)

	cmd.Env = append(os.Environ(), "KUBECONFIG="+hubKubeconfig)

	return cmd.CombinedOutput()
}

// resolveOcPath finds oc binary via PATH or fallback locations.
func resolveOcPath() string {
	if pathFromLookup, lookErr := exec.LookPath("oc"); lookErr == nil {
		return pathFromLookup
	}

	for _, candidate := range fallbackOcPaths {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	klog.V(tsparams.LogLevel).Infof(
		"Could not resolve oc from PATH or fallback locations %v, defaulting to bare 'oc'", fallbackOcPaths)

	return "oc"
}
