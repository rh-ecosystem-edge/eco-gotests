package daemonlogs

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// CollectionResult is the output of a long-window daemon log collection.
type CollectionResult struct {
	NodeName  string
	StartedAt time.Time
	EndedAt   time.Time
	Lines     []string
	Errors    []error
}

// ParseResult contains parsed entries and the number of dropped candidate lines.
type ParseResult[T any] struct {
	Entries        []T
	DroppedLines   int
	CandidateLines int
}

// PTP4LEntry is a parsed ptp4l delay log line.
type PTP4LEntry struct {
	Raw    string
	Offset int64
	State  string
}

// PHC2SYSEntry is a parsed phc2sys delay log line.
type PHC2SYSEntry struct {
	Raw    string
	Offset int64
	State  string
}

// ParsedLogs contains parsed daemon streams and full raw lines.
type ParsedLogs struct {
	Lines           []string
	PTP4L           ParseResult[PTP4LEntry]
	PHC2SYS         ParseResult[PHC2SYSEntry]
	PTP4LStartCount int
}

var (
	ptp4lPattern   = regexp.MustCompile(`^ptp4l\[.*?\boffset\s+(?P<offset>-?\d+)\s+(?P<state>s\d+).*delay`)
	phc2sysPattern = regexp.MustCompile(`^phc2sys\[.*?\boffset\s+(?P<offset>-?\d+)\s+(?P<state>s\d+).*delay`)

	ptp4lOffsetIndex   = ptp4lPattern.SubexpIndex("offset")
	ptp4lStateIndex    = ptp4lPattern.SubexpIndex("state")
	phc2sysOffsetIndex = phc2sysPattern.SubexpIndex("offset")
	phc2sysStateIndex  = phc2sysPattern.SubexpIndex("state")
)

// ParseLogs parses raw daemon log lines into typed per-process streams.
func ParseLogs(lines []string) ParsedLogs {
	return ParsedLogs{
		Lines:           slices.Clone(lines),
		PTP4L:           ParsePTP4L(lines),
		PHC2SYS:         ParsePHC2SYS(lines),
		PTP4LStartCount: countPTP4LStarts(lines),
	}
}

// ParsePTP4L parses ptp4l delay lines into typed entries. Lines is not modified.
func ParsePTP4L(lines []string) ParseResult[PTP4LEntry] {
	result := ParseResult[PTP4LEntry]{}

	for _, line := range lines {
		match := ptp4lPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		result.CandidateLines++

		offset, err := strconv.ParseInt(match[ptp4lOffsetIndex], 10, 64)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("ptp4l: dropping line with unparseable offset %q: %v", line, err)

			result.DroppedLines++

			continue
		}

		result.Entries = append(result.Entries, PTP4LEntry{
			Raw:    line,
			Offset: offset,
			State:  match[ptp4lStateIndex],
		})
	}

	return result
}

// ParsePHC2SYS parses phc2sys delay lines into typed entries. Lines is not modified.
func ParsePHC2SYS(lines []string) ParseResult[PHC2SYSEntry] {
	result := ParseResult[PHC2SYSEntry]{}

	for _, line := range lines {
		match := phc2sysPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		result.CandidateLines++

		offset, err := strconv.ParseInt(match[phc2sysOffsetIndex], 10, 64)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("phc2sys: dropping line with unparseable offset %q: %v", line, err)

			result.DroppedLines++

			continue
		}

		result.Entries = append(result.Entries, PHC2SYSEntry{
			Raw:    line,
			Offset: offset,
			State:  match[phc2sysStateIndex],
		})
	}

	return result
}

// countPTP4LStarts counts how many times ptp4l was started by looking for "Starting ptp4l" in the log lines.
func countPTP4LStarts(lines []string) int {
	startCount := 0

	for _, line := range lines {
		if strings.Contains(line, "Starting ptp4l") {
			startCount++
		}
	}

	return startCount
}
