package ptpleap

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// announcementPattern is a regular expression that matches the last leap event announcement.
// An example of an announcement is: "\n3692217600     37    # 1 Jan 2017".
var announcementPattern = regexp.MustCompile(`\n(\d+\s+\d+\s+#\s\d+\s[a-zA-Z]+\s\d{4})\n\n`)

// leapLinePattern is a regular expression that matches the last line of the leap event announcement.
// An example of a leap line is: "3692217600     37    #".
var leapLinePattern = regexp.MustCompile(`^\s*\d+\s+\d+\s+#`)

// GetLastAnnouncement returns the last leap event announcement from a leap-configmap Data.
func GetLastAnnouncement(leapConfigMapData string) (string, error) {
	if len(leapConfigMapData) == 0 {
		return leapConfigMapData, nil
	}

	announcementSlice := announcementPattern.FindStringSubmatch(leapConfigMapData)

	if len(announcementSlice) < 2 {
		return "", fmt.Errorf("error finding the last announcement")
	}

	return announcementSlice[1], nil
}

// RemoveLastLeapAnnouncement removes the last "leap announcement" line,
// i.e., the last line that looks like: "<seconds> <offset> # <date>".
func RemoveLastLeapAnnouncement(s string) string {
	lines := strings.Split(s, "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		if leapLinePattern.MatchString(lines[i]) {
			lines = append(lines[:i], lines[i+1:]...)

			break
		}
	}

	return strings.Join(lines, "\n")
}

// ParseAnnouncementDate parses the date portion of a leap event announcement line.
// An example announcement is: "3692217600     37    # 1 Jan 2017".
func ParseAnnouncementDate(announcement string) (time.Time, error) {
	announcementFields := strings.SplitN(announcement, "#", 2)
	if len(announcementFields) != 2 {
		return time.Time{}, fmt.Errorf("unexpected announcement format: %q", announcement)
	}

	return time.Parse("2 Jan 2006", strings.TrimSpace(announcementFields[1]))
}
