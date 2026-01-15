package seedgenerationconfig

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"k8s.io/klog/v2"
)

// SeedGenerationConfig contains configuration for seed generation tests.
type SeedGenerationConfig struct {
	*config.GeneralConfig
	TargetSNOKubeConfig string `envconfig:"ECO_LCA_IBU_CNF_KUBECONFIG_TARGET_SNO"`
	IbguSeedImage       string `yaml:"ibgu_seed_image" envconfig:"ECO_LCA_IBGU_SEED_IMAGE"`
}

// NewSeedGenerationConfig returns an instance of SeedGenerationConfig.
func NewSeedGenerationConfig() *SeedGenerationConfig {
	klog.V(90).Info("Creating new SeedGenerationConfig struct")

	var seedConfig SeedGenerationConfig

	seedConfig.GeneralConfig = config.NewConfig()
	if seedConfig.GeneralConfig == nil {
		klog.V(90).Info("Failed to initialize GeneralConfig")

		return nil
	}

	err := envconfig.Process("", &seedConfig)
	if err != nil {
		klog.V(90).Infof("Error reading environment variables: %v", err)

		return nil
	}

	return &seedConfig
}

// GetTargetSNOAPIClient returns the API client for the target SNO cluster.
func (config *SeedGenerationConfig) GetTargetSNOAPIClient() *clients.Settings {
	if config == nil || config.TargetSNOKubeConfig == "" {
		return nil
	}

	return clients.New(config.TargetSNOKubeConfig)
}
