// Package sma provides helpers for manipulating SMA pin connections on multi-NIC
// Grandmaster nodes. SMA (SubMiniature version A) cables synchronize multiple Intel
// E810 NICs so they can all act as Grandmasters.
package sma

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// DisconnectSma disables the given SMA pin on the node. It first tries the sysfs path; if the
// pin file is absent (ice driver migrated to DPLL netlink), it falls back to the host DPLL CLI.
func DisconnectSma(client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) error {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return disconnectSmaViaSysfs(client, nodeName, ifaceName, pinName)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL CLI fallback", pinName, ifaceName)

	return disconnectSmaViaDpll(client, nodeName, ifaceName, pinName)
}

// ReconnectSma restores the given SMA pin to smaConfig (e.g. "1 1"). It first tries the sysfs
// path; if the pin file is absent, it falls back to the host DPLL CLI. On the DPLL path, smaConfig
// is not used because it is a sysfs-format string that has no meaning in the DPLL subsystem;
// reconnectSmaViaDpll uses prio=3 as the conventional default instead.
func ReconnectSma(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName, smaConfig string) error {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return reconnectSmaViaSysfs(client, nodeName, ifaceName, pinName, smaConfig)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL CLI fallback", pinName, ifaceName)

	return reconnectSmaViaDpll(client, nodeName, ifaceName, pinName)
}

// IsSmaConnected returns true if the given SMA pin is actively connected. It first tries the
// sysfs path; if the pin file is absent, it falls back to the host DPLL CLI.
func IsSmaConnected(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) (bool, error) {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return isSmaConnectedViaSysfs(client, nodeName, ifaceName, pinName)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL CLI fallback", pinName, ifaceName)

	return isSmaConnectedViaDpll(client, nodeName, ifaceName, pinName)
}

// hasSysfsSmaPin checks whether the legacy sysfs pin file exists for the given interface and pin
// name. Returns false when the ice driver has migrated pin management to the DPLL netlink subsystem.
func hasSysfsSmaPin(client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) bool {
	cmd := fmt.Sprintf("ls /sys/class/net/%s/device/ptp/*/pins/%s 2>/dev/null", ifaceName, pinName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd)

	return err == nil && strings.TrimSpace(out) != ""
}

// readSysfsSmaPin reads the current sysfs SMA pin file, which holds two space-separated
// integers: func (0=disabled, 1=RX, 2=TX) and chan (the connector channel number, e.g. 1
// for SMA1/U.FL1 or 2 for SMA2/U.FL2).
func readSysfsSmaPin(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) (funcVal, channel string, err error) {
	cmd := fmt.Sprintf("cat /sys/class/net/%s/device/ptp/*/pins/%s", ifaceName, pinName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return "", "", fmt.Errorf("failed to read sysfs pin %s for %s: %w", pinName, ifaceName, err)
	}

	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 2 {
		return "", "", fmt.Errorf("unexpected sysfs pin format for %s/%s: %q", ifaceName, pinName, out)
	}

	return fields[0], fields[1], nil
}

// writeSysfsSmaPin writes a value to the sysfs SMA pin file.
func writeSysfsSmaPin(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName, value string) error {
	cmd := fmt.Sprintf("echo %s > /sys/class/net/%s/device/ptp/*/pins/%s", value, ifaceName, pinName)

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryDelay(10*time.Second))

	return err
}

// disconnectSmaViaSysfs disables the SMA pin by reading the current channel from sysfs and
// writing func=0 while preserving the channel value.
func disconnectSmaViaSysfs(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) error {
	_, channel, err := readSysfsSmaPin(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	return writeSysfsSmaPin(client, nodeName, ifaceName, pinName, fmt.Sprintf("0 %s", channel))
}

// reconnectSmaViaSysfs restores the SMA pin to smaConfig by writing it directly to the sysfs pin file.
func reconnectSmaViaSysfs(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName, smaConfig string) error {
	return writeSysfsSmaPin(client, nodeName, ifaceName, pinName, smaConfig)
}

// isSmaConnectedViaSysfs reads the SMA pin value and returns true if the func field is not "0".
func isSmaConnectedViaSysfs(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) (bool, error) {
	funcVal, _, err := readSysfsSmaPin(client, nodeName, ifaceName, pinName)
	if err != nil {
		return false, err
	}

	return funcVal != "0", nil
}

// dpllPinIDPattern matches "id <number>" or a bare "<number>" anchored to end-of-string, avoiding
// false positives on earlier numeric fields in the dpll CLI output.
var dpllPinIDPattern = regexp.MustCompile(`(?:^|\s)(?:id\s+)?(\d+)\s*$`)

// dpllDirectionOutput is the DPLL parent-device direction value for output pins.
const dpllDirectionOutput = "output"

// getDpllSmaPinID resolves the DPLL pin ID for the given network interface and pin name. It reads
// the PCI serial number via devlink, converts it to a DPLL clock-id, then queries the host dpll
// binary (via nsenter) to look up the pin.
func getDpllSmaPinID(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) (string, error) {
	cmd := fmt.Sprintf(
		`PCI=$(readlink /sys/class/net/%s/device | xargs basename) && `+
			`SERIAL=$(devlink dev info pci/$PCI 2>/dev/null | awk '/serial_number/{print $NF}') && `+
			`CLOCK_ID=$(printf '%%d' 0x$(echo $SERIAL | tr -d '-')) && `+
			`nsenter -t 1 -m -n -- /usr/sbin/dpll pin id-get board-label %s clock-id $CLOCK_ID`,
		ifaceName, pinName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return "", fmt.Errorf("failed to resolve DPLL %s pin ID for %s: %w", pinName, ifaceName, err)
	}

	cleaned := strings.ReplaceAll(strings.TrimSpace(out), "\r", "")

	m := dpllPinIDPattern.FindStringSubmatch(cleaned)
	if len(m) < 2 {
		return "", fmt.Errorf("failed to parse DPLL %s pin ID from output: %q", pinName, out)
	}

	return m[1], nil
}

type dpllParentDevice struct {
	id        string
	direction string
	prio      string
	state     string
}

type dpllPinState struct {
	parents []dpllParentDevice
}

// getDpllPinState runs "dpll pin show" on the host and parses the result.
func getDpllPinState(client *clients.Settings, nodeName, pinID string) (dpllPinState, error) {
	cmd := fmt.Sprintf("nsenter -t 1 -m -n -- /usr/sbin/dpll pin show id %s", pinID)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetryDelay(10*time.Second),
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return dpllPinState{}, fmt.Errorf("failed to show DPLL pin %s: %w", pinID, err)
	}

	pinState := parseDpllPinShow(out)

	klog.V(tsparams.LogLevel).Infof("DPLL pin %s state: %+v", pinID, pinState)

	return pinState, nil
}

// parseDpllPinShow parses the human-readable output of "dpll pin show" into a structured pin state.
// The parent-device section uses an inline format where each parent is a single indented line:
//
//	parent-device:
//	  id 0 direction input prio 0 state connected phase-offset -12345
func parseDpllPinShow(output string) dpllPinState {
	var state dpllPinState

	cleaned := strings.ReplaceAll(output, "\r", "")
	inParentBlock := false

	for _, line := range strings.Split(cleaned, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "parent-device") && strings.HasSuffix(trimmed, ":") {
			inParentBlock = true

			continue
		}

		if inParentBlock {
			if trimmed == "" {
				continue
			}

			if strings.HasPrefix(trimmed, "id ") {
				state.parents = append(state.parents, parseParentDeviceLine(trimmed))

				continue
			}

			inParentBlock = false
		}
	}

	return state
}

// parseParentDeviceLine parses an inline parent-device line such as:
//
//	id 2 direction input prio 0 state connected phase-offset -12345
func parseParentDeviceLine(line string) dpllParentDevice {
	var parent dpllParentDevice

	fields := strings.Fields(line)

	for idx := 0; idx < len(fields)-1; idx++ {
		switch fields[idx] {
		case "id":
			parent.id = fields[idx+1]
		case "direction":
			parent.direction = fields[idx+1]
		case "prio":
			parent.prio = fields[idx+1]
		case "state":
			parent.state = fields[idx+1]
		}
	}

	return parent
}

// disconnectSmaViaDpll disables the SMA pin via the host DPLL CLI. For input pins it sets all
// parents to prio=255 state=selectable (lowest priority, effectively disabled). For output pins it
// sets all parents to state=disconnected.
func disconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) error {
	pinID, err := getDpllSmaPinID(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	pinState, err := getDpllPinState(client, nodeName, pinID)
	if err != nil {
		return err
	}

	if len(pinState.parents) == 0 {
		return fmt.Errorf("no parent devices found for DPLL %s pin %s on %s", pinName, pinID, ifaceName)
	}

	dpllArgs := fmt.Sprintf("id %s", pinID)

	if pinState.parents[0].direction == dpllDirectionOutput {
		for _, p := range pinState.parents {
			dpllArgs += fmt.Sprintf(" parent-device %s state disconnected", p.id)
		}
	} else {
		for _, p := range pinState.parents {
			dpllArgs += fmt.Sprintf(" parent-device %s prio 255 state selectable", p.id)
		}
	}

	cmd := fmt.Sprintf("nsenter -t 1 -m -n -- /usr/sbin/dpll pin set %s", dpllArgs)

	klog.V(tsparams.LogLevel).Infof("DPLL disconnect %s: %s", pinName, cmd)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryDelay(10*time.Second))

	return err
}

// reconnectSmaViaDpll re-enables the SMA pin via the host DPLL CLI. For input pins it sets all
// parents to prio=3 state=selectable; prio=3 is used as a conventional default matching the value
// the PTP operator assigns to SMA input pins during initial configuration. For output pins it sets
// all parents to state=connected. Note: the sysfs-format smaConfig string is not applicable to the
// DPLL path and is intentionally not used here.
func reconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) error {
	pinID, err := getDpllSmaPinID(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	pinState, err := getDpllPinState(client, nodeName, pinID)
	if err != nil {
		return err
	}

	if len(pinState.parents) == 0 {
		return fmt.Errorf("no parent devices found for DPLL %s pin %s on %s", pinName, pinID, ifaceName)
	}

	dpllArgs := fmt.Sprintf("id %s", pinID)

	if pinState.parents[0].direction == dpllDirectionOutput {
		for _, p := range pinState.parents {
			dpllArgs += fmt.Sprintf(" parent-device %s state connected", p.id)
		}
	} else {
		for _, p := range pinState.parents {
			dpllArgs += fmt.Sprintf(" parent-device %s prio 3 state selectable", p.id)
		}
	}

	cmd := fmt.Sprintf("nsenter -t 1 -m -n -- /usr/sbin/dpll pin set %s", dpllArgs)

	klog.V(tsparams.LogLevel).Infof("DPLL reconnect %s: %s", pinName, cmd)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryDelay(10*time.Second))

	return err
}

// isSmaConnectedViaDpll returns true if the SMA pin is actively connected via the DPLL CLI. For
// input pins, connected means at least one parent has prio < 255. For output pins, connected means
// at least one parent has state=connected.
func isSmaConnectedViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Name, pinName string) (bool, error) {
	pinID, err := getDpllSmaPinID(client, nodeName, ifaceName, pinName)
	if err != nil {
		return false, err
	}

	pinState, err := getDpllPinState(client, nodeName, pinID)
	if err != nil {
		return false, err
	}

	if len(pinState.parents) == 0 {
		return false, fmt.Errorf("no parent devices found for DPLL %s pin %s on %s", pinName, pinID, ifaceName)
	}

	if pinState.parents[0].direction == dpllDirectionOutput {
		for _, p := range pinState.parents {
			if p.state == "connected" {
				return true, nil
			}
		}

		return false, nil
	}

	for _, p := range pinState.parents {
		if p.prio != "255" {
			return true, nil
		}
	}

	return false, nil
}
