package diskencryption

import (
	"encoding/json"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
)

// GetIgnitionConfigFromMachineConfig creates a luks IgnitionConfig from the provided machineconfig.
func GetIgnitionConfigFromMachineConfig(
	apiClient *clients.Settings, machineConfigName string) (*IgnitionConfig, error) {
	machineConfigBuilder, err := mco.PullMachineConfig(apiClient, machineConfigName)
	if err != nil {
		return nil, fmt.Errorf("error pulling machineconfig %s due to %w", machineConfigName, err)
	}

	ignitionConfig, err := createIgnitionFromMachineConfig(machineConfigBuilder)
	if err != nil {
		return nil, fmt.Errorf("error creating ignition config from machineconfig %s due to %w", machineConfigName, err)
	}

	if len(ignitionConfig.Storage.LUKS) != 1 {
		return nil, fmt.Errorf("error received multiple luks devices from machineconfig %s and expected 1", machineConfigName)
	}

	return &ignitionConfig, nil
}

func createIgnitionFromMachineConfig(builder *mco.MCBuilder) (IgnitionConfig, error) {
	var ignitionConfig IgnitionConfig

	err := json.Unmarshal(builder.Object.Spec.Config.Raw, &ignitionConfig)
	if err != nil {
		return IgnitionConfig{}, err
	}

	return ignitionConfig, nil
}
