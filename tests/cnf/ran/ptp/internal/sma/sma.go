// Package sma provides helpers for manipulating SMA pin connections on multi-NIC
// Grandmaster nodes. SMA (SubMiniature version A) cables synchronize multiple Intel
// E810 NICs so they can all act as Grandmasters.
package sma

import (
	"errors"
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

const (
	// hostDpllBin invokes the host's dpll CLI from inside the PTP daemon pod by entering the mount and network
	// namespaces of PID 1. The binary ships with iproute2 >= 6.17 and is absent from older RHCOS images.
	hostDpllBin = "nsenter -t 1 -m -n -- /usr/sbin/dpll"
	// podDpllBin invokes a dpll CLI shipped in the PTP daemon container image itself. No nsenter is needed since
	// the daemon pod uses the host network, so DPLL netlink is reachable directly.
	podDpllBin = "dpll"
)

// ErrSmaUnsupported is returned when an SMA pin cannot be inspected or manipulated on a node because neither the
// legacy sysfs pin file nor a dpll CLI is available. Callers that can skip instead of failing should use
// [GetCapability] to detect this before attempting any SMA operation.
var ErrSmaUnsupported = errors.New(
	"no sysfs SMA pin file and no dpll CLI (iproute2 >= 6.17) on the host or in the PTP daemon pod")

// Capability describes the mechanism available on a node for inspecting and manipulating an SMA pin. SMA pin control
// moved from sysfs to the DPLL netlink subsystem in newer ice drivers, and the userspace dpll CLI that exposes it is
// only present on sufficiently new hosts, so neither mechanism can be assumed.
type Capability int

const (
	// CapabilityNone means the SMA pin cannot be controlled at all on this node.
	CapabilityNone Capability = iota
	// CapabilitySysfs means the legacy sysfs pin file is present and is used to control the pin.
	CapabilitySysfs
	// CapabilityHostDpll means the host provides /usr/sbin/dpll, reached from the daemon pod via nsenter.
	CapabilityHostDpll
	// CapabilityPodDpll means the PTP daemon container image ships a dpll CLI on its PATH.
	CapabilityPodDpll
)

// String returns a human readable name for the capability.
func (capability Capability) String() string {
	switch capability {
	case CapabilitySysfs:
		return "sysfs"
	case CapabilityHostDpll:
		return "host dpll CLI"
	case CapabilityPodDpll:
		return "pod dpll CLI"
	case CapabilityNone:
		return "none"
	default:
		return "unknown"
	}
}

// dpllBin returns the dpll CLI invocation for this capability, or an empty string for capabilities that are not based
// on the dpll CLI.
func (capability Capability) dpllBin() string {
	switch capability {
	case CapabilityHostDpll:
		return hostDpllBin
	case CapabilityPodDpll:
		return podDpllBin
	case CapabilitySysfs, CapabilityNone:
		return ""
	default:
		return ""
	}
}

// requireDpllBin returns the dpll CLI invocation for this capability, or [ErrSmaUnsupported] when the pin cannot be
// controlled through the dpll CLI.
func (capability Capability) requireDpllBin(ifaceName iface.Iface, pinName string) (string, error) {
	dpllBin := capability.dpllBin()
	if dpllBin == "" {
		return "", fmt.Errorf("cannot control %s pin for interface %s: %w", pinName, ifaceName, ErrSmaUnsupported)
	}

	return dpllBin, nil
}

// GetCapability determines how the given SMA pin can be controlled on the node, preferring the legacy sysfs pin file
// and falling back to the host dpll CLI then a dpll CLI inside the daemon pod. It returns (CapabilityNone, nil) when
// the pin cannot be found on the node, and (CapabilityNone, error) when infrastructure prevents discovery (e.g., the
// daemon pod is unavailable or kubectl exec is denied). Callers should skip on the former and fail on the latter.
func GetCapability(client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (Capability,
	error) {
	exists, err := hasSysfsSmaPin(client, nodeName, ifaceName, pinName)
	if err != nil {
		return CapabilityNone, fmt.Errorf("failed to probe sysfs SMA pin on node %s: %w", nodeName, err)
	}

	if exists {
		return CapabilitySysfs, nil
	}

	klog.V(tsparams.LogLevel).Infof(
		"sysfs %s pin not found for %s on node %s, looking for a dpll CLI", pinName, ifaceName, nodeName)

	var lastError error

	for _, capability := range []Capability{CapabilityHostDpll, CapabilityPodDpll} {
		exists, err := hasDpllCLI(client, nodeName, capability.dpllBin())
		if err != nil {
			lastError = err

			continue
		}

		if exists {
			return capability, nil
		}
	}

	if lastError != nil {
		return CapabilityNone, fmt.Errorf(
			"failed to probe DPLL CLI on node %s after sysfs probe failed: %w", nodeName, lastError)
	}

	klog.V(tsparams.LogLevel).Infof("no dpll CLI available on node %s for %s pin %s", nodeName, ifaceName, pinName)

	return CapabilityNone, nil
}

// hasDpllCLI reports whether the given dpll CLI invocation is usable from the PTP daemon pod. It runs the read-only
// "device show" subcommand and returns (true, nil) on success, (false, nil) when the binary is not found, and
// (false, error) when infrastructure prevents the probe (e.g., the daemon pod is unreachable or kubectl exec is
// denied).
// The probe is retried with a short delay to account for transient daemon or netlink unavailability.
func hasDpllCLI(client *clients.Settings, nodeName, dpllBin string) (bool, error) {
	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, dpllBin+" device show",
		ptpdaemon.WithRetries(2),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryDelay(10*time.Second))
	if err != nil {
		// Exit code 127 means command not found, which is expected when the dpll binary is not available.
		// Treat this as binary absence (false, nil), not as an infrastructure error.
		if strings.Contains(strings.ToLower(err.Error()), "exit code 127") {
			return false, nil
		}

		klog.V(tsparams.LogLevel).Infof("dpll CLI %q probe on node %s: %v", dpllBin, nodeName, err)

		return false, err
	}

	return true, nil
}

// DisconnectSma disables the given SMA pin on the node, using whichever mechanism [GetCapability] reports. It returns
// an error wrapping [ErrSmaUnsupported] when the pin cannot be controlled, or any infrastructure error from discovery.
func DisconnectSma(client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) error {
	capability, err := GetCapability(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	return DisconnectSmaWithCapability(client, nodeName, ifaceName, pinName, capability)
}

// ReconnectSma restores the given SMA pin to smaConfig (e.g. "1 1"), using whichever mechanism [GetCapability]
// reports. On the DPLL path, smaConfig is not used because it is a sysfs-format string that has no meaning in the DPLL
// subsystem; reconnectSmaViaDpll uses prio=3 as the conventional default instead. It returns an error wrapping
// [ErrSmaUnsupported] when the pin cannot be controlled, or any infrastructure error from discovery.
func ReconnectSma(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, smaConfig string) error {
	capability, err := GetCapability(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	return ReconnectSmaWithCapability(client, nodeName, ifaceName, pinName, smaConfig, capability)
}

// IsSmaConnected returns true if the given SMA pin is actively connected, using whichever mechanism [GetCapability]
// reports. It returns an error wrapping [ErrSmaUnsupported] when the pin cannot be inspected, or any infrastructure
// error from discovery.
func IsSmaConnected(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (bool, error) {
	capability, err := GetCapability(client, nodeName, ifaceName, pinName)
	if err != nil {
		return false, err
	}

	return IsSmaConnectedWithCapability(client, nodeName, ifaceName, pinName, capability)
}

// DisconnectSmaWithCapability disables the SMA pin using a pre-probed capability, avoiding redundant [GetCapability]
// calls when the capability is already known.
func DisconnectSmaWithCapability(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string, capability Capability) error {
	if capability == CapabilitySysfs {
		return disconnectSmaViaSysfs(client, nodeName, ifaceName, pinName)
	}

	dpllBin, err := capability.requireDpllBin(ifaceName, pinName)
	if err != nil {
		return err
	}

	return disconnectSmaViaDpll(client, nodeName, ifaceName, pinName, dpllBin)
}

// ReconnectSmaWithCapability restores the SMA pin using a pre-probed capability, avoiding redundant [GetCapability]
// calls when the capability is already known.
func ReconnectSmaWithCapability(
	client *clients.Settings,
	nodeName string, ifaceName iface.Iface, pinName, smaConfig string, capability Capability) error {
	if capability == CapabilitySysfs {
		return reconnectSmaViaSysfs(client, nodeName, ifaceName, pinName, smaConfig)
	}

	dpllBin, err := capability.requireDpllBin(ifaceName, pinName)
	if err != nil {
		return err
	}

	return reconnectSmaViaDpll(client, nodeName, ifaceName, pinName, dpllBin)
}

// IsSmaConnectedWithCapability returns true if the SMA pin is actively connected using a pre-probed capability,
// avoiding redundant [GetCapability] calls when the capability is already known.
func IsSmaConnectedWithCapability(
	client *clients.Settings,
	nodeName string, ifaceName iface.Iface, pinName string, capability Capability) (bool, error) {
	if capability == CapabilitySysfs {
		return isSmaConnectedViaSysfs(client, nodeName, ifaceName, pinName)
	}

	dpllBin, err := capability.requireDpllBin(ifaceName, pinName)
	if err != nil {
		return false, err
	}

	return isSmaConnectedViaDpll(client, nodeName, ifaceName, pinName, dpllBin)
}

// hasSysfsSmaPin checks whether the legacy sysfs pin file exists for the given interface and pin
// name. Returns (true, nil) when the pin file exists, (false, nil) when the ice driver has migrated pin
// management to the DPLL netlink subsystem, and (false, error) when infrastructure prevents the check
// (e.g., the daemon pod is unreachable or kubectl exec is denied). The file not found condition (ls exit code 2)
// is treated as absence, not as an error.
func hasSysfsSmaPin(client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (bool, error) {
	cmd := fmt.Sprintf("ls /sys/class/net/%s/device/ptp/*/pins/%s 2>/dev/null", ifaceName, pinName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd)
	if err != nil {
		// ls exit code 2 means file not found, which is expected on systems with DPLL-based pin management.
		// Treat this as file absence (false, nil), not as an infrastructure error.
		if strings.Contains(strings.ToLower(err.Error()), "exit code 2") {
			return false, nil
		}

		return false, err
	}

	return strings.TrimSpace(out) != "", nil
}

// readSysfsSmaPin reads the current sysfs SMA pin file, which holds two space-separated
// integers: func (0=disabled, 1=RX, 2=TX) and chan (the connector channel number, e.g. 1
// for SMA1/U.FL1 or 2 for SMA2/U.FL2).
func readSysfsSmaPin(
	client *clients.Settings,
	nodeName string,
	ifaceName iface.Iface,
	pinName string) (funcVal, channel string, err error) {
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
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, value string) error {
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
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) error {
	_, channel, err := readSysfsSmaPin(client, nodeName, ifaceName, pinName)
	if err != nil {
		return err
	}

	return writeSysfsSmaPin(client, nodeName, ifaceName, pinName, fmt.Sprintf("0 %s", channel))
}

// reconnectSmaViaSysfs restores the SMA pin to smaConfig by writing it directly to the sysfs pin file.
func reconnectSmaViaSysfs(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, smaConfig string) error {
	return writeSysfsSmaPin(client, nodeName, ifaceName, pinName, smaConfig)
}

// isSmaConnectedViaSysfs reads the SMA pin value and returns true if the func field is not "0".
func isSmaConnectedViaSysfs(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (bool, error) {
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
// the PCI serial number via devlink, converts it to a DPLL clock-id, then queries the dpll CLI
// given by dpllBin to look up the pin.
func getDpllSmaPinID(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, dpllBin string) (string, error) {
	cmd := fmt.Sprintf(
		`PCI=$(readlink /sys/class/net/%s/device | xargs basename) && `+
			`SERIAL=$(devlink dev info pci/$PCI 2>/dev/null | awk '/serial_number/{print $NF}') && `+
			`CLOCK_ID=$(printf '%%d' 0x$(echo $SERIAL | tr -d '-')) && `+
			`%s pin id-get board-label %s clock-id $CLOCK_ID`,
		ifaceName, dpllBin, pinName)

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

// getDpllPinState runs "dpll pin show" through the given dpll CLI and parses the result.
func getDpllPinState(client *clients.Settings, nodeName, pinID, dpllBin string) (dpllPinState, error) {
	cmd := fmt.Sprintf("%s pin show id %s", dpllBin, pinID)

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

// resolveDpllPin resolves the DPLL pin ID for the given interface and pin name and reads its current state. It
// returns an error when the pin has no parent devices, since every subsequent operation acts on the parents.
func resolveDpllPin(
	client *clients.Settings,
	nodeName string,
	ifaceName iface.Iface,
	pinName, dpllBin string) (pinID string, state dpllPinState, err error) {
	pinID, err = getDpllSmaPinID(client, nodeName, ifaceName, pinName, dpllBin)
	if err != nil {
		return "", dpllPinState{}, err
	}

	state, err = getDpllPinState(client, nodeName, pinID, dpllBin)
	if err != nil {
		return "", dpllPinState{}, err
	}

	if len(state.parents) == 0 {
		return "", dpllPinState{}, fmt.Errorf(
			"no parent devices found for DPLL %s pin %s on %s", pinName, pinID, ifaceName)
	}

	return pinID, state, nil
}

// setSmaViaDpll applies parentArgs to every parent device of the SMA pin via the DPLL CLI, choosing inputArgs or
// outputArgs based on the direction reported for the pin's parents.
func setSmaViaDpll(
	client *clients.Settings,
	nodeName string,
	ifaceName iface.Iface,
	pinName, dpllBin, inputArgs, outputArgs string) error {
	pinID, pinState, err := resolveDpllPin(client, nodeName, ifaceName, pinName, dpllBin)
	if err != nil {
		return err
	}

	parentArgs := inputArgs
	if pinState.parents[0].direction == dpllDirectionOutput {
		parentArgs = outputArgs
	}

	dpllArgs := fmt.Sprintf("id %s", pinID)
	for _, parent := range pinState.parents {
		dpllArgs += fmt.Sprintf(" parent-device %s %s", parent.id, parentArgs)
	}

	cmd := fmt.Sprintf("%s pin set %s", dpllBin, dpllArgs)

	klog.V(tsparams.LogLevel).Infof("DPLL set %s: %s", pinName, cmd)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryDelay(10*time.Second))

	return err
}

// disconnectSmaViaDpll disables the SMA pin via the DPLL CLI. For input pins it sets all parents
// to prio=255 state=selectable (lowest priority, effectively disabled). For output pins it sets
// all parents to state=disconnected.
func disconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, dpllBin string) error {
	return setSmaViaDpll(
		client, nodeName, ifaceName, pinName, dpllBin, "prio 255 state selectable", "state disconnected")
}

// reconnectSmaViaDpll re-enables the SMA pin via the DPLL CLI. For input pins it sets all parents
// to prio=3 state=selectable; prio=3 is used as a conventional default matching the value the PTP
// operator assigns to SMA input pins during initial configuration. For output pins it sets all
// parents to state=connected. Note: the sysfs-format smaConfig string is not applicable to the
// DPLL path and is intentionally not used here.
func reconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, dpllBin string) error {
	return setSmaViaDpll(
		client, nodeName, ifaceName, pinName, dpllBin, "prio 3 state selectable", "state connected")
}

// isSmaConnectedViaDpll returns true if the SMA pin is actively connected via the DPLL CLI. For
// input pins, connected means at least one parent has prio < 255. For output pins, connected means
// at least one parent has state=connected.
func isSmaConnectedViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, dpllBin string) (bool, error) {
	_, pinState, err := resolveDpllPin(client, nodeName, ifaceName, pinName, dpllBin)
	if err != nil {
		return false, err
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
