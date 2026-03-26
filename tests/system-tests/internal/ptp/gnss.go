package ptp

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// SimulateGNSSLoss runs ubxtool to require an impossible number of satellites (GNSS loss simulation).
func SimulateGNSSLoss(apiClient *clients.Settings, nodeName, protocolVersion string) error {
	cmd := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,50,1", protocolVersion)

	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		daemonPod, err := GetLinuxptpDaemonPodOnNode(apiClient, nodeName)
		if err != nil {
			lastErr = err
			time.Sleep(5 * time.Second)
			continue
		}

		buf, err := daemonPod.ExecCommand([]string{"sh", "-c", cmd}, DaemonContainerName)
		if err != nil {
			lastErr = fmt.Errorf("ubxtool failed: %w, output: %s", err, buf.String())
			time.Sleep(5 * time.Second)
			continue
		}

		return nil
	}

	return lastErr
}

// SimulateGNSSRecovery resets ubxtool satellite threshold after SimulateGNSSLoss.
func SimulateGNSSRecovery(apiClient *clients.Settings, nodeName, protocolVersion string) error {
	cmd := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,0,1", protocolVersion)

	daemonPod, err := GetLinuxptpDaemonPodOnNode(apiClient, nodeName)
	if err != nil {
		return err
	}

	buf, err := daemonPod.ExecCommand([]string{"sh", "-c", cmd}, DaemonContainerName)
	if err != nil {
		return fmt.Errorf("ubxtool recovery failed: %w, output: %s", err, buf.String())
	}

	return nil
}
