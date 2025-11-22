package iface

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
)

// GetNICDriver uses ethtool to retrieve the driver for a given network interface on a specified node.
func GetNICDriver(client *clients.Settings, nodeName string, ifName Name) (string, error) {
	command := fmt.Sprintf("ethtool -i %s | grep --color=no driver | awk '{print $2}'", ifName)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return "", fmt.Errorf("failed to get NIC driver for interface %s on node %s: %w", ifName, nodeName, err)
	}

	return strings.TrimSpace(output), nil
}

// GetEgressInterfaceName retrieves the name of the interface that is connected to the egress network. Tests should
// avoid bringing down this interface so they maintain cluster connectivity.
func GetEgressInterfaceName(client *clients.Settings, nodeName string) (Name, error) {
	command := "MAC=$(cat /sys/class/net/br-ex/address); ip addr | grep -B 1 ${MAC} | " +
		"grep \" UP \" | grep -v br-ex | awk '{print $2}' | tr -d [:]"

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return "", fmt.Errorf("failed to get OCP interface name for node %s: %w", nodeName, err)
	}

	return Name(strings.TrimSpace(output)), nil
}

// GetPTPHardwareClock uses ethtool to retrieve the PTP hardware clock for a given network interface on a specified
// node.
func GetPTPHardwareClock(client *clients.Settings, nodeName string, ifName Name) (int, error) {
	command := fmt.Sprintf("ethtool -T %s | grep 'PTP Hardware Clock' | cut -d' ' -f4", ifName)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return -1, fmt.Errorf("failed to get PTP hardware clock for interface %s on node %s: %w", ifName, nodeName, err)
	}

	hardwareClock, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return -1, fmt.Errorf("failed to convert PTP hardware clock for interface %s on node %s to int: %w",
			ifName, nodeName, err)
	}

	return hardwareClock, nil
}

// AdjustPTPHardwareClock adjusts the PTP hardware clock for a given network interface on a specified node. This affects
// the CLOCK_REALTIME offset. The amount is in seconds.
func AdjustPTPHardwareClock(client *clients.Settings, nodeName string, ifName Name, amount float64) error {
	hardwareClock, err := GetPTPHardwareClock(client, nodeName, ifName)
	if err != nil {
		return fmt.Errorf("failed to get PTP hardware clock for interface %s on node %s: %w", ifName, nodeName, err)
	}

	command := fmt.Sprintf("phc_ctl /dev/ptp%d adjust %f", hardwareClock, amount)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return fmt.Errorf("failed to adjust PTP hardware clock for interface %s on node %s: %w", ifName, nodeName, err)
	}

	return nil
}

// ResetPTPHardwareClock resets the PTP hardware clock for a given network interface on a specified node.
func ResetPTPHardwareClock(client *clients.Settings, nodeName string, ifName Name) error {
	hardwareClock, err := GetPTPHardwareClock(client, nodeName, ifName)
	if err != nil {
		return fmt.Errorf("failed to get PTP hardware clock for interface %s on node %s: %w", ifName, nodeName, err)
	}

	command := fmt.Sprintf("phc_ctl /dev/ptp%d set", hardwareClock)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return fmt.Errorf("failed to reset PTP hardware clock for interface %s on node %s: %w", ifName, nodeName, err)
	}

	return nil
}

// SetInterfaceStatus sets a given interface to a given state. It will wait up to 15 seconds for the interface to be in
// the expected state after setting it.
func SetInterfaceStatus(client *clients.Settings, nodeName string, iface Name, state InterfaceState) error {
	command := fmt.Sprintf("ip link set %s %s", iface, state)

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true))
	if err != nil {
		return err
	}

	// Grep will return 1 if the interface is not found, which will return an error. We retry up to 5 times with a 3
	// second delay to wait for the interface to be up.
	command = fmt.Sprintf("ip link show %s | grep \" state %s \"", iface, strings.ToUpper(string(state)))

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(5), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryDelay(3*time.Second))
	if err != nil {
		return err
	}

	return nil
}
