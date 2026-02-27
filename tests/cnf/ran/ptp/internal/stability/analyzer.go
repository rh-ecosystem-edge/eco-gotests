package stability

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// DefaultOffsetThresholdAbsoluteNanoseconds is the default absolute offset threshold used by stability analysis. It is
// intentionally positive, since it is meant for comparison with absolute offsets.
const DefaultOffsetThresholdAbsoluteNanoseconds int64 = 100

// OffsetStatistics captures descriptive statistics about absolute offsets over a set of samples. All offsets are in
// nanoseconds.
type OffsetStatistics struct {
	// MaxAbs is the maximum absolute offset in nanoseconds.
	MaxAbs int64
	// MinAbs is the minimum absolute offset in nanoseconds.
	MinAbs int64
	// AvgAbs is the average absolute offset in nanoseconds.
	AvgAbs float64
	// SampleCount is the number of samples used to compute the statistics.
	SampleCount int

	totalAbs float64
}

// observe adds a new offset sample to the statistics.
func (s *OffsetStatistics) observe(offset int64) {
	absoluteValue := abs(offset)

	if s.SampleCount == 0 {
		s.MinAbs = absoluteValue
		s.MaxAbs = absoluteValue
	} else {
		if absoluteValue < s.MinAbs {
			s.MinAbs = absoluteValue
		}

		if absoluteValue > s.MaxAbs {
			s.MaxAbs = absoluteValue
		}
	}

	s.totalAbs += float64(absoluteValue)
	s.SampleCount++
	s.AvgAbs = s.totalAbs / float64(s.SampleCount)
}

// StateTransition describes a servo state change between adjacent parsed entries.
type StateTransition struct {
	// From is the servo state of the previous entry. It is the letter s followed by a number. For example, "s1"
	// means the servo is in state 1.
	From string
	// To is the servo state of the current entry. It is the letter s followed by a number. For example, "s1" means
	// the servo is in state 1.
	To string
	// Raw is the full raw log line that was matched.
	Raw string
}

// ProcessResult groups per-process output fields from stability analysis.
type ProcessResult struct {
	// Stats is the descriptive statistics for the process's offsets.
	Stats OffsetStatistics
	// ThresholdViolationCount is the number of s2 entries whose absolute offset exceeds the threshold.
	ThresholdViolationCount int
	// StateTransitions is the servo state transitions between adjacent entries.
	StateTransitions []StateTransition

	name      string
	pattern   *regexp.Regexp
	threshold int64

	candidateLines int
	droppedLines   int

	prevState string
}

// processEntry tries to parse a log line and updates per-process accumulators if it matches.
func (p *ProcessResult) processEntry(line string) {
	result := tryParseEntry(line, p.pattern)
	if !result.Matched {
		return
	}

	p.candidateLines++

	if result.Dropped {
		klog.V(tsparams.LogLevel).Infof("%s: dropping line with unparseable offset %q", p.name, line)

		p.droppedLines++

		return
	}

	p.Stats.observe(result.Entry.Offset)

	if result.Entry.State == "s2" && abs(result.Entry.Offset) > p.threshold {
		p.ThresholdViolationCount++
	}

	if p.prevState != "" && p.prevState != result.Entry.State {
		p.StateTransitions = append(p.StateTransitions, StateTransition{
			From: p.prevState,
			To:   result.Entry.State,
			Raw:  result.Entry.Raw,
		})
	}

	p.prevState = result.Entry.State
}

// parseWarning returns a human-readable warning if any lines were dropped during parsing, or an empty string otherwise.
func (p *ProcessResult) parseWarning() string {
	if p.droppedLines == 0 {
		return ""
	}

	return fmt.Sprintf("%s dropped %d/%d candidate delay lines during parsing",
		p.name, p.droppedLines, p.candidateLines)
}

// AnalysisResult is the pass/fail decision output of stability analysis.
type AnalysisResult struct {
	// Passed is true if the analysis passed, false otherwise. It counts as passed if there are no failure details.
	Passed bool
	// Details is a list of failure detail messages. It is empty if the analysis passed.
	Details []string

	// PTP4L is the per-process result for ptp4l.
	PTP4L ProcessResult
	// PHC2SYS is the per-process result for phc2sys.
	PHC2SYS ProcessResult

	// PTP4LStartCount is the number of times the ptp4l process was started.
	PTP4LStartCount uint
	// FaultyLineCount is the count of lines containing the word "FAULTY".
	FaultyLineCount int
	// TimeoutLineCount is the count of lines containing the word "timeout".
	TimeoutLineCount int

	// ParseWarnings is a list of warnings that occurred during parsing. These are warnings where the log lines
	// could not be parsed into log entries.
	ParseWarnings []string
}

// AnalyzeFromFile performs a single-pass streaming analysis of the daemon log file at filePath. It reads the file line
// by line to keep memory bounded regardless of file size.
func AnalyzeFromFile(filePath string, thresholdAbsoluteNanoseconds int64) (AnalysisResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}
	defer file.Close()

	if thresholdAbsoluteNanoseconds <= 0 {
		thresholdAbsoluteNanoseconds = DefaultOffsetThresholdAbsoluteNanoseconds
	}

	result := AnalysisResult{
		PTP4L: ProcessResult{
			name:      string(processes.Ptp4l),
			pattern:   ptp4lPattern,
			threshold: thresholdAbsoluteNanoseconds,
		},
		PHC2SYS: ProcessResult{
			name:      string(processes.Phc2sys),
			pattern:   phc2sysPattern,
			threshold: thresholdAbsoluteNanoseconds,
		},
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		result.processLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return AnalysisResult{}, fmt.Errorf("error reading log file %s: %w", filePath, err)
	}

	result.finalize()

	return result, nil
}

// processLine parses and accumulates a single log line.
func (a *AnalysisResult) processLine(line string) {
	if containsFaulty(line) {
		a.FaultyLineCount++
	}

	if containsTimeout(line) {
		a.TimeoutLineCount++
	}

	if isPTP4LStart(line) {
		a.PTP4LStartCount++
	}

	a.PTP4L.processEntry(line)
	a.PHC2SYS.processEntry(line)
}

// finalize processes the accumulated data and determines the pass/fail decision.
func (a *AnalysisResult) finalize() {
	a.ParseWarnings = a.buildParseWarnings()
	a.Details = buildFailureDetails(*a)
	a.Passed = len(a.Details) == 0
}

// buildParseWarnings returns human-readable warnings for any lines that were dropped during parsing.
func (a *AnalysisResult) buildParseWarnings() []string {
	var warnings []string

	if w := a.PTP4L.parseWarning(); w != "" {
		warnings = append(warnings, w)
	}

	if w := a.PHC2SYS.parseWarning(); w != "" {
		warnings = append(warnings, w)
	}

	return warnings
}

// DiagnosticMessage builds a concise multi-line summary for assertions and report entries.
func (a AnalysisResult) DiagnosticMessage() string {
	var builder strings.Builder

	if len(a.Details) == 0 {
		builder.WriteString("No stability anomalies detected.")
	} else {
		builder.WriteString("Stability anomalies detected:")

		for _, detail := range a.Details {
			builder.WriteString("\n- ")
			builder.WriteString(detail)
		}
	}

	if len(a.ParseWarnings) > 0 {
		builder.WriteString("\nParse warnings:")

		for _, warning := range a.ParseWarnings {
			builder.WriteString("\n- ")
			builder.WriteString(warning)
		}
	}

	builder.WriteByte('\n')
	builder.WriteString(formatStatsLine("ptp4l", a.PTP4L.Stats))
	builder.WriteByte('\n')
	builder.WriteString(formatStatsLine("phc2sys", a.PHC2SYS.Stats))
	fmt.Fprintf(&builder, "\nptp4l_start_count=%d", a.PTP4LStartCount)

	return builder.String()
}

// buildFailureDetails collects one detail string per stability check that failed.
func buildFailureDetails(result AnalysisResult) []string {
	var details []string

	if result.PTP4L.Stats.SampleCount == 0 {
		details = append(details, "no ptp4l delay logs parsed")
	}

	if result.PHC2SYS.Stats.SampleCount == 0 {
		details = append(details, "no phc2sys delay logs parsed")
	}

	if result.FaultyLineCount > 0 {
		details = append(details, fmt.Sprintf("found %d lines containing FAULTY", result.FaultyLineCount))
	}

	if result.TimeoutLineCount > 0 {
		details = append(details, fmt.Sprintf("found %d lines containing timeout", result.TimeoutLineCount))
	}

	if result.PTP4L.ThresholdViolationCount > 0 {
		details = append(details,
			fmt.Sprintf("found %d ptp4l s2 offset violations over threshold", result.PTP4L.ThresholdViolationCount))
	}

	if result.PHC2SYS.ThresholdViolationCount > 0 {
		details = append(details, fmt.Sprintf(
			"found %d phc2sys s2 offset violations over threshold", result.PHC2SYS.ThresholdViolationCount))
	}

	if len(result.PTP4L.StateTransitions) > 0 {
		details = append(details, fmt.Sprintf("found %d ptp4l state transitions", len(result.PTP4L.StateTransitions)))
	}

	if result.PTP4LStartCount > 1 {
		details = append(details, fmt.Sprintf("found %d ptp4l restarts", result.PTP4LStartCount-1))
	}

	return details
}

// formatStatsLine renders an OffsetStatistics value as a single key=value diagnostic line.
func formatStatsLine(process string, stats OffsetStatistics) string {
	return fmt.Sprintf("%s_offsets_max_abs=%d min_abs=%d avg_abs=%.3f samples=%d",
		process, stats.MaxAbs, stats.MinAbs, stats.AvgAbs, stats.SampleCount)
}

// abs returns the absolute value of an int64. It ignores the possibility of overflow since it is not applicable to the
// use case of PTP offsets, which should never be the minimum int64 value.
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
