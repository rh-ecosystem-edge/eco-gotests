// Package sriovconfig provides SR-IOV specific configuration for OCP tests.
package sriovconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/internal/ocpconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultOcpSriovParamsFile path to config file with default ocp sriov parameters.
	PathToDefaultOcpSriovParamsFile = "./default.yaml"
)

// SriovOcpConfig type keeps sriov configuration.
type SriovOcpConfig struct {
	*ocpconfig.OcpConfig
	OcpSriovOperatorNamespace string `yaml:"sriov_operator_namespace" envconfig:"ECO_OCP_SRIOV_OPERATOR_NAMESPACE"`
	OcpSriovTestContainer     string `yaml:"ocp_sriov_test_container" envconfig:"ECO_OCP_SRIOV_TEST_CONTAINER"`
	SriovInterfaces           string `envconfig:"ECO_OCP_SRIOV_SRIOV_INTERFACE_LIST"`
}

// sriovYAMLConfig is used to decode only sriov-specific fields from YAML
// without affecting the embedded OcpConfig/GeneralConfig structs.
type sriovYAMLConfig struct {
	OcpSriovOperatorNamespace string `yaml:"sriov_operator_namespace"`
	OcpSriovTestContainer     string `yaml:"ocp_sriov_test_container"`
}

// sriovEnvConfig is used to decode only sriov-specific fields from environment
// without affecting the embedded OcpConfig/GeneralConfig structs.
type sriovEnvConfig struct {
	OcpSriovOperatorNamespace string `envconfig:"ECO_OCP_SRIOV_OPERATOR_NAMESPACE"`
	OcpSriovTestContainer     string `envconfig:"ECO_OCP_SRIOV_TEST_CONTAINER"`
	SriovInterfaces           string `envconfig:"ECO_OCP_SRIOV_SRIOV_INTERFACE_LIST"`
}

// NewSriovOcpConfig returns instance of SriovConfig config type.
func NewSriovOcpConfig() *SriovOcpConfig {
	log.Print("Creating new SriovOcpConfig struct")

	var sriovOcpConf SriovOcpConfig

	sriovOcpConf.OcpConfig = ocpconfig.NewOcpConfig()

	if sriovOcpConf.OcpConfig == nil {
		log.Print("Error to initialize OcpConfig")

		return nil
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Print("Error: unable to determine config file path")

		return nil
	}

	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultOcpSriovParamsFile)

	err := readFile(&sriovOcpConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&sriovOcpConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &sriovOcpConf
}

func readFile(sriovOcpConfig *SriovOcpConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	// Use a temporary struct to decode only sriov-specific fields
	// This prevents the YAML decoder from affecting embedded OcpConfig/GeneralConfig
	var yamlConf sriovYAMLConfig

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&yamlConf)
	if err != nil {
		return err
	}

	// Copy decoded values to the actual config
	sriovOcpConfig.OcpSriovOperatorNamespace = yamlConf.OcpSriovOperatorNamespace
	sriovOcpConfig.OcpSriovTestContainer = yamlConf.OcpSriovTestContainer

	return nil
}

func readEnv(sriovOcpConfig *SriovOcpConfig) error {
	// Only process environment variables for sriov-specific fields
	// Use a temporary struct to avoid affecting embedded configs
	var envConf sriovEnvConfig

	err := envconfig.Process("", &envConf)
	if err != nil {
		return err
	}

	// Override with env values if set
	if envConf.OcpSriovOperatorNamespace != "" {
		sriovOcpConfig.OcpSriovOperatorNamespace = envConf.OcpSriovOperatorNamespace
	}

	if envConf.OcpSriovTestContainer != "" {
		sriovOcpConfig.OcpSriovTestContainer = envConf.OcpSriovTestContainer
	}

	if envConf.SriovInterfaces != "" {
		sriovOcpConfig.SriovInterfaces = envConf.SriovInterfaces
	}

	return nil
}
