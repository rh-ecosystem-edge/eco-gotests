package helper

import (
	"context"
	"fmt"
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

// cguEventScopeAnnotation is the annotation TALM sets on ClusterGroupUpgrade events to indicate whether the event is
// global, batch, or cluster scoped. See TALM-events-test-plan.md.
const cguEventScopeAnnotation = "cgu.openshift.io/event-type"

// cguRegardingKind is the regarding.kind value TALM sets on every ClusterGroupUpgrade event.
const cguRegardingKind = "ClusterGroupUpgrade"

// Annotation keys TALM sets on specific ClusterGroupUpgrade event reasons, beyond the scope annotation. See the
// "Key fields on each event object" table in TALM-events-test-plan.md. Each is only expected on the reason(s)
// noted below; formatCGUEvents surfaces whichever of them are actually present on a given event so debug output
// can confirm presence/values ahead of writing real annotation assertions.
const (
	// cguMissingClustersAnnotation is set on CguValidationFailure events reporting missing spoke clusters.
	cguMissingClustersAnnotation = "cgu.openshift.io/missing-clusters"
	// cguMissingClustersCountAnnotation accompanies cguMissingClustersAnnotation with a count of missing clusters.
	cguMissingClustersCountAnnotation = "cgu.openshift.io/missing-clusters-count"
	// cguMissingPoliciesAnnotation is set on CguValidationFailure events reporting missing managed policies.
	cguMissingPoliciesAnnotation = "cgu.openshift.io/missing-policies"
	// cguTimedoutClustersAnnotation is set on CguTimedout events reporting which clusters timed out.
	cguTimedoutClustersAnnotation = "cgu.openshift.io/timedout-clusters"
)

// cguDebugAnnotations lists the annotation keys formatCGUEvents checks for on each event, in the order they
// should be printed when present. Kept as an ordered slice (rather than ranging over a map) so debug output is
// stable across runs, which matters when diffing jenkins logs between runs.
var cguDebugAnnotations = []string{
	cguMissingClustersAnnotation,
	cguMissingClustersCountAnnotation,
	cguMissingPoliciesAnnotation,
	cguTimedoutClustersAnnotation,
}

// cguDebugAnnotationFields maps each annotation in cguDebugAnnotations to a bare logfmt field name (dots and
// slashes aren't friendly logfmt keys) for formatCGUEventLogfmt. Ordered for stable output, same reason as
// cguDebugAnnotations above.
var cguDebugAnnotationFields = []struct {
	annotationKey string
	logfmtField   string
}{
	{cguMissingClustersAnnotation, "missing_clusters"},
	{cguMissingClustersCountAnnotation, "missing_clusters_count"},
	{cguMissingPoliciesAnnotation, "missing_policies"},
	{cguTimedoutClustersAnnotation, "timedout_clusters"},
}

// DefaultCGUEventCheckpointDelaySeconds is the wait before listing events in PrintCGUEventsCheckpoint when
// delaySeconds is zero or negative (pass 0 at call sites to use this default).
const DefaultCGUEventCheckpointDelaySeconds = 10

// talmCguCheckpointLogTag and talmCguEventLogTag are stable, literal markers prefixed to every
// PrintCGUEventsCheckpoint log line. Unlike grepping for "cguevents.go:<line>", these tags don't shift when this
// file is edited, and they let a log consumer (e.g. `rg 'TALM_CGU_EVENT tc=47948'`) filter or extract exactly the
// lines it wants without multi-line context flags — see TALM-events-implementation-plan.md's debug-output notes.
const (
	talmCguCheckpointLogTag = "TALM_CGU_CHECKPOINT"
	talmCguEventLogTag      = "TALM_CGU_EVENT"
)

// GetCGUEvents lists events.k8s.io/v1 events regarding ClusterGroupUpgrade resources in tsparams.TestNamespace on
// the hub cluster, sorted by creation timestamp (oldest first). When cguName is non-empty, results are further
// filtered to events regarding that specific CGU (filtering in-process in addition to the server-side field
// selector, in case regarding.name filtering isn't honored by a given API server version). This mirrors the
// GetCGUEvents helper described in TALM-events-implementation-plan.md and is used here as a debug preview of real
// TALM event behavior ahead of implementing real assertions with it.
func GetCGUEvents(cguName string) ([]*eventsv1.Event, error) {
	fieldSet := fields.Set{"regarding.kind": cguRegardingKind}
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
		return cguEvents[i].CreationTimestamp.Before(&cguEvents[j].CreationTimestamp)
	})

	return cguEvents, nil
}

// PrintCGUEvents is a temporary debugging helper that logs all ClusterGroupUpgrade events in tsparams.TestNamespace
// on the hub cluster. It is meant to be called from AfterEach blocks as a full-state safety-net snapshot, so events
// are captured even when a test fails for reasons unrelated to events (AfterEach always runs, even after a failed
// Expect). See TALM-events-test-plan.md / TALM-events-implementation-plan.md while investigating TALM CGU event
// behavior; this and PrintCGUEventsCheckpoint should be removed once real event assertions are implemented.
func PrintCGUEvents() {
	cguEvents, err := GetCGUEvents("")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to get CGU events in the %s namespace: %v", tsparams.TestNamespace, err)

		return
	}

	klog.V(tsparams.LogLevel).Infof(
		"CGU events in the %s namespace:\n%s", tsparams.TestNamespace, formatCGUEvents(cguEvents))
}

// PrintCGUEventsCheckpoint is a temporary debugging helper that logs the ClusterGroupUpgrade events currently
// present for cguName as one TALM_CGU_CHECKPOINT summary line followed by one TALM_CGU_EVENT line per event,
// logfmt-style (space-separated key=value pairs, double-quoted free-text values). Every line is self-contained
// (repeats tc/checkpoint/cgu) and independently greppable, so a log consumer never needs multi-line context to
// pull out just this checkpoint's events — e.g. `rg 'TALM_CGU_EVENT tc=47948'` or `rg -o 'reason=\S+'`.
//
// tcID is the Polarion test case ID (e.g. "47948") so a checkpoint is self-identifying without cross-referencing
// ginkgo's [It]/failure-summary output. checkpoint labels the test milestone just reached, and expected lists the
// event reason/scope combinations TALM-events-test-plan.md / TALM-events-implementation-plan.md say should be
// present at this point, for human comparison against actual (this helper does not assert; see package doc).
//
// delaySeconds controls how long to wait before listing events, giving TALM time to emit events after a condition
// becomes true. Pass 0 (or any value <= 0) to use DefaultCGUEventCheckpointDelaySeconds (10); pass a positive
// integer to override.
//
// This never fails the test: if fetching events errors, the error is logged and the function returns without
// panicking, so a broken event fetch can never mask or replace a real test failure. Call it between a milestone
// wait (e.g. WaitForCondition) and that wait's own Expect(err) assertion, so the checkpoint is captured on a
// best-effort basis even if the wait itself timed out.
func PrintCGUEventsCheckpoint(tcID, checkpoint, cguName string, delaySeconds int, expected ...string) {
	delay := delaySeconds
	if delay <= 0 {
		delay = DefaultCGUEventCheckpointDelaySeconds
	}

	time.Sleep(time.Duration(delay) * time.Second)

	cguEvents, err := GetCGUEvents(cguName)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("%s tc=%s checkpoint=%s cgu=%s delay_seconds=%d error=%s",
			talmCguCheckpointLogTag, tcID, logfmtQuote(checkpoint), cguName, delay, logfmtQuote(err.Error()))

		return
	}

	klog.V(tsparams.LogLevel).Infof("%s tc=%s checkpoint=%s cgu=%s delay_seconds=%d expected=%s event_count=%d",
		talmCguCheckpointLogTag, tcID, logfmtQuote(checkpoint), cguName, delay,
		logfmtQuote(strings.Join(expected, "; ")), len(cguEvents))

	for _, event := range cguEvents {
		klog.V(tsparams.LogLevel).Infof("%s", formatCGUEventLogfmt(tcID, checkpoint, cguName, event))
	}
}

// formatCGUEventLogfmt renders a single event as one TALM_CGU_EVENT logfmt line carrying the checkpoint's
// tc/checkpoint/cgu context plus that event's timestamp, type, reason, scope, any negative-path annotations from
// cguDebugAnnotationFields that are present, and note. Kept separate from PrintCGUEventsCheckpoint so the
// per-event line format can be reused or tested independently of the surrounding checkpoint/summary logic.
func formatCGUEventLogfmt(tcID, checkpoint, cguName string, event *eventsv1.Event) string {
	scope := event.Annotations[cguEventScopeAnnotation]
	if scope == "" {
		scope = "-"
	}

	fields := []string{
		talmCguEventLogTag,
		"tc=" + tcID,
		"checkpoint=" + logfmtQuote(checkpoint),
		"cgu=" + cguName,
		"ts=" + event.CreationTimestamp.Format(time.RFC3339),
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

// logfmtQuote double-quotes value and escapes any embedded quotes/backslashes/newlines (via strconv.Quote), so
// free-text fields in TALM_CGU_CHECKPOINT/TALM_CGU_EVENT lines (checkpoint labels, expected lists, annotation
// values, notes) are always a single well-formed logfmt token even when they contain spaces or commas.
func logfmtQuote(value string) string {
	return strconv.Quote(value)
}

// ClearCGUEvents deletes all ClusterGroupUpgrade events in tsparams.TestNamespace on the hub cluster. It is meant to
// be called from BeforeEach blocks so that events fetched later in the test only reflect what happened during the
// current test rather than accumulating across the whole suite run.
//
// This issues a single server-side DeleteCollection request (via the embedded controller-runtime client's
// DeleteAllOf, the Go equivalent of `oc delete events -n <namespace> --field-selector=...`) rather than listing
// events and deleting each one individually. Deleting one event at a time via a builder's Delete() sends a per-object
// DELETE request that silently treats a 404 as success, which can mask cases where nothing was actually deleted;
// DeleteAllOf avoids that by scoping a single collection-level delete to the namespace and field selector, matching
// the manual `oc delete events` workflow this replaces. Deletion is best-effort and logged rather than fatal, since
// it is a debug convenience rather than part of the test's real setup.
func ClearCGUEvents() {
	if err := eventsv1.AddToScheme(HubAPIClient.Scheme()); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to attach the events/v1 scheme for clearing CGU events: %v", err)

		return
	}

	err := HubAPIClient.DeleteAllOf(context.TODO(), &eventsv1.Event{},
		runtimeclient.InNamespace(tsparams.TestNamespace),
		runtimeclient.MatchingFieldsSelector{Selector: fields.Set{"regarding.kind": cguRegardingKind}.AsSelector()})
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to clear CGU events in the %s namespace: %v", tsparams.TestNamespace, err)

		return
	}

	klog.V(tsparams.LogLevel).Infof("Cleared CGU events in the %s namespace", tsparams.TestNamespace)
}

// formatCGUEvents renders events as a compact, human-readable multi-line summary for debug logging: creation time,
// type, reason, scope annotation, regarding name, the negative-path annotations in cguDebugAnnotations (when
// present), and note per event.
func formatCGUEvents(cguEvents []*eventsv1.Event) string {
	if len(cguEvents) == 0 {
		return "  (none)"
	}

	lines := make([]string, 0, len(cguEvents))

	for _, event := range cguEvents {
		scope := event.Annotations[cguEventScopeAnnotation]
		if scope == "" {
			scope = "-"
		}

		line := fmt.Sprintf("  %s  %-7s %-36s scope=%-7s regarding=%-24s",
			event.CreationTimestamp.Format(time.RFC3339), event.Type, event.Reason, scope, event.Regarding.Name)

		if annotations := formatCGUDebugAnnotations(event.Annotations); annotations != "" {
			line += " " + annotations
		}

		lines = append(lines, line+fmt.Sprintf(" note=%s", event.Note))
	}

	return strings.Join(lines, "\n")
}

// formatCGUDebugAnnotations renders whichever keys in cguDebugAnnotations are present and non-empty on
// annotations as a single "annotations=[key=value, ...]" segment, or "" when none of them are present. Kept
// separate from formatCGUEvents so scope formatting and negative-path annotation formatting don't get tangled
// into one function.
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
