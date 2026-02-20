package stability

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
)

// DefaultOffsetThresholdAbs is the default absolute offset threshold used by stability analysis. It is measured in
// nanoseconds.
const DefaultOffsetThresholdAbs int64 = 100

// OffsetStatistics captures max, min and average absolute offsets.
type OffsetStatistics struct {
	MaxAbs      int64
	MinAbs      int64
	AvgAbs      float64
	SampleCount int
}

// StateTransition describes a ptp4l state change between adjacent parsed entries.
type StateTransition struct {
	From string
	To   string
	Raw  string
}

// AnalysisResult is the pass/fail decision output of stability analysis.
type AnalysisResult struct {
	Passed  bool
	Errors  int
	Details []string

	PTP4LStats   OffsetStatistics
	PHC2SYSStats OffsetStatistics

	PTP4LStartCount int

	FaultyLines      []string
	TimeoutLines     []string
	StateTransitions []StateTransition

	PTP4LThresholdViolations   []daemonlogs.PTP4LEntry
	PHC2SYSThresholdViolations []daemonlogs.PHC2SYSEntry

	ParseWarnings []string
}

// Analyze evaluates parsed daemon logs against stability policy.
func Analyze(parsed daemonlogs.ParsedLogs, thresholdAbs int64) AnalysisResult {
	if thresholdAbs <= 0 {
		thresholdAbs = DefaultOffsetThresholdAbs
	}

	result := AnalysisResult{
		PTP4LStats:      calculateStatisticsFromPTP4L(parsed.PTP4L.Entries),
		PHC2SYSStats:    calculateStatisticsFromPHC2SYS(parsed.PHC2SYS.Entries),
		PTP4LStartCount: parsed.PTP4LStartCount,
		ParseWarnings:   buildParseWarnings(parsed),
	}

	result.FaultyLines = collectLinesContaining(parsed.Lines, "faulty")
	result.TimeoutLines = collectLinesContaining(parsed.Lines, "timeout")
	result.PTP4LThresholdViolations = findPTP4LThresholdViolations(parsed.PTP4L.Entries, thresholdAbs)
	result.PHC2SYSThresholdViolations = findPHC2SYSThresholdViolations(parsed.PHC2SYS.Entries, thresholdAbs)
	result.StateTransitions = findStateTransitions(parsed.PTP4L.Entries)

	result.Details = buildFailureDetails(result)
	result.Errors = len(result.Details)
	result.Passed = result.Errors == 0

	return result
}

// DiagnosticMessage builds a concise multi-line summary for assertions and report entries.
func (result AnalysisResult) DiagnosticMessage() string {
	var summaryLines []string

	if len(result.Details) == 0 {
		summaryLines = append(summaryLines, "No stability anomalies detected.")
	} else {
		summaryLines = append(summaryLines, "Stability anomalies detected:")
		for _, detail := range result.Details {
			summaryLines = append(summaryLines, "- "+detail)
		}
	}

	if len(result.ParseWarnings) > 0 {
		summaryLines = append(summaryLines, "Parse warnings:")
		for _, warning := range result.ParseWarnings {
			summaryLines = append(summaryLines, "- "+warning)
		}
	}

	summaryLines = append(summaryLines,
		formatStatsLine("ptp4l", result.PTP4LStats),
		formatStatsLine("phc2sys", result.PHC2SYSStats),
		fmt.Sprintf("ptp4l_start_count=%d", result.PTP4LStartCount),
	)

	return strings.Join(summaryLines, "\n")
}

// buildParseWarnings returns human-readable warnings for any lines that were dropped during parsing.
func buildParseWarnings(parsed daemonlogs.ParsedLogs) []string {
	var warnings []string

	if parsed.PTP4L.DroppedLines > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"ptp4l dropped %d/%d candidate delay lines during parsing",
			parsed.PTP4L.DroppedLines, parsed.PTP4L.CandidateLines))
	}

	if parsed.PHC2SYS.DroppedLines > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"phc2sys dropped %d/%d candidate delay lines during parsing",
			parsed.PHC2SYS.DroppedLines, parsed.PHC2SYS.CandidateLines))
	}

	return warnings
}

// buildFailureDetails collects one detail string per stability check that failed.
func buildFailureDetails(result AnalysisResult) []string {
	var details []string

	if len(result.FaultyLines) > 0 {
		details = append(details, fmt.Sprintf("found %d lines containing FAULTY", len(result.FaultyLines)))
	}

	if len(result.TimeoutLines) > 0 {
		details = append(details, fmt.Sprintf("found %d lines containing timeout", len(result.TimeoutLines)))
	}

	if len(result.PTP4LThresholdViolations) > 0 {
		details = append(details,
			fmt.Sprintf("found %d ptp4l s2 offset violations over threshold", len(result.PTP4LThresholdViolations)))
	}

	if len(result.PHC2SYSThresholdViolations) > 0 {
		details = append(details, fmt.Sprintf(
			"found %d phc2sys s2 offset violations over threshold", len(result.PHC2SYSThresholdViolations)))
	}

	if len(result.StateTransitions) > 0 {
		details = append(details, fmt.Sprintf("found %d ptp4l state transitions", len(result.StateTransitions)))
	}

	return details
}

// collectLinesContaining returns all lines that contain needle (case-insensitive).
func collectLinesContaining(lines []string, needle string) []string {
	var matches []string

	lowerNeedle := strings.ToLower(needle)

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerNeedle) {
			matches = append(matches, line)
		}
	}

	return matches
}

// findPTP4LThresholdViolations returns ptp4l s2 entries whose absolute offset exceeds thresholdAbs.
func findPTP4LThresholdViolations(entries []daemonlogs.PTP4LEntry, thresholdAbs int64) []daemonlogs.PTP4LEntry {
	var violations []daemonlogs.PTP4LEntry

	for _, entry := range entries {
		if entry.State != "s2" {
			continue
		}

		if abs(entry.Offset) > thresholdAbs {
			violations = append(violations, entry)
		}
	}

	return violations
}

// findPHC2SYSThresholdViolations returns phc2sys s2 entries whose absolute offset exceeds thresholdAbs.
func findPHC2SYSThresholdViolations(entries []daemonlogs.PHC2SYSEntry, thresholdAbs int64) []daemonlogs.PHC2SYSEntry {
	var violations []daemonlogs.PHC2SYSEntry

	for _, entry := range entries {
		if entry.State != "s2" {
			continue
		}

		if abs(entry.Offset) > thresholdAbs {
			violations = append(violations, entry)
		}
	}

	return violations
}

// findStateTransitions returns transitions where the ptp4l state changed between adjacent entries.
func findStateTransitions(entries []daemonlogs.PTP4LEntry) []StateTransition {
	var transitions []StateTransition

	var previousState string

	for _, entry := range entries {
		if previousState != "" && previousState != entry.State {
			transitions = append(transitions, StateTransition{
				From: previousState,
				To:   entry.State,
				Raw:  entry.Raw,
			})
		}

		previousState = entry.State
	}

	return transitions
}

// calculateStatisticsFromPTP4L extracts offsets from ptp4l entries and computes aggregate statistics.
func calculateStatisticsFromPTP4L(entries []daemonlogs.PTP4LEntry) OffsetStatistics {
	values := make([]int64, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Offset)
	}

	return calculateStatistics(values)
}

// calculateStatisticsFromPHC2SYS extracts offsets from phc2sys entries and computes aggregate statistics.
func calculateStatisticsFromPHC2SYS(entries []daemonlogs.PHC2SYSEntry) OffsetStatistics {
	values := make([]int64, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Offset)
	}

	return calculateStatistics(values)
}

// calculateStatistics computes max, min, and average absolute offset over the given values.
func calculateStatistics(values []int64) OffsetStatistics {
	stats := OffsetStatistics{
		SampleCount: len(values),
	}

	if len(values) == 0 {
		return stats
	}

	firstAbs := abs(values[0])
	minAbs := firstAbs
	maxAbs := firstAbs
	totalAbs := float64(firstAbs)

	for _, value := range values[1:] {
		absoluteValue := abs(value)
		totalAbs += float64(absoluteValue)

		if absoluteValue < minAbs {
			minAbs = absoluteValue
		}

		if absoluteValue > maxAbs {
			maxAbs = absoluteValue
		}
	}

	stats.AvgAbs = totalAbs / float64(len(values))
	stats.MaxAbs = maxAbs
	stats.MinAbs = minAbs

	return stats
}

// formatStatsLine renders an OffsetStatistics value as a single key=value diagnostic line.
func formatStatsLine(process string, stats OffsetStatistics) string {
	return fmt.Sprintf("%s_offsets_max_abs=%d min_abs=%d avg_abs=%.3f samples=%d",
		process, stats.MaxAbs, stats.MinAbs, stats.AvgAbs, stats.SampleCount)
}

// abs returns the absolute value of an int64.
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
