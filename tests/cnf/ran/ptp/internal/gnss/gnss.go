package gnss

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	ptpv2alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v2alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
)

const (
	// UbloxProtocolE810 is the ubxtool -P revision for E810 GNSS receivers.
	UbloxProtocolE810 = "29.20"
	// UbloxProtocolE825E830 is the ubxtool -P revision for E825/E830 and GNR-D HardwareConfig GNSS.
	UbloxProtocolE825E830 = "29.25"
)

// GetUbloxProtocolVersion returns the protocol version to pass to the ubxtool command.
// Plugin path (pre-4.22 / non-HardwareConfig): e825 or e830 → 29.25, e810 → 29.20.
// HardwareConfig path (4.22+/GNR-D): prefers an explicit -P from GNSS ExtraCommands, otherwise 29.25.
//
// For more information about the 29.25 protocol version, see https://content.u-blox.com/sites/default/files/documents/
// u-blox-F9-TIM-2.25_InterfaceDescription_UBXDOC-963802114-13231.pdf.
//
// For more information about the 29.20 protocol version, see https://content.u-blox.com/sites/default/files/
// u-blox-F9-TIM-2.20_InterfaceDescription_UBX-21048598.pdf.
func GetUbloxProtocolVersion(profile *ptpv1.PtpProfile, hwConfig *ptp.HardwareConfigBuilder) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("profile is nil")
	}

	if profile.Name == nil {
		return "", fmt.Errorf("profile name is nil")
	}

	if len(profile.Plugins) > 0 {
		return getUbloxProtocolFromPlugins(profile)
	}

	if hwConfig != nil {
		return getUbloxProtocolFromHardwareConfig(hwConfig)
	}

	return "", fmt.Errorf("profile %q has no plugins and no associated HardwareConfig", *profile.Name)
}

func getUbloxProtocolFromPlugins(profile *ptpv1.PtpProfile) (string, error) {
	if _, hasE825 := profile.Plugins[string(ptp.PluginTypeE825)]; hasE825 {
		return UbloxProtocolE825E830, nil
	}

	if _, hasE830 := profile.Plugins[string(ptp.PluginTypeE830)]; hasE830 {
		return UbloxProtocolE825E830, nil
	}

	if _, hasE810 := profile.Plugins[string(ptp.PluginTypeE810)]; hasE810 {
		return UbloxProtocolE810, nil
	}

	return "", fmt.Errorf("profile %q does not have any e825, e830, or e810 plugins", *profile.Name)
}

// getUbloxProtocolFromHardwareConfig resolves ubxtool -P for HardwareConfig-based T-GM profiles.
// ExtraCommands may embed an explicit -P; otherwise GNR-D platforms use the E825/E830 revision.
func getUbloxProtocolFromHardwareConfig(hwConfig *ptp.HardwareConfigBuilder) (string, error) {
	if hwConfig == nil || hwConfig.Definition == nil {
		return "", fmt.Errorf("HardwareConfig is nil")
	}

	chain := hwConfig.Definition.Spec.Profile.ClockChain
	if chain != nil && chain.Behavior != nil {
		for _, source := range chain.Behavior.Sources {
			if source.SourceType != ptpv2alpha1.SourceTypeGNSS || source.GNSSConfig == nil {
				continue
			}

			if version := protocolFromExtraCommands(source.GNSSConfig.Init.ExtraCommands); version != "" {
				return version, nil
			}
		}
	}

	return UbloxProtocolE825E830, nil
}

func protocolFromExtraCommands(commands []ptpv2alpha1.UBLXCommand) string {
	for _, cmd := range commands {
		for i, arg := range cmd.Args {
			if arg == "-P" && i+1 < len(cmd.Args) && cmd.Args[i+1] != "" {
				return cmd.Args[i+1]
			}
		}
	}

	return ""
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

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true))
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

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true))
	if err != nil {
		return fmt.Errorf("failed to restore GNSS sync on node %s: %w", nodeName, err)
	}

	return nil
}
