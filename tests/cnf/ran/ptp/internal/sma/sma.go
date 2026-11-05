// Package sma provides helpers for manipulating SMA pin connections on multi-NIC
// Grandmaster nodes. SMA (SubMiniature version A) cables synchronize multiple Intel
// E810 NICs so they can all act as Grandmasters.
package sma

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// DisconnectSma disables the given SMA pin on the node. It first tries the sysfs path; if the
// pin file is absent (ice driver migrated to DPLL netlink), it uses DPLL generic netlink.
func DisconnectSma(client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) error {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return disconnectSmaViaSysfs(client, nodeName, ifaceName, pinName)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL netlink", pinName, ifaceName)

	return disconnectSmaViaDpll(client, nodeName, ifaceName, pinName)
}

// ReconnectSma restores the given SMA pin to smaConfig (e.g. "1 1"). It first tries the sysfs
// path; if the pin file is absent, it uses DPLL generic netlink. On the DPLL path, smaConfig
// is not used because it is a sysfs-format string that has no meaning in the DPLL subsystem;
// reconnectSmaViaDpll uses prio=3 as the conventional default instead.
func ReconnectSma(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName, smaConfig string) error {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return reconnectSmaViaSysfs(client, nodeName, ifaceName, pinName, smaConfig)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL netlink", pinName, ifaceName)

	return reconnectSmaViaDpll(client, nodeName, ifaceName, pinName)
}

// IsSmaConnected returns true if the given SMA pin is actively connected. It first tries the
// sysfs path; if the pin file is absent, it uses DPLL generic netlink.
func IsSmaConnected(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (bool, error) {
	if hasSysfsSmaPin(client, nodeName, ifaceName, pinName) {
		return isSmaConnectedViaSysfs(client, nodeName, ifaceName, pinName)
	}

	klog.V(tsparams.LogLevel).Infof("sysfs %s pin not found for %s, using DPLL netlink", pinName, ifaceName)

	return isSmaConnectedViaDpll(client, nodeName, ifaceName, pinName)
}

// hasSysfsSmaPin checks whether the legacy sysfs pin file exists for the given interface and pin
// name. Returns false when the ice driver has migrated pin management to the DPLL netlink subsystem.
func hasSysfsSmaPin(client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) bool {
	cmd := fmt.Sprintf("ls /sys/class/net/%s/device/ptp/*/pins/%s 2>/dev/null", ifaceName, pinName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd)

	return err == nil && strings.TrimSpace(out) != ""
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

// The DPLL uAPI is stable and defined in linux/dpll.h. Keep the values local so
// this package does not need generated kernel bindings.
const (
	dpllFamilyName    = "dpll"
	dpllFamilyVersion = 1

	dpllCmdPinIDGet = 7
	dpllCmdPinGet   = 8
	dpllCmdPinSet   = 9

	dpllPinAttrID           = 1
	dpllPinAttrParentID     = 2
	dpllPinAttrClockID      = 5
	dpllPinAttrBoardLabel   = 6
	dpllPinAttrDirection    = 10
	dpllPinAttrPriority     = 14
	dpllPinAttrState        = 15
	dpllPinAttrParentDevice = 17

	dpllPinDirectionInput  = 1
	dpllPinDirectionOutput = 2

	dpllPinStateConnected    = 1
	dpllPinStateDisconnected = 2
	dpllPinStateSelectable   = 3

	dpllDisconnectedPriority = 255
	dpllDefaultPriority      = 3
)

type dpllParentDevice struct {
	id        uint32
	direction uint32
	priority  uint32
	state     uint32
}

type dpllPinState struct {
	parents []dpllParentDevice
}

type dpllExecutor interface {
	Execute(genetlink.Message, uint16, netlink.HeaderFlags) ([]genetlink.Message, error)
}

type dpllNetlinkClient struct {
	conn   dpllExecutor
	family genetlink.Family
}

func newDpllNetlinkClient() (*dpllNetlinkClient, func() error, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open DPLL generic netlink connection: %w", err)
	}

	family, err := conn.GetFamily(dpllFamilyName)
	if err != nil {
		_ = conn.Close()

		return nil, nil, fmt.Errorf("failed to find DPLL generic netlink family: %w", err)
	}

	return &dpllNetlinkClient{conn: conn, family: family}, conn.Close, nil
}

func (c *dpllNetlinkClient) execute(command uint8, data []byte) ([]genetlink.Message, error) {
	responses, err := c.conn.Execute(genetlink.Message{
		Header: genetlink.Header{Command: command, Version: c.family.Version},
		Data:   data,
	}, c.family.ID, netlink.Request)
	if err != nil {
		return nil, fmt.Errorf("DPLL netlink command %d failed: %w", command, err)
	}

	return responses, nil
}

// getDpllClockID obtains the clock ID associated with ifaceName. DPLL identifies
// E810 devices by the PCI serial number, which devlink exposes in the daemon pod.
func getDpllClockID(client *clients.Settings, nodeName string, ifaceName iface.Iface) (uint64, error) {
	cmd := fmt.Sprintf(
		`PCI=$(readlink /sys/class/net/%s/device | xargs basename) && `+
			`devlink dev info pci/$PCI 2>/dev/null | awk '/serial_number/{print $NF}'`,
		ifaceName)

	out, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return 0, fmt.Errorf("failed to get DPLL clock ID for %s: %w", ifaceName, err)
	}

	serial := strings.ReplaceAll(strings.TrimSpace(out), "-", "")

	clockID, err := strconv.ParseUint(serial, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse DPLL clock ID %q for %s: %w", out, ifaceName, err)
	}

	return clockID, nil
}

func (c *dpllNetlinkClient) pinID(clockID uint64, pinName string) (uint32, error) {
	attrs, err := marshalDpllAttributes(func(ae *netlink.AttributeEncoder) {
		ae.Uint64(dpllPinAttrClockID, clockID)
		ae.String(dpllPinAttrBoardLabel, pinName)
	})
	if err != nil {
		return 0, err
	}

	responses, err := c.execute(dpllCmdPinIDGet, attrs)
	if err != nil {
		return 0, err
	}

	for _, response := range responses {
		pinIdentifier, found, err := dpllPinIDFromAttributes(response.Data)
		if err != nil {
			return 0, err
		}

		if found {
			return pinIdentifier, nil
		}
	}

	return 0, fmt.Errorf("DPLL pin ID response did not include an ID for %s", pinName)
}

func (c *dpllNetlinkClient) pinState(pinID uint32) (dpllPinState, error) {
	attrs, err := marshalDpllAttributes(func(ae *netlink.AttributeEncoder) {
		ae.Uint32(dpllPinAttrID, pinID)
	})
	if err != nil {
		return dpllPinState{}, err
	}

	responses, err := c.execute(dpllCmdPinGet, attrs)
	if err != nil {
		return dpllPinState{}, err
	}

	if len(responses) == 0 {
		return dpllPinState{}, fmt.Errorf("DPLL pin %d response was empty", pinID)
	}

	state, err := dpllPinStateFromAttributes(responses[0].Data)
	if err != nil {
		return dpllPinState{}, err
	}

	klog.V(tsparams.LogLevel).Infof("DPLL pin %d state: %+v", pinID, state)

	return state, nil
}

func (c *dpllNetlinkClient) setPinParents(pinID uint32, parents []dpllParentDevice, reconnect bool) error {
	attrs, err := marshalDpllAttributes(func(attributeEncoder *netlink.AttributeEncoder) {
		attributeEncoder.Uint32(dpllPinAttrID, pinID)

		for _, parent := range parents {
			parent := parent

			attributeEncoder.Nested(dpllPinAttrParentDevice, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(dpllPinAttrParentID, parent.id)

				if parent.direction == dpllPinDirectionOutput {
					if reconnect {
						nae.Uint32(dpllPinAttrState, dpllPinStateConnected)
					} else {
						nae.Uint32(dpllPinAttrState, dpllPinStateDisconnected)
					}

					return nil
				}

				if reconnect {
					nae.Uint32(dpllPinAttrPriority, dpllDefaultPriority)
				} else {
					nae.Uint32(dpllPinAttrPriority, dpllDisconnectedPriority)
				}

				nae.Uint32(dpllPinAttrState, dpllPinStateSelectable)

				return nil
			})
		}
	})
	if err != nil {
		return err
	}

	_, err = c.execute(dpllCmdPinSet, attrs)

	return err
}

func marshalDpllAttributes(encode func(*netlink.AttributeEncoder)) ([]byte, error) {
	ae := netlink.NewAttributeEncoder()
	encode(ae)

	attrs, err := ae.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode DPLL netlink attributes: %w", err)
	}

	return attrs, nil
}

func dpllPinIDFromAttributes(data []byte) (uint32, bool, error) {
	attrDecoder, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return 0, false, fmt.Errorf("failed to decode DPLL pin ID response: %w", err)
	}

	for attrDecoder.Next() {
		if attrDecoder.Type() == dpllPinAttrID {
			return attrDecoder.Uint32(), true, attrDecoder.Err()
		}
	}

	return 0, false, attrDecoder.Err()
}

func dpllPinStateFromAttributes(data []byte) (dpllPinState, error) {
	attrDecoder, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return dpllPinState{}, fmt.Errorf("failed to decode DPLL pin response: %w", err)
	}

	var state dpllPinState

	for attrDecoder.Next() {
		if attrDecoder.Type() != dpllPinAttrParentDevice {
			continue
		}

		parent, err := dpllParentDeviceFromAttributes(attrDecoder.Bytes())
		if err != nil {
			return dpllPinState{}, err
		}

		state.parents = append(state.parents, parent)
	}

	if err := attrDecoder.Err(); err != nil {
		return dpllPinState{}, fmt.Errorf("failed to decode DPLL pin response: %w", err)
	}

	return state, nil
}

func dpllParentDeviceFromAttributes(data []byte) (dpllParentDevice, error) {
	attrDecoder, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return dpllParentDevice{}, fmt.Errorf("failed to decode DPLL parent device: %w", err)
	}

	var parent dpllParentDevice

	for attrDecoder.Next() {
		switch attrDecoder.Type() {
		case dpllPinAttrParentID:
			parent.id = attrDecoder.Uint32()
		case dpllPinAttrDirection:
			parent.direction = attrDecoder.Uint32()
		case dpllPinAttrPriority:
			parent.priority = attrDecoder.Uint32()
		case dpllPinAttrState:
			parent.state = attrDecoder.Uint32()
		}
	}

	if err := attrDecoder.Err(); err != nil {
		return dpllParentDevice{}, fmt.Errorf("failed to decode DPLL parent device: %w", err)
	}

	return parent, nil
}

func withDpllNetlink(action func(*dpllNetlinkClient) error) error {
	dpll, closeConn, err := newDpllNetlinkClient()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeConn(); closeErr != nil {
			klog.V(tsparams.LogLevel).Infof("failed to close DPLL netlink connection: %v", closeErr)
		}
	}()

	return action(dpll)
}

// disconnectSmaViaDpll disables the SMA pin through DPLL generic netlink. For input pins it sets all
// parents to prio=255 state=selectable (lowest priority, effectively disabled). For output pins it
// sets all parents to state=disconnected.
func disconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) error {
	clockID, err := getDpllClockID(client, nodeName, ifaceName)
	if err != nil {
		return err
	}

	return withDpllNetlink(func(dpll *dpllNetlinkClient) error {
		pinID, err := dpll.pinID(clockID, pinName)
		if err != nil {
			return fmt.Errorf("failed to resolve DPLL %s pin ID for %s: %w", pinName, ifaceName, err)
		}

		pinState, err := dpll.pinState(pinID)
		if err != nil {
			return fmt.Errorf("failed to show DPLL pin %d: %w", pinID, err)
		}

		if len(pinState.parents) == 0 {
			return fmt.Errorf("no parent devices found for DPLL %s pin %d on %s", pinName, pinID, ifaceName)
		}

		return dpll.setPinParents(pinID, pinState.parents, false)
	})
}

// reconnectSmaViaDpll restores the SMA pin through DPLL generic netlink. Input pins use the PTP
// operator's conventional priority 3; output pins are reconnected. The sysfs-format smaConfig is
// intentionally not used by the DPLL API.
func reconnectSmaViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) error {
	clockID, err := getDpllClockID(client, nodeName, ifaceName)
	if err != nil {
		return err
	}

	return withDpllNetlink(func(dpll *dpllNetlinkClient) error {
		pinID, err := dpll.pinID(clockID, pinName)
		if err != nil {
			return fmt.Errorf("failed to resolve DPLL %s pin ID for %s: %w", pinName, ifaceName, err)
		}

		pinState, err := dpll.pinState(pinID)
		if err != nil {
			return fmt.Errorf("failed to show DPLL pin %d: %w", pinID, err)
		}

		if len(pinState.parents) == 0 {
			return fmt.Errorf("no parent devices found for DPLL %s pin %d on %s", pinName, pinID, ifaceName)
		}

		return dpll.setPinParents(pinID, pinState.parents, true)
	})
}

// isSmaConnectedViaDpll reports connectivity from DPLL generic netlink. An input pin is connected
// when a parent priority is below 255; an output pin is connected when a parent state is connected.
func isSmaConnectedViaDpll(
	client *clients.Settings, nodeName string, ifaceName iface.Iface, pinName string) (bool, error) {
	clockID, err := getDpllClockID(client, nodeName, ifaceName)
	if err != nil {
		return false, err
	}

	connected := false
	err = withDpllNetlink(func(dpll *dpllNetlinkClient) error {
		pinID, err := dpll.pinID(clockID, pinName)
		if err != nil {
			return fmt.Errorf("failed to resolve DPLL %s pin ID for %s: %w", pinName, ifaceName, err)
		}

		pinState, err := dpll.pinState(pinID)
		if err != nil {
			return fmt.Errorf("failed to show DPLL pin %d: %w", pinID, err)
		}

		if len(pinState.parents) == 0 {
			return fmt.Errorf("no parent devices found for DPLL %s pin %d on %s", pinName, pinID, ifaceName)
		}

		for _, parent := range pinState.parents {
			if parent.direction == dpllPinDirectionOutput {
				connected = connected || parent.state == dpllPinStateConnected
			} else {
				connected = connected || parent.priority != dpllDisconnectedPriority
			}
		}

		return nil
	})

	return connected, err
}
