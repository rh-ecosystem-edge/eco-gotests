package systemtestsconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kelseyhightower/envconfig"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"

	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultSystemTestsParamsFile path to config file with default system tests parameters.
	PathToDefaultSystemTestsParamsFile = "./default.yaml"
)

// SystemTestsConfig type keeps general configuration.
type SystemTestsConfig struct {
	*config.GeneralConfig

	IpmiToolImage          string `yaml:"ipmitool_image" envconfig:"ECO_SYSTEM_TESTS_IPMITOOL_IMAGE"`
	CNFGoTestsClientImage  string `yaml:"cnf_gotests_client_image" envconfig:"ECO_SYSTEM_TESTS_CNF_CLIENT_IMAGE"`
	DestinationRegistryURL string `yaml:"dest_registry_url" envconfig:"ECO_SYSTEM_TESTS_DEST_REGISTRY_URL"`
}

// NewSystemTestsConfig returns instance of SystemTestsConfig type.
func NewSystemTestsConfig() *SystemTestsConfig {
	log.Print("Creating new SystemTestsConfig struct")

	var systemConf SystemTestsConfig

	systemConf.GeneralConfig = config.NewConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultSystemTestsParamsFile)

	err := readFile(&systemConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&systemConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &systemConf
}

func readFile(systemtestsConfig *SystemTestsConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&systemtestsConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(systemtestsConfig *SystemTestsConfig) error {
	err := envconfig.Process("", systemtestsConfig)
	if err != nil {
		return err
	}

	return nil
}
