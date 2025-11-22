package ocpconfig

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
	// PathToDefaultOcpParamsFile path to config file with default ocp parameters.
	PathToDefaultOcpParamsFile = "./default.yaml"
)

// OcpConfig type keeps ocp configuration.
type OcpConfig struct {
	*config.GeneralConfig
}

// NewOcpConfig returns instance of OcpConfig config type.
func NewOcpConfig() *OcpConfig {
	log.Print("Creating new OcpConfig struct")

	var ocpConf OcpConfig

	ocpConf.GeneralConfig = config.NewConfig()

	if ocpConf.GeneralConfig == nil {
		log.Print("Error to initialize GeneralConfig")

		return nil
	}

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultOcpParamsFile)

	err := readFile(&ocpConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&ocpConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &ocpConf
}

func readFile(ocpConfig *OcpConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(ocpConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(ocpConfig *OcpConfig) error {
	err := envconfig.Process("", ocpConfig)
	if err != nil {
		return err
	}

	return nil
}
