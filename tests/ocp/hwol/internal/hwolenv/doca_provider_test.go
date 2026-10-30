//go:build unit_test

package hwolenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocaProviderCursor(t *testing.T) {
	anchor := strings.Repeat("a", 64)
	cursor, err := parseDocaProviderCursor("2052 243269761 540267 " + anchor + " 12005\r\n")
	require.NoError(t, err)
	require.Equal(t, DocaProviderCursor{device: 2052, inode: 243269761, offset: 540267, anchor: anchor, pid: 12005}, cursor)
	for _, input := range []string{
		"", "1 2 3 invalid 4", "1 0 3 " + anchor + " 4", "1 2 3 " + anchor + " 0",
		"1 2 -1 " + anchor + " 4", "1 2 3 " + anchor + " 4 extra",
	} {
		_, err := parseDocaProviderCursor(input)
		require.Error(t, err, input)
	}
}

func TestParseRepresentorEvidence(t *testing.T) {
	pcis := []string{"0000:4b:00.4", "4b:00.5"}
	valid := "OK 0000:4b:00.4 -> ens3f0np0_1 on br-test\r\nOK 0000:4b:00.5 -> ens3f0np0_2 on br-test\n"
	reps, err := parseRepresentorEvidence(valid, "br-test", pcis)
	require.NoError(t, err)
	require.Equal(t, []string{"ens3f0np0_1", "ens3f0np0_2"}, reps)

	for _, tc := range []struct {
		name, output string
		pcis         []string
	}{
		{"missing", strings.Split(valid, "\n")[0], pcis},
		{"wrong bridge", strings.ReplaceAll(valid, "br-test", "other"), pcis},
		{"duplicate PCI", valid, []string{"4b:00.4", "0000:4b:00.4"}},
		{"duplicate representor", strings.ReplaceAll(valid, "ens3f0np0_2", "ens3f0np0_1"), pcis},
		{"unknown PCI", strings.ReplaceAll(valid, "0000:4b:00.5", "0000:4b:00.6"), pcis},
		{"invalid PCI", valid, []string{"../../etc/passwd"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRepresentorEvidence(tc.output, "br-test", tc.pcis)
			require.Error(t, err)
		})
	}
}

func TestVerifyDocaProviderAssignments(t *testing.T) {
	const prefix = "2026-10-29T19:29:52.244Z|00312|netdev_offload|INFO|"
	log := prefix + "ens3f0np0_1: Assigned flow API 'doca_flow_api'.\n" +
		prefix + "ens3f0np0_2: Assigned flow API 'doca_flow_api'.\n"
	reps := []string{"ens3f0np0_1", "ens3f0np0_2"}
	require.NoError(t, verifyDocaProviderAssignments(log, reps))
	require.ErrorContains(t, verifyDocaProviderAssignments(log, []string{"ens3f0np0_1", "ens3f0np0_3"}), "ens3f0np0_3")
	require.ErrorContains(t, verifyDocaProviderAssignments(log+prefix+"ens3f0np0_1: Assigned flow API 'linux_tc'.\n", reps), "linux_tc")
	require.NoError(t, verifyDocaProviderAssignments(prefix+"ens3f0np0_1: Assigned flow API 'linux_tc'.\n"+log, reps))
	require.Error(t, verifyDocaProviderAssignments(log, []string{"ens3f0np0_1", "ens3f0np0_1"}))
	require.Error(t, verifyDocaProviderAssignments(log, nil))
}

func TestDocaProviderLogCursorGuards(t *testing.T) {
	for _, command := range []string{"bash", "python3", "sha256sum", "dd", "tail"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s required for provider log script test", command)
		}
	}
	for _, scenario := range []string{"fresh", "rotated", "rewritten", "restarted", "symlink", "oversized", "device-changed"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			require.NoError(t, os.Mkdir(bin, 0700))
			stat := `#!/bin/sh
exec python3 -c 'import os,sys; s=os.stat(sys.argv[-1]); print(s.st_dev,s.st_ino,s.st_size)' "$@"
`
			systemctl := `#!/bin/sh
test "$1" = --no-pager || exit 1
case "$2" in
  is-active) exit 0 ;;
  show) cat "$HWOL_TEST_PID_FILE" ;;
  *) exit 1 ;;
esac
`
			require.NoError(t, os.WriteFile(filepath.Join(bin, "stat"), []byte(stat), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(bin, "systemctl"), []byte(systemctl), 0700))
			pidFile := filepath.Join(dir, "pid")
			logFile := filepath.Join(dir, "ovs.log")
			require.NoError(t, os.WriteFile(pidFile, []byte("123\n"), 0600))
			require.NoError(t, os.WriteFile(logFile, []byte("old log entry\n"), 0600))
			run := func(script string, args ...string) (string, error) {
				command := exec.Command("bash", append([]string{"-c", script, "bash"}, args...)...)
				command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HWOL_TEST_PID_FILE="+pidFile)
				out, err := command.CombinedOutput()
				return string(out), err
			}
			out, err := run(captureDocaProviderCursorScript, logFile)
			require.NoError(t, err, out)
			cursor, err := parseDocaProviderCursor(out)
			require.NoError(t, err)
			file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
			require.NoError(t, err)
			_, err = file.WriteString("fresh assignment\n")
			require.NoError(t, err)
			require.NoError(t, file.Close())

			switch scenario {
			case "rotated":
				require.NoError(t, os.Rename(logFile, logFile+".old"))
				require.NoError(t, os.WriteFile(logFile, []byte("old log entry\nfresh assignment\n"), 0600))
			case "rewritten":
				require.NoError(t, os.WriteFile(logFile, []byte("new log entry\nfresh assignment\n"), 0600))
			case "restarted":
				require.NoError(t, os.WriteFile(pidFile, []byte("456\n"), 0600))
			case "symlink":
				require.NoError(t, os.Rename(logFile, logFile+".old"))
				require.NoError(t, os.Symlink(logFile+".old", logFile))
			case "oversized":
				require.NoError(t, os.Truncate(logFile, int64(cursor.offset)+16777217))
			case "device-changed":
				cursor.device++
			}
			out, err = run(readDocaProviderLogScript, logFile, strconv.FormatUint(cursor.device, 10),
				strconv.FormatUint(cursor.inode, 10), strconv.FormatUint(cursor.offset, 10), cursor.anchor,
				strconv.FormatUint(cursor.pid, 10))
			if scenario == "fresh" {
				require.NoError(t, err, out)
				require.Equal(t, "fresh assignment\n", out)
			} else {
				require.Error(t, err, "unsafe %s must fail closed", scenario)
			}
		})
	}
}
