package stability

import (
	"regexp"
	"strconv"
	"strings"
)

// LogEntry is a parsed delay log line from a synchronization daemon (ptp4l or phc2sys). Each process contains a clock
// offset and a servo state, both of which are extracted from the log line.
type LogEntry struct {
	// Raw is the full raw log line that was matched.
	Raw string
	// Offset is the offset of the log line in nanoseconds.
	Offset int64
	// State is the servo state of the log line. It is the letter s followed by a number. For example, "s1" means
	// the servo is in state 1.
	State string
}

var (
	// ptp4lPattern is a regular expression that matches the ptp4l log lines. For example:
	//  ptp4l[401304.873]: [ptp4l.1.config:6] master offset         -3 s2 freq  -94379 path delay       161
	ptp4lPattern = regexp.MustCompile(`^ptp4l\[.*?\boffset\s+(?P<offset>-?\d+)\s+(?P<state>s\d+).*delay`)
	// phc2sysPattern is a regular expression that matches the phc2sys log lines. For example:
	//  phc2sys[401304.879]: [ptp4l.1.config:6] CLOCK_REALTIME phc offset        -5 s2 freq  -19334 delay    470
	phc2sysPattern = regexp.MustCompile(`^phc2sys\[.*?\boffset\s+(?P<offset>-?\d+)\s+(?P<state>s\d+).*delay`)

	// ptp4lSummaryPattern matches enhanced log-reduction summaries. For example:
	//  ptp4l[1788394724.219]: [ptp4l.1.config:6] master offset summary: cnt=161, min=-12, max=10, avg=0.70, SD=4.48
	ptp4lSummaryPattern = regexp.MustCompile(
		`^ptp4l\[.*?master offset summary: cnt=(?P<cnt>\d+), min=(?P<min>-?\d+), max=(?P<max>-?\d+)`)
	// phc2sysSummaryPattern matches enhanced log-reduction summaries. For example:
	//  phc2sys[1788394730.119]: [ptp4l.1.config:6] phc offset summary: cnt=160, min=-10, max=8, avg=-0.39, SD=4.38
	phc2sysSummaryPattern = regexp.MustCompile(
		`^phc2sys\[.*?phc offset summary: cnt=(?P<cnt>\d+), min=(?P<min>-?\d+), max=(?P<max>-?\d+)`)
)

// SummaryLogEntry is a parsed offset summary line emitted when log reduction is enabled.
type SummaryLogEntry struct {
	Raw string
	Cnt int
	Min int64
	Max int64
}

// lockedServoState is assumed for summary windows during stability testing while clocks are locked.
const lockedServoState = "s2"

// ParseResult holds the outcome of attempting to parse a single log line.
type ParseResult struct {
	// Entry is the parsed log entry. It is zero-valued when Matched is false or Dropped is true.
	Entry LogEntry
	// Matched is true when the line matched the given regex pattern.
	Matched bool
	// Dropped is true when the line matched but the offset could not be parsed as an integer.
	Dropped bool
}

// normalizeLinuxptpLine strips container log prefixes so linuxptp process lines can be matched.
func normalizeLinuxptpLine(line string) string {
	for _, marker := range []string{"ptp4l[", "phc2sys["} {
		if idx := strings.Index(line, marker); idx >= 0 {
			return line[idx:]
		}
	}

	return line
}

// tryParseSummaryEntry attempts to parse a log-reduction offset summary line.
func tryParseSummaryEntry(line string, pattern *regexp.Regexp) (SummaryLogEntry, bool) {
	match := pattern.FindStringSubmatch(line)
	if match == nil {
		return SummaryLogEntry{}, false
	}

	cnt, err := strconv.Atoi(match[pattern.SubexpIndex("cnt")])
	if err != nil || cnt <= 0 {
		return SummaryLogEntry{Raw: line}, true
	}

	minOffset, err := strconv.ParseInt(match[pattern.SubexpIndex("min")], 10, 64)
	if err != nil {
		return SummaryLogEntry{Raw: line}, true
	}

	maxOffset, err := strconv.ParseInt(match[pattern.SubexpIndex("max")], 10, 64)
	if err != nil {
		return SummaryLogEntry{Raw: line}, true
	}

	return SummaryLogEntry{
		Raw: line,
		Cnt: cnt,
		Min: minOffset,
		Max: maxOffset,
	}, true
}

// tryParseEntry attempts to parse a single log line against the given pattern.
func tryParseEntry(line string, pattern *regexp.Regexp) ParseResult {
	match := pattern.FindStringSubmatch(line)
	if match == nil {
		return ParseResult{}
	}

	offsetIndex := pattern.SubexpIndex("offset")
	stateIndex := pattern.SubexpIndex("state")

	offset, err := strconv.ParseInt(match[offsetIndex], 10, 64)
	if err != nil {
		return ParseResult{Matched: true, Dropped: true}
	}

	return ParseResult{
		Entry: LogEntry{
			Raw:    line,
			Offset: offset,
			State:  match[stateIndex],
		},
		Matched: true,
	}
}

// isPTP4LStart returns true if the line indicates a ptp4l process start.
func isPTP4LStart(line string) bool {
	return strings.Contains(line, "Starting ptp4l")
}

// containsFaulty returns true if line contains "faulty" (case-insensitive).
func containsFaulty(line string) bool {
	return strings.Contains(strings.ToLower(line), "faulty")
}

// containsTimeout returns true if line contains "timeout" (case-insensitive).
func containsTimeout(line string) bool {
	return strings.Contains(strings.ToLower(line), "timeout")
}
