package coreconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/cnfconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultCnfCoreParamsFile path to config file with default core parameters.
	PathToDefaultCnfCoreParamsFile = "./default.yaml"
)

// CoreConfig type keeps core configuration.
type CoreConfig struct {
	*cnfconfig.CNFConfig
}

// NewCoreConfig returns instance of CoreConfig config type.
func NewCoreConfig() *CoreConfig {
	log.Print("Creating new CoreConfig struct")

	var coreConf CoreConfig

	coreConf.CNFConfig = cnfconfig.NewCNFConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultCnfCoreParamsFile)

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

func readFile(coreConfig *CoreConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&coreConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(coreConfig *CoreConfig) error {
	err := envconfig.Process("", coreConfig)
	if err != nil {
		return err
	}

	return nil
}
