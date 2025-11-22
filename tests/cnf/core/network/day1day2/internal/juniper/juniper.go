package juniper

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

// InterfaceConfigs stores configuration strings for switch interfaces.
var InterfaceConfigs []string

// DumpInterfaceConfigs fetches and stores interface configurations for the specified switch interfaces.
func DumpInterfaceConfigs(currentSession *JunosSession, switchInterfaces []string) error {
	klog.V(90).Infof("Dumping configuration for the switch interfaces: %v", switchInterfaces)

	for _, switchInterface := range switchInterfaces {
		config, err := currentSession.GetInterfaceConfig(switchInterface)
		if err != nil {
			return err
		}

		InterfaceConfigs = append(InterfaceConfigs, config)
	}

	return nil
}

// RemoveAllConfigurationFromInterfaces removes all configuration from given switch interfaces.
func RemoveAllConfigurationFromInterfaces(currentSession *JunosSession, switchInterfaces []string) error {
	klog.V(90).Infof("Removing configuration from the switch interfaces: %v", switchInterfaces)

	for _, switchInterface := range switchInterfaces {
		commands := []string{fmt.Sprintf("edit interfaces %s", switchInterface), DeleteAction}

		err := currentSession.Config(commands)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetNonLacpLag configures non-LACP Link Aggregated ports on Junos devices.
func SetNonLacpLag(currentSession *JunosSession, slaveInterfaceNames []string, aggregatedInterfaceName string) error {
	klog.V(90).Infof("Creating Link Aggregated switch interface %s with enslaved ports %v",
		aggregatedInterfaceName, slaveInterfaceNames)

	var commands []string

	if len(slaveInterfaceNames) > 0 {
		for _, slaveInterfaceName := range slaveInterfaceNames {
			commands = append(commands, fmt.Sprintf("set interfaces %s ether-options 802.3ad %s",
				slaveInterfaceName, aggregatedInterfaceName))
		}
	}

	commands = append(commands, fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching",
		aggregatedInterfaceName))

	return currentSession.Config(commands)
}

// SetVlanOnTrunkInterface configures VLAN on a trunk interface.
func SetVlanOnTrunkInterface(currentSession *JunosSession, vlan, switchInterface string) error {
	klog.V(90).Infof("Configuring vlan %s on the trunk interface %s", vlan, switchInterface)

	return currentSession.Config([]string{
		fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk vlan members vlan%s",
			switchInterface, vlan)})
}

// DisableSwitchInterface disables a specified switch interface.
func DisableSwitchInterface(currentSession *JunosSession, switchInterface string) error {
	klog.V(90).Infof("Disabling interface: %s", switchInterface)

	return setSwitchInterfaceStatus(currentSession, switchInterface, SetAction)
}

// EnableSwitchInterface enables a specified switch interface.
func EnableSwitchInterface(currentSession *JunosSession, switchInterface string) error {
	klog.V(90).Infof("Enabling interface: %s", switchInterface)

	return setSwitchInterfaceStatus(currentSession, switchInterface, DeleteAction)
}

// IsSwitchInterfaceUp checks if a switch interface is up.
func IsSwitchInterfaceUp(currentSession *JunosSession, switchInterface string) (bool, error) {
	klog.V(90).Infof("Checking is interface %s up", switchInterface)

	jsonOutput, err := currentSession.RunOperationalCMD(fmt.Sprintf("show interfaces %s", switchInterface))
	if err != nil {
		return false, err
	}

	var interfaceStatus InterfaceStatus

	err = json.Unmarshal([]byte(jsonOutput), &interfaceStatus)
	if err != nil {
		return false, err
	}

	return interfaceStatus.InterfaceInformation[0].PhysicalInterface[0].OperStatus[0].Data == "up", nil
}

// RestoreSwitchInterfacesConfiguration restores the configuration of specified switch interfaces.
func RestoreSwitchInterfacesConfiguration(currentSession *JunosSession, switchInterfaces []string) error {
	klog.V(90).Infof("Restoring configuration for the interfaces: %v", switchInterfaces)

	err := RemoveAllConfigurationFromInterfaces(currentSession, switchInterfaces)
	if err != nil {
		return err
	}

	return restoreInterfaceConfigs(currentSession)
}

// DeleteInterfaces deletes given interfaces.
func DeleteInterfaces(currentSession *JunosSession, interfaceNames []string) error {
	klog.V(90).Infof("Deleting the interfaces: %v", interfaceNames)

	var commands []string
	for _, interfaceName := range interfaceNames {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", interfaceName))
	}

	return currentSession.Config(commands)
}

func restoreInterfaceConfigs(currentSession *JunosSession) error {
	klog.V(90).Infof("Restoring all saved configuration")

	if len(InterfaceConfigs) > 0 {
		var err error
		for _, interfaceConfig := range InterfaceConfigs {
			err = currentSession.ApplyConfigInterface(interfaceConfig)
			if err != nil {
				return err
			}
		}

		InterfaceConfigs = []string{}
	}

	return nil
}

func setSwitchInterfaceStatus(currentSession *JunosSession, switchInterface, action string) error {
	klog.V(90).Infof("Changing interface %s status with action %s", switchInterface, action)

	if action != SetAction && action != DeleteAction {
		return fmt.Errorf("unknown action %s", action)
	}

	return currentSession.Config([]string{fmt.Sprintf("%s interfaces %s disable", action, switchInterface)})
}
