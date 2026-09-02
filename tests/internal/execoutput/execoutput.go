// Package execoutput normalizes stdout from remote exec (MCO nsenter, linuxptp-daemon pod)
// so tests can parse command output without ANSI color codes or log timestamp prefixes.
package execoutput

import (
	"regexp"
	"strings"
)

var (
	ansiEscapeRegex   = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	isoLogPrefixRegex = regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2}T[\d:.]+Z: `)
	leadingPIDRegex   = regexp.MustCompile(`^\s*(\d+)\b`)
	numericPIDRegex   = regexp.MustCompile(`^\d+$`)
)

// Normalize strips carriage returns, ANSI CSI sequences, and ISO-8601 log prefixes injected
// by machine-config-daemon and linuxptp-daemon logging around exec output.
func Normalize(output string) string {
	output = strings.ReplaceAll(output, "\r", "")
	output = ansiEscapeRegex.ReplaceAllString(output, "")
	output = isoLogPrefixRegex.ReplaceAllString(output, "")

	return output
}

// LeadingPID returns the first decimal PID token on a line after normalization.
func LeadingPID(line string) (string, bool) {
	line = strings.TrimSpace(Normalize(line))
	match := leadingPIDRegex.FindStringSubmatch(line)
	if len(match) < 2 {
		return "", false
	}

	return match[1], true
}

// LinesAsPIDs extracts leading PIDs from each line of command output.
func LinesAsPIDs(output string) []string {
	normalized := Normalize(output)
	var pids []string

	for line := range strings.SplitSeq(normalized, "\n") {
		pid, ok := LeadingPID(line)
		if !ok {
			continue
		}

		pids = append(pids, pid)
	}

	return pids
}

// IsNumericPID reports whether s is a non-empty PID string.
func IsNumericPID(pid string) bool {
	return numericPIDRegex.MatchString(strings.TrimSpace(pid))
}
