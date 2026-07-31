package profiles

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv2alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v2alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
)

// GetHoldoverHardwareConfigSettings reads holdover parameters from the first subsystem
// in the ClockChain that has HoldoverParameters set, and maps them to HoldoverPluginSettings.
func GetHoldoverHardwareConfigSettings(
	hwConfig *ptp.HardwareConfigBuilder) (*HoldoverPluginSettings, error) {
	params, _, err := firstHoldoverParameters(hwConfig)
	if err != nil {
		return nil, err
	}

	return &HoldoverPluginSettings{
		LocalHoldoverTimeout:   uint(params.LocalHoldoverTimeout),
		LocalMaxHoldoverOffSet: uint(params.LocalMaxHoldoverOffset),
		MaxInSpecOffset:        uint(params.MaxInSpecOffset),
	}, nil
}

// SetHoldoverHardwareConfigSettings writes holdover parameters into the HardwareConfig CR
// and updates it on the cluster. Re-fetches the CR before mutating to avoid corrupting
// the shared Definition pointer held by other ProfileInfo clones on the same profile.
func SetHoldoverHardwareConfigSettings(
	hwConfig *ptp.HardwareConfigBuilder, settings HoldoverPluginSettings) error {
	fresh, err := hwConfig.Get()
	if err != nil {
		return fmt.Errorf("failed to re-fetch HardwareConfig %q before update: %w",
			hwConfig.Definition.Name, err)
	}

	hwConfig.Definition = fresh

	_, idx, err := firstHoldoverParameters(hwConfig)
	if err != nil {
		return err
	}

	hwConfig.Definition.Spec.Profile.ClockChain.Structure[idx].DPLL.HoldoverParameters =
		&ptpv2alpha1.HoldoverParameters{
			MaxInSpecOffset:        uint64(settings.MaxInSpecOffset),
			LocalMaxHoldoverOffset: uint64(settings.LocalMaxHoldoverOffSet),
			LocalHoldoverTimeout:   uint64(settings.LocalHoldoverTimeout),
		}

	_, err = hwConfig.Update()

	return err
}

// firstHoldoverParameters returns the HoldoverParameters pointer and its subsystem index
// from the first subsystem in the ClockChain that has them set.
func firstHoldoverParameters(
	hwConfig *ptp.HardwareConfigBuilder) (*ptpv2alpha1.HoldoverParameters, int, error) {
	chain := hwConfig.Definition.Spec.Profile.ClockChain
	if chain == nil {
		return nil, -1, fmt.Errorf("HardwareConfig %q has no ClockChain",
			hwConfig.Definition.Name)
	}

	for i := range chain.Structure {
		if chain.Structure[i].DPLL.HoldoverParameters != nil {
			return chain.Structure[i].DPLL.HoldoverParameters, i, nil
		}
	}

	return nil, -1, fmt.Errorf("HardwareConfig %q: no HoldoverParameters found in any subsystem",
		hwConfig.Definition.Name)
}

// GetGmInterfaceFromHardwareConfig returns the GNSS-facing network interface from a HardwareConfig
// ClockChain. Prefers gnssConfig.match.ethernetInterface, then the GNSS subsystem's
// dpll.networkInterface, then the first ethernet port on that subsystem.
func GetGmInterfaceFromHardwareConfig(hwConfig *ptp.HardwareConfigBuilder) (iface.Name, error) {
	if hwConfig == nil || hwConfig.Definition == nil {
		return "", fmt.Errorf("HardwareConfig is nil")
	}

	chain := hwConfig.Definition.Spec.Profile.ClockChain
	if chain == nil {
		return "", fmt.Errorf("HardwareConfig %q has no ClockChain", hwConfig.Definition.Name)
	}

	if chain.Behavior != nil {
		for _, source := range chain.Behavior.Sources {
			if source.SourceType != ptpv2alpha1.SourceTypeGNSS {
				continue
			}

			if source.GNSSConfig != nil && source.GNSSConfig.Match != nil &&
				source.GNSSConfig.Match.EthernetInterface != "" {
				return iface.Name(source.GNSSConfig.Match.EthernetInterface), nil
			}

			if ifaceName, err := networkInterfaceForSubsystem(chain, source.Subsystem); err == nil {
				return ifaceName, nil
			}
		}
	}

	for i := range chain.Structure {
		if ifaceName, err := networkInterfaceForSubsystem(chain, chain.Structure[i].Name); err == nil {
			return ifaceName, nil
		}
	}

	return "", fmt.Errorf("HardwareConfig %q: no GM GNSS interface found in ClockChain",
		hwConfig.Definition.Name)
}

func networkInterfaceForSubsystem(chain *ptpv2alpha1.ClockChain, subsystemName string) (iface.Name, error) {
	for i := range chain.Structure {
		sub := &chain.Structure[i]
		if sub.Name != subsystemName {
			continue
		}

		if sub.DPLL.NetworkInterface != "" {
			return iface.Name(sub.DPLL.NetworkInterface), nil
		}

		for _, eth := range sub.Ethernet {
			if len(eth.Ports) > 0 && eth.Ports[0] != "" {
				return iface.Name(eth.Ports[0]), nil
			}
		}
	}

	return "", fmt.Errorf("subsystem %q has no network interface", subsystemName)
}
