package rhwaconfig

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultRhwaParamsFile path to config file with default rhwa parameters.
	PathToDefaultRhwaParamsFile = "./default.yaml"
)

// BMCDetails holds BMC connection details for a single node.
type BMCDetails struct {
	Address  string `yaml:"address" json:"address"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// Decode implements the envconfig.Decoder interface to parse a JSON string
// from an environment variable into BMCDetails.
func (b *BMCDetails) Decode(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return json.Unmarshal([]byte(value), b)
}

// RHWAConfig type keeps rhwa configuration.
type RHWAConfig struct {
	*config.GeneralConfig

	// NHC/SNR sudden-loss test configuration.
	TargetWorker    string     `yaml:"nhc_target_worker" envconfig:"ECO_RHWA_NHC_TARGET_WORKER"`
	FailoverWorkers []string   `yaml:"nhc_failover_workers" envconfig:"ECO_RHWA_NHC_FAILOVER_WORKERS"`
	StorageClass    string     `yaml:"nhc_storage_class" envconfig:"ECO_RHWA_NHC_STORAGE_CLASS"`
	AppImage        string     `yaml:"nhc_app_image" envconfig:"ECO_RHWA_NHC_APP_IMAGE"`
	TargetWorkerBMC BMCDetails `yaml:"nhc_target_worker_bmc" envconfig:"ECO_RHWA_NHC_TARGET_WORKER_BMC"`
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
