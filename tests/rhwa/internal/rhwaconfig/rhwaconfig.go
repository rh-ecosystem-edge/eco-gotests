package rhwaconfig

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
	// PathToDefaultRhwaParamsFile path to config file with default rhwa parameters.
	PathToDefaultRhwaParamsFile = "./default.yaml"
)

// RHWAConfig type keeps rhwa configuration.
type RHWAConfig struct {
	*config.GeneralConfig
}

// NewRHWAConfig returns instance of RHWA config type.
func NewRHWAConfig() *RHWAConfig {
	log.Print("Creating new RHWAConfig struct")

	var rhwaConf RHWAConfig

	rhwaConf.GeneralConfig = config.NewConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultRhwaParamsFile)

	err := readFile(&rhwaConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&rhwaConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &rhwaConf
}

func readFile(rhwaConfig *RHWAConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&rhwaConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(rhwaConfig *RHWAConfig) error {
	err := envconfig.Process("", rhwaConfig)
	if err != nil {
		return err
	}

	return nil
}
