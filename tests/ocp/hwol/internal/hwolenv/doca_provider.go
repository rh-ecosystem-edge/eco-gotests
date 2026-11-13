package hwolenv

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

// DocaProviderCursor identifies an OVS log position and process before workload setup.
type DocaProviderCursor struct {
	device uint64
	inode  uint64
	offset uint64
	anchor string
	pid    uint64
}

var (
	// Extracts the (netdev, flow API) pair from the OVS log lines that report which
	// offload provider a netdev was assigned to:
	//
	//	grep -a "Assigned flow API" /var/log/openvswitch/ovs-vswitchd.log
	//
	//	* ...|netdev_offload|INFO|ens3f0np0_1: Assigned flow API 'doca_flow_api'.
	//	* ...|netdev_offload|INFO|ens3f0np0_2: Assigned flow API 'doca_flow_api'.
	//	  ...|netdev_offload|INFO|eno12399: Assigned flow API 'linux_tc'.
	//	  ...|netdev_offload|INFO|f6557de780f6949: Assigned flow API 'linux_tc'.
	//
	// * marks the workload-representor assignments this suite verifies. The unmarked
	// lines match too, but under other netdev keys (host NICs, OVS/OVN ports, the
	// managed bridge, opaque VF IDs) and are ignored. The module field must be
	// exactly "netdev_offload", skipping "netdev_offload(pmd-...)" and
	// "netdev_offload_tc" lines.
	providerAssignmentRE = regexp.MustCompile(`\|netdev_offload\|[^|]+\|([^:|[:space:]]+): Assigned flow API '([^']+)'`)

	// Validates a sha256sum digest, as used for the log-content anchor.
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

	// Rejects crafted or malformed representor names in parsed script output.
	interfaceNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

	// Accepts a VF PCI address in short or full form (4b:00.4, 0000:4b:00.4).
	vfPCIAddressRE = regexp.MustCompile(`(?i)^(?:[0-9a-f]{4}:)?[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
)

const docaProviderLogPath = "/var/log/openvswitch/ovs-vswitchd.log"

// Snapshots the log position and daemon identity that readDocaProviderLogScript later
// verifies: device, inode, byte offset, a sha256 anchor of the preceding 256 bytes,
// and the ovs-vswitchd MainPID — re-checked before returning.
const captureDocaProviderCursorScript = `
set -euo pipefail
log="$1"
# The log must be a plain file, not a symlink.
test -f "$log"
test ! -L "$log"
# Tie the cursor to one running ovs-vswitchd instance.
systemctl --no-pager is-active --quiet ovs-vswitchd.service
pid="$(systemctl --no-pager show ovs-vswitchd -p MainPID --value)"
test "$pid" -gt 0
# File identity (device, inode) and the byte offset new lines start after.
set -- $(stat -c '%d %i %s' "$log")
device="$1"; inode="$2"; offset="$3"
# Content anchor: hash the 256 bytes before the offset.
window=256
if [ "$offset" -lt "$window" ]; then window="$offset"; fi
anchor="$(dd if="$log" bs=1 skip="$((offset-window))" count="$window" 2>/dev/null | sha256sum | cut -d ' ' -f1)"
# Re-check that the file and daemon did not move while snapshotting.
set -- $(stat -c '%d %i %s' "$log")
test "$1" = "$device"
test "$2" = "$inode"
test "$3" -ge "$offset"
test "$(systemctl --no-pager show ovs-vswitchd -p MainPID --value)" = "$pid"
printf '%s %s %s %s %s\n' "$device" "$inode" "$offset" "$anchor" "$pid"
`

// Emits only the bytes after the cursor offset, failing closed if the file, its content
// or the daemon no longer match — checked both before and after the read. A
// journald-style cursor for a plain log file: tail, plus proof that tailing is safe.
const readDocaProviderLogScript = `
set -euo pipefail
log="$1"; device="$2"; inode="$3"; offset="$4"; anchor="$5"; pid="$6"
# check_log: fail unless file, content and daemon still match the cursor.
check_log() {
  test -f "$log"
  test ! -L "$log"
  # Same file: device and inode must match the cursor.
  set -- $(stat -c '%d %i %s' "$log")
  test "$1" = "$device"
  test "$2" = "$inode"
  # At most 16 MiB of new data; anything larger means the offset is stale.
  test "$3" -ge "$offset"
  test "$(( $3 - offset ))" -le 16777216
  # Same content: the 256 bytes before the offset must hash to the anchor.
  window=256
  if [ "$offset" -lt "$window" ]; then window="$offset"; fi
  actual="$(dd if="$log" bs=1 skip="$((offset-window))" count="$window" 2>/dev/null | sha256sum | cut -d ' ' -f1)"
  test "$actual" = "$anchor"
  # Same daemon: active ovs-vswitchd with the recorded MainPID.
  systemctl --no-pager is-active --quiet ovs-vswitchd.service
  test "$(systemctl --no-pager show ovs-vswitchd -p MainPID --value)" = "$pid"
}
check_log
# Only the bytes appended after the cursor offset.
tail -c "+$((offset+1))" "$log"
# The log must not have changed while it was being read.
check_log
`

// CaptureDocaProviderCursor records the current log and process before network creation.
func CaptureDocaProviderCursor(nodeName, image string) (cursor DocaProviderCursor, resultErr error) {
	if nodeName == "" || image == "" {
		return cursor, fmt.Errorf("node and debug image are required")
	}

	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return cursor, err
	}
	defer closeOvsDebugPod(debugPod, &resultErr)

	out, err := debugPod.ExecCommandWithTimeout([]string{
		"chroot", "/host", "bash", "-c", captureDocaProviderCursorScript, "bash", docaProviderLogPath,
	}, 30*time.Second)
	if err != nil {
		return cursor, fmt.Errorf("failed to capture OVS provider log cursor on %s: %w (out=%s)", nodeName, err, out.String())
	}

	return parseDocaProviderCursor(out.String())
}

func parseDocaProviderCursor(output string) (DocaProviderCursor, error) {
	fields := strings.Fields(output)
	if len(fields) != 5 || !sha256RE.MatchString(fields[3]) {
		return DocaProviderCursor{}, fmt.Errorf("invalid OVS provider log cursor %q", output)
	}

	values := make([]uint64, 4)

	for valueIndex, fieldIndex := range []int{0, 1, 2, 4} {
		value, err := strconv.ParseUint(fields[fieldIndex], 10, 63)
		if err != nil {
			return DocaProviderCursor{}, fmt.Errorf("invalid OVS provider log cursor %q", output)
		}

		values[valueIndex] = value
	}

	if values[1] == 0 || values[3] == 0 {
		return DocaProviderCursor{}, fmt.Errorf("invalid OVS provider log inode or PID")
	}

	return DocaProviderCursor{
		device: values[0], inode: values[1], offset: values[2], anchor: fields[3], pid: values[3],
	}, nil
}

func readDocaProviderLog(debugPod *pod.Builder, cursor DocaProviderCursor) (string, error) {
	if cursor.inode == 0 || cursor.pid == 0 || !sha256RE.MatchString(cursor.anchor) {
		return "", fmt.Errorf("invalid OVS provider log cursor")
	}

	out, err := debugPod.ExecCommandWithTimeout([]string{"chroot", "/host", "bash", "-c", readDocaProviderLogScript,
		"bash", docaProviderLogPath, strconv.FormatUint(cursor.device, 10), strconv.FormatUint(cursor.inode, 10),
		strconv.FormatUint(cursor.offset, 10), cursor.anchor, strconv.FormatUint(cursor.pid, 10)}, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("OVS provider log or process changed, or could not be read: %w (out=%s)", err, out.String())
	}

	return out.String(), nil
}

// Extracts the representor names from vfRepresentorOnBridgeScript output, one line
// per VF:
//
//	OK 0000:4b:00.4 -> ens3f0np0_1 on br-0000_4b_00.0
//
// Any deviation from that shape is an error.
func parseRepresentorEvidence(output, bridge string, pciAddrs []string) ([]string, error) {
	wanted := make(map[string]bool, len(pciAddrs))
	for _, pci := range pciAddrs {
		if !vfPCIAddressRE.MatchString(pci) {
			return nil, fmt.Errorf("invalid VF PCI address %q", pci)
		}

		pci = strings.ToLower(pci)
		if strings.Count(pci, ":") == 1 {
			pci = "0000:" + pci
		}

		if wanted[pci] {
			return nil, fmt.Errorf("duplicate VF PCI address %s", pci)
		}

		wanted[pci] = true
	}

	seenPCI := make(map[string]bool, len(wanted))
	seenRep := make(map[string]bool, len(wanted))

	var representors []string

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "OK" {
			continue
		}

		if len(fields) != 6 || fields[2] != "->" || fields[4] != "on" || fields[5] != bridge ||
			!wanted[strings.ToLower(fields[1])] || seenPCI[strings.ToLower(fields[1])] ||
			!interfaceNameRE.MatchString(fields[3]) || seenRep[fields[3]] {
			return nil, fmt.Errorf("invalid VF representor evidence %q", line)
		}

		seenPCI[strings.ToLower(fields[1])] = true
		seenRep[fields[3]] = true
		representors = append(representors, fields[3])
	}

	if len(representors) != len(wanted) || len(wanted) == 0 {
		return nil, fmt.Errorf("missing VF representor evidence")
	}

	return representors, nil
}

// Requires each workload representor's latest flow API assignment in the given log
// section to be doca_flow_api; later lines win per netdev.
func verifyDocaProviderAssignments(log string, representors []string) error {
	if len(representors) == 0 {
		return fmt.Errorf("VF representors are required")
	}

	assignments := make(map[string]string)
	for _, match := range providerAssignmentRE.FindAllStringSubmatch(log, -1) {
		assignments[match[1]] = match[2]
	}

	seen := make(map[string]bool)
	for _, rep := range representors {
		if !interfaceNameRE.MatchString(rep) || seen[rep] {
			return fmt.Errorf("invalid or duplicate VF representor %q", rep)
		}

		seen[rep] = true
		if assignments[rep] != "doca_flow_api" {
			return fmt.Errorf("VF representor %s latest flow API assignment is %q, want doca_flow_api", rep, assignments[rep])
		}
	}

	return nil
}
