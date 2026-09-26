package execoutput

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStripsMCOANSIFromEthtool(t *testing.T) {
	t.Parallel()

	raw := "\x1b[1;31m2026-09-02T22:29:13.122647Z: driver: ice\n" +
		"version: 5.14.0-687.42.1.el9_8.x86_64+rt\n" +
		"firmware-version: 1.22 0x8001873d 1.3909.0\n" +
		"supports-register-dump: yes\nsup\x1b[0m\n" +
		"\x1b[1;31m2026-09-02T22:29:13.122701Z: ports-priv-flags: yes\n\x1b[0m\n"

	normalized := Normalize(raw)
	assert.True(t, stringsHasPrefixLine(normalized, "driver: ice"), "normalized output: %q", normalized)
}

func TestLeadingPIDFromPgrepNoise(t *testing.T) {
	t.Parallel()

	raw := "\x1b[1;31m2026-08-29T22:47:25.064767Z: 119854 ptp4l -f /var/run/ptp4l.1.config\n"

	pid, ok := LeadingPID(raw)
	require.True(t, ok)
	assert.Equal(t, "119854", pid)
}

func TestLinesAsPIDsSkipsNonPIDLines(t *testing.T) {
	t.Parallel()

	raw := "\x1b[1;31m2026-08-29T22:47:25Z: 119854 ptp4l -f /var/run/ptp4l.1.config\n" +
		"ptp4l log noise\n" +
		"120001 ptp4l -f /var/run/ptp4l.0.config\n"

	pids := LinesAsPIDs(raw)
	assert.Equal(t, []string{"119854", "120001"}, pids)
}

func stringsHasPrefixLine(output, prefix string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSpace(line) == prefix {
			return true
		}
	}

	return false
}
