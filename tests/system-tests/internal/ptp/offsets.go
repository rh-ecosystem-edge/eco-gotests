package ptp

import (
	"fmt"
	"regexp"
	"strconv"
)

var offsetLogLine = regexp.MustCompile(`offset\s+(-?\d+)`)

// PTPOffsetsWithinSymmetricNS reports whether every "offset <n>" in logStr is within [-maxAbsNS, maxAbsNS].
func PTPOffsetsWithinSymmetricNS(logStr string, maxAbsNS int) (ok bool, detail string) {
	if maxAbsNS < 0 {
		return false, "invalid maxAbsNS: must be non-negative"
	}

	matches := offsetLogLine.FindAllStringSubmatch(logStr, -1)
	if len(matches) == 0 {

		return false, "no offset values found in PTP daemon logs"
	}

	for _, m := range matches {
		if len(m) < 2 {
			return false, "invalid offset match in PTP daemon logs (missing capture group)"
		}

		offset, err := strconv.Atoi(m[1])
		if err != nil {
			return false, fmt.Sprintf("failed to parse offset value %q from PTP daemon logs: %v", m[1], err)
		}

		if offset < -maxAbsNS || offset > maxAbsNS {

			return false, fmt.Sprintf("offset %d is outside ±%dns threshold", offset, maxAbsNS)
		}
	}

	return true, ""
}
