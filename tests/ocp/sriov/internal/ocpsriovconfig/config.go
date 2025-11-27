package ocpsriovconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
	SriovInterfaces           string `envconfig:"ECO_OCP_SRIOV_INTERFACE_LIST"`
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

	_, filename, _, _ := runtime.Caller(0)
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

// GetSriovInterfaces checks the ECO_OCP_SRIOV_INTERFACE_LIST env var
// and returns required number of SR-IOV interfaces.
func (sriovOcpConfig *SriovOcpConfig) GetSriovInterfaces(requestedNumber int) ([]string, error) {
	if sriovOcpConfig.SriovInterfaces == "" {
		return nil, fmt.Errorf(
			"no SR-IOV interfaces configured, check ECO_OCP_SRIOV_INTERFACE_LIST env var")
	}

	if requestedNumber < 0 {
		return nil, fmt.Errorf("requestedNumber must be non-negative, got %d", requestedNumber)
	}

	requestedInterfaceList := strings.Split(sriovOcpConfig.SriovInterfaces, ",")

	if len(requestedInterfaceList) == 0 {
		return nil, fmt.Errorf(
			"no valid SR-IOV interfaces after parsing, check ECO_OCP_SRIOV_INTERFACE_LIST env var")
	}

	if len(requestedInterfaceList) < requestedNumber {
		return nil, fmt.Errorf(
			"the number of SR-IOV interfaces is less than %d,"+
				" check ECO_OCP_SRIOV_INTERFACE_LIST env var", requestedNumber)
	}

	return requestedInterfaceList, nil
}

func readFile(sriovOcpConfig *SriovOcpConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(sriovOcpConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(sriovOcpConfig *SriovOcpConfig) error {
	err := envconfig.Process("", sriovOcpConfig)
	if err != nil {
		return err
	}

	return nil
}
