package iface

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
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
	// NM-Managed interfaces can cause issues in tests which bring down the interface.
	var managed string

	switch state {
	case InterfaceStateUp:
		managed = "yes"
	case InterfaceStateDown:
		managed = "no"
	}

	// Best effort - ignore errors if nmcli isn't available or interface isn't NM-managed
	nmCommand := fmt.Sprintf("nmcli device set %s managed %s || true", iface, managed)

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, nmCommand,
		ptpdaemon.WithRetries(1), ptpdaemon.WithRetryOnError(true))
	if err != nil {
		return err
	}

	command := fmt.Sprintf("ip link set %s %s", iface, state)

	_, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
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

var phcCtlCmpRegex = regexp.MustCompile(`offset from CLOCK_REALTIME is ([-0-9]+)ns`)

// GetPTPClockSystemTimeOffset compares a PTP clock with the system clock and returns the offset with nanosecond
// precision. If the error is not nil, the offset will be 0, so error should be checked before using the offset.
//
// Note that the returned value is an offset, so it may be negative even though it is a duration.
func GetPTPClockSystemTimeOffset(client *clients.Settings, nodeName string, ptpClockIndex int) (time.Duration, error) {
	command := fmt.Sprintf("phc_ctl /dev/ptp%d cmp", ptpClockIndex)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return 0, fmt.Errorf("failed to compare PTP clock with system clock on node %s: %w", nodeName, err)
	}

	matches := phcCtlCmpRegex.FindStringSubmatch(output)

	if len(matches) < 2 {
		return 0, fmt.Errorf("failed to parse phc_ctl output: %s", output)
	}

	offset, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to convert offset %s to int: %w", matches[1], err)
	}

	return time.Duration(offset) * time.Nanosecond, nil
}

// PollPTPClockSystemTimeOffset repeatedly compares a PTP clock with the system clock every second over a specified
// duration. It begins by getting the initial offset and then ensures during polling the difference between the current
// offset and the initial offset does not exceed the threshold.
//
// Since the goal is to ensure that one clock does not deviate too much from the other, we use the relative offset
// instead of the absolute offset to account for any persistent differences between the clocks. These differences may be
// due to the TAI-UTC offset, for example.
//
// Flakiness is tolerated by retrying errors getting the PTP clock system time offset. However, if there are 120
// consecutive failures, or 2 minutes, the function will return an error.
func PollPTPClockSystemTimeOffset(
	client *clients.Settings, nodeName string, ptpClockIndex int, duration time.Duration, threshold time.Duration) error {
	initialOffset, err := GetPTPClockSystemTimeOffset(client, nodeName, ptpClockIndex)
	if err != nil {
		return fmt.Errorf("failed to get initial PTP clock system time offset on node %s: %w", nodeName, err)
	}

	klog.V(tsparams.LogLevel).Infof("Initial PTP clock system time offset on node %s: %s", nodeName, initialOffset)

	consecutiveFailures := 0

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, duration, true, func(ctx context.Context) (bool, error) {
			offset, err := GetPTPClockSystemTimeOffset(client, nodeName, ptpClockIndex)
			if err != nil {
				// We want to be somewhat lenient with errors that could be due to network issues or
				// other transient failures. Deviations outside the threshold are treated more harshly.
				klog.V(tsparams.LogLevel).Infof("Failed to get PTP clock system time offset on node %s: %v", nodeName, err)

				consecutiveFailures++

				if consecutiveFailures >= 120 {
					klog.V(tsparams.LogLevel).Info("Maximum consecutive failures reached, returning error")

					return false, fmt.Errorf("maximum consecutive failures reached: %w", err)
				}

				return false, nil
			}

			consecutiveFailures = 0

			relativeOffset := offset - initialOffset

			if relativeOffset.Abs() >= threshold {
				return false, fmt.Errorf("system clock deviation %s exceeds %s", relativeOffset, threshold)
			}

			return false, nil
		})

	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		// If the offset never exceeds the threshold, the function will timeout and return a
		// context.DeadlineExceeded error. Error should never be nil but is included for completeness.
		return nil
	}

	return fmt.Errorf("failed to poll PTP clock system time offset on node %s: %w", nodeName, err)
}
