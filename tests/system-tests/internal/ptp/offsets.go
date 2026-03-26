package ptp

import (
	"fmt"
	"regexp"
	"strconv"
)

var offsetLogLine = regexp.MustCompile(`offset\s+(-?\d+)`)

// PTPOffsetsWithinSymmetricNS reports whether every "offset <n>" in logStr is within [-maxAbsNS, maxAbsNS].
func PTPOffsetsWithinSymmetricNS(logStr string, maxAbsNS int) (ok bool, detail string) {
	matches := offsetLogLine.FindAllStringSubmatch(logStr, -1)
	if len(matches) == 0 {

		return false, "no offset values found in PTP daemon logs"
	}

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}

		offset, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		if offset < -maxAbsNS || offset > maxAbsNS {

			return false, fmt.Sprintf("offset %d is outside ±%dns threshold", offset, maxAbsNS)
		}
	}

	return true, ""
}
