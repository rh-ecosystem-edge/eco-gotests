package stability

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryParseEntryDelayLines(t *testing.T) {
	t.Parallel()

	line := "ptp4l[401304.873]: [ptp4l.1.config:6] master offset         -3 s2 freq  -94379 path delay       161"
	result := tryParseEntry(line, ptp4lPattern)
	require.True(t, result.Matched)
	require.False(t, result.Dropped)
	assert.Equal(t, int64(-3), result.Entry.Offset)
	assert.Equal(t, "s2", result.Entry.State)
}

func TestTryParseSummaryEntry(t *testing.T) {
	t.Parallel()

	line := "ptp4l[1788394724.219]: [ptp4l.1.config:6] master offset summary: cnt=161, min=-12, max=10, avg=0.70, SD=4.48"
	summary, ok := tryParseSummaryEntry(line, ptp4lSummaryPattern)
	require.True(t, ok)
	assert.Equal(t, 161, summary.Cnt)
	assert.Equal(t, int64(-12), summary.Min)
	assert.Equal(t, int64(10), summary.Max)
}

func TestNormalizeLinuxptpLine(t *testing.T) {
	t.Parallel()

	prefixed := "2026-09-03T00:18:54.313Z ptp4l[1.2]: [cfg] master offset summary: cnt=1, min=0, max=0"
	assert.Equal(t, "ptp4l[1.2]: [cfg] master offset summary: cnt=1, min=0, max=0", normalizeLinuxptpLine(prefixed))
}

func TestAnalyzeFromFileLogReductionSummaries(t *testing.T) {
	t.Parallel()

	const log = `noise from operator
ptp4l[1.0]: [ptp4l.1.config:6] master offset summary: cnt=10, min=-5, max=4, avg=0.00, SD=1.00
phc2sys[1.1]: [ptp4l.1.config:6] phc offset summary: cnt=10, min=-3, max=2, avg=0.00, SD=1.00
`

	tempFile := t.TempDir() + "/daemon.log"
	require.NoError(t, os.WriteFile(tempFile, []byte(log), 0o600))

	result, err := AnalyzeFromFile(tempFile, 100)
	require.NoError(t, err)
	assert.True(t, result.Passed, result.DiagnosticMessage())
	assert.Positive(t, result.PTP4L.Stats.SampleCount)
	assert.Positive(t, result.PHC2SYS.Stats.SampleCount)
}

func TestAnalyzeFromFileGNRDLogReductionArtifact(t *testing.T) {
	t.Parallel()

	logPath := "../../../../../../logs/kniqe-ci-ocp-far-edge-vran-tests-7430/failed_ptp_suite_test/" +
		"failed_ptp_suite_test/PTP_Stability_validates_PTP_stability_and_offset_behavior_over_configured_duration/" +
		"openshift-ptp_linuxptp-daemon-bt5zp_pods_logs.log"
	if _, err := os.Stat(logPath); err != nil {
		t.Skip("CI stability log artifact not present locally")
	}

	result, err := AnalyzeFromFile(logPath, 100)
	require.NoError(t, err)
	assert.True(t, result.Passed, result.DiagnosticMessage())
}
