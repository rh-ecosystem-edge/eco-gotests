package cnfconfig

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
	// PathToDefaultCnfParamsFile path to config file with default cnf parameters.
	PathToDefaultCnfParamsFile = "./default.yaml"
)

// CNFConfig type keeps core configuration.
type CNFConfig struct {
	*config.GeneralConfig
}

// NewCNFConfig returns instance of CNF config type.
func NewCNFConfig() *CNFConfig {
	log.Print("Creating new CNFConfig struct")

	var coreConf CNFConfig

	coreConf.GeneralConfig = config.NewConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultCnfParamsFile)

	err := readFile(&coreConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&coreConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &coreConf
}

func readFile(cnfConfig *CNFConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&cnfConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(cnfConfig *CNFConfig) error {
	err := envconfig.Process("", cnfConfig)
	if err != nil {
		return err
	}

	return nil
}
