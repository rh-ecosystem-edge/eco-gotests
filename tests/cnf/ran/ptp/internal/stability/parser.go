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
)

// ParseResult holds the outcome of attempting to parse a single log line.
type ParseResult struct {
	// Entry is the parsed log entry. It is zero-valued when Matched is false or Dropped is true.
	Entry LogEntry
	// Matched is true when the line matched the given regex pattern.
	Matched bool
	// Dropped is true when the line matched but the offset could not be parsed as an integer.
	Dropped bool
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
