package gnss

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
)

// GetUbloxProtocolVersion returns the protocol version to pass to the ubxtool command. It uses the presence of plugins
// in the profile to determine the protocol version. If one of the e825 or e830 plugins is present, it returns "29.25".
// If the e810 plugin is present, it returns "29.20". Otherwise, it returns an error.
//
// For more information about the 29.25 protocol version, see https://content.u-blox.com/sites/default/files/documents/
// u-blox-F9-TIM-2.25_InterfaceDescription_UBXDOC-963802114-13231.pdf.
//
// For more information about the 29.20 protocol version, see https://content.u-blox.com/sites/default/files/
// u-blox-F9-TIM-2.20_InterfaceDescription_UBX-21048598.pdf.
func GetUbloxProtocolVersion(profile *ptpv1.PtpProfile) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("profile is nil")
	}

	if profile.Name == nil {
		return "", fmt.Errorf("profile name is nil")
	}

	if len(profile.Plugins) < 1 {
		return "", fmt.Errorf("profile %q does not have any plugins", *profile.Name)
	}

	if _, hasE825 := profile.Plugins["e825"]; hasE825 {
		return "29.25", nil
	}

	if _, hasE830 := profile.Plugins["e830"]; hasE830 {
		return "29.25", nil
	}

	if _, hasE810 := profile.Plugins["e810"]; hasE810 {
		return "29.20", nil
	}

	return "", fmt.Errorf("profile %q does not have any e825, e830, or e810 plugins", *profile.Name)
}

// SimulateSyncLoss simulates a loss of GNSS sync by setting the required number of satellites for a fix to be
// artificially high by using the ubxtool command.
func SimulateSyncLoss(client *clients.Settings, nodeName string, protocolVersion string) error {
	if protocolVersion == "" {
		return fmt.Errorf("protocol version is empty")
	}

	// The ubxtool command sends a UBX-CFG-VALSET message to the receiver.
	// -P %s: Sets the UBX protocol version for the command.
	// -w 1:  Waits 1 second for an ACK from the receiver.
	// -v 3:  Sets verbosity to high for debugging.
	// -z CFG-NAVSPG-INFIL_NCNOTHRS,50,1: This is the fault injection.
	//   - ITEM:   CFG-NAVSPG-INFIL_NCNOTHRS is the number of satellites with acceptable noise covariance thresholds
	//             required for a fix to be attempted.
	//   - VAL:    50 is an artificially high number of satellites required for a fix to be attempted.
	//   - LAYERS: 1 specifies the write is to the RAM layer only, causing the change to be in place until reboot.
	command := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,50,1", protocolVersion)

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return fmt.Errorf("failed to simulate GNSS loss on node %s: %w", nodeName, err)
	}

	return nil
}

// SimulateSyncRecovery simulates a recovery of GNSS sync by setting the required number of satellites for a fix to be
// attempted back to the default value, using the ubxtool command.
func SimulateSyncRecovery(client *clients.Settings, nodeName string, protocolVersion string) error {
	if protocolVersion == "" {
		return fmt.Errorf("protocol version is empty")
	}

	// The ubxtool command sends a UBX-CFG-VALSET message to the receiver.
	// -P %s: Sets the UBX protocol version for the command.
	// -w 1:  Waits 1 second for an ACK from the receiver.
	// -v 3:  Sets verbosity to high for debugging.
	// -z CFG-NAVSPG-INFIL_NCNOTHRS,0,1: This undoes the fault injection.
	//   - ITEM:   CFG-NAVSPG-INFIL_NCNOTHRS is the number of satellites with acceptable noise covariance thresholds
	//             required for a fix to be attempted.
	//   - VAL:    0 is the default number of satellites required for a fix to be attempted.
	//   - LAYERS: 1 specifies the write is to the RAM layer only, causing the change to be in place until reboot.
	command := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,0,1", protocolVersion)

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return fmt.Errorf("failed to restore GNSS sync on node %s: %w", nodeName, err)
	}

	return nil
}
