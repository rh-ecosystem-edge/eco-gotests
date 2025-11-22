package accelconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

const (
	// PathToDefaultAccelParamsFile path to config file with default accel tests parameters.
	PathToDefaultAccelParamsFile = "./default.yaml"
)

// AccelConfig contains environment information related to ocp upgrade tests.
type AccelConfig struct {
	PullSecret           string `envconfig:"ECO_ACCEL_PULL_SECRET"`
	Registry             string `envconfig:"ECO_ACCEL_REGISTRY"`
	UpgradeTargetVersion string `envconfig:"ECO_ACCEL_UPGRADE_TARGET_IMAGE"`
	SpokeKubeConfig      string `envconfig:"ECO_ACCEL_SPOKE_KUBECONFIG"`
	HubClusterName       string `envconfig:"ECO_ACCEL_HUB_CLUSTER_NAME"`
	HubMinorVersion      string `envconfig:"ECO_ACCEL_HUB_MINOR_VERSION"`
	IBUWorkloadImage     string `yaml:"ibu_workload_image" envconfig:"ECO_ACCEL_WORKLOAD_IMAGE"`
	SpokeAPIClient       *clients.Settings
	*config.GeneralConfig
}

// NewAccelConfig returns instance of AccelConfig type.
func NewAccelConfig() *AccelConfig {
	log.Print("Creating new AccelConfig")

	var accelConfig AccelConfig

	accelConfig.GeneralConfig = config.NewConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	configFile := filepath.Join(baseDir, PathToDefaultAccelParamsFile)

	err := readFile(&accelConfig, configFile)
	if err != nil {
		klog.V(90).Infof("Error reading config file %s", configFile)

		return nil
	}

	err = envconfig.Process("eco_accel_", &accelConfig)
	if err != nil {
		log.Printf("failed to instantiate AccelConfig: %v", err)

		return nil
	}

	if accelConfig.SpokeKubeConfig != "" {
		klog.V(90).Infof("Creating spoke api client from %s", accelConfig.SpokeKubeConfig)

		if accelConfig.SpokeAPIClient = clients.New(
			accelConfig.SpokeKubeConfig); accelConfig.SpokeAPIClient == nil {
			klog.V(90).Infof("failed to load provided spoke kubeconfig")
		}
	} else {
		accelConfig.SpokeAPIClient = nil
	}

	return &accelConfig
}

func readFile(accelConfig *AccelConfig, configFile string) error {
	openedConfigFile, err := os.Open(configFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedConfigFile.Close()
	}()

	decoder := yaml.NewDecoder(openedConfigFile)

	err = decoder.Decode(&accelConfig)

	return err
}
