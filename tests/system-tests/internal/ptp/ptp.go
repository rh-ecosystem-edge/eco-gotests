package ptp

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

const (
	machineConfigNamespace = "openshift-machine-config-operator"
	machineConfigDaemonPod = "machine-config-daemon"
	ptpNamespace           = "openshift-ptp"
	ptpLinuxPod            = "linuxptp-daemon"
	ptpLinuxContainer      = "linuxptp-daemon-container"
)

func isClockSync(apiClient *clients.Settings) (bool, error) {
	podList, err := pod.List(apiClient, machineConfigNamespace)
	if err != nil {
		return false, fmt.Errorf("failed to get Machine config pod list, %w", err)
	}

	SyncMessage := "System clock synchronized: yes"

	for _, pod := range podList {
		if strings.Contains(pod.Object.Name, machineConfigDaemonPod) {
			const maxRetries = 6

			var cmd bytes.Buffer

			var lastErr error

			for iter := 0; iter < maxRetries; iter++ {
				synccmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "timedatectl"}
				cmd, lastErr = pod.ExecCommand(synccmd)

				// Success condition: no error AND non-empty response AND contains sync message
				if lastErr == nil && len(cmd.String()) > 0 && strings.Contains(cmd.String(), SyncMessage) {
					return true, nil
				}

				if iter < maxRetries-1 {
					time.Sleep(10 * time.Second)
				}
			}

			// Check final result after all retries
			if lastErr != nil {
				return false, fmt.Errorf("failed to check clock sync status from machine config container after %d retries, %w, %s",
					maxRetries, lastErr, cmd.String())
			}

			if len(cmd.String()) == 0 {
				return false, fmt.Errorf("failed to check clock sync status from machine config container, "+
					"empty output after %d retries", maxRetries)
			}

			if !strings.Contains(cmd.String(), SyncMessage) {
				return false, fmt.Errorf("clock not in sync after %d retries", maxRetries)
			}

			return true, nil
		}
	}

	return false, fmt.Errorf("sync status could not be verified")
}

func isPtpClockSync(apiClient *clients.Settings) (bool, error) {
	podList, err := pod.List(apiClient, ptpNamespace)
	if err != nil {
		return false, fmt.Errorf("failed to get PTP pod list, %w", err)
	}

	ptpSyncPattern := `openshift_ptp_clock_state{iface="CLOCK_REALTIME",node=".*",process="phc2sys"} 1`
	ptpRe := regexp.MustCompile(ptpSyncPattern)

	for _, pod := range podList {
		if strings.Contains(pod.Object.Name, ptpLinuxPod) {
			const maxRetries = 6

			var cmd bytes.Buffer

			var lastErr error

			for iter := 0; iter < maxRetries; iter++ {
				synccmd := []string{"curl", "-s", "http://localhost:9091/metrics"}
				cmd, lastErr = pod.ExecCommand(synccmd)

				// Success condition: no error AND non-empty response
				if lastErr == nil && len(cmd.String()) > 0 {
					break
				}

				if iter < maxRetries-1 {
					time.Sleep(10 * time.Second)
				}
			}

			// Check final result after all retries
			if lastErr != nil {
				return false, fmt.Errorf("failed to check PTP sync status, %w, %s", lastErr, cmd.String())
			}

			if len(cmd.String()) == 0 {
				return false, fmt.Errorf("failed to check PTP sync status, empty response after %d retries", maxRetries)
			}

			if !ptpRe.MatchString(cmd.String()) {
				return false, fmt.Errorf("PTP not in sync, %s", cmd.String())
			}

			return true, nil
		}
	}

	return false, fmt.Errorf("sync status could not be verified")
}

// ValidatePTPStatus checks the clock sync status and also checks the PTP logs.
func ValidatePTPStatus(apiClient *clients.Settings, timeInterval time.Duration) (bool, error) {
	clockSync, err := isClockSync(apiClient)
	if err != nil {
		return false, err
	}

	ptpSync, err := isPtpClockSync(apiClient)
	if err != nil {
		return false, err
	}

	ptpSync = ptpSync && clockSync

	podList, err := pod.List(apiClient, ptpNamespace)
	if err != nil {
		return ptpSync, err
	}

	if len(podList) == 0 {
		return ptpSync, fmt.Errorf("PTP logs don't exist")
	}

	var ptpLog string

	for _, pod := range podList {
		if strings.Contains(pod.Object.Name, ptpLinuxPod) {
			ptpLog, err = pod.GetLog(timeInterval, ptpLinuxContainer)
			if err != nil {
				return ptpSync, err
			}
		}
	}

	switch {
	case strings.Contains(ptpLog, "timed out while polling for tx timestamp"):
		return ptpSync, fmt.Errorf("error: PTP timed out")
	case strings.Contains(ptpLog, "jump"):
		return ptpSync, fmt.Errorf("error: PTP jump")
	case len(ptpLog) == 0:
		return ptpSync, fmt.Errorf("error: PTP logs not found")
	}

	return ptpSync, nil
}
