package config

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

const (
	// PathToDefaultDiskEncryptionParamsFile path to config file with default disk encryption parameters.
	PathToDefaultDiskEncryptionParamsFile = "./default.yaml"
)

// DiskEncrptionConfig type keeps disk encryption configuration.
type DiskEncrptionConfig struct {
	*systemtestsconfig.SystemTestsConfig
	// BMCClient provides access to the BMC. Nil when BMC configs are not provided.
	Spoke1BMC   *bmc.BMC
	BMCUsername string        `yaml:"BMCUsername" envconfig:"ECO_SYSTEM_TESTS_BMC_USERNAME"`
	BMCPassword string        `yaml:"BMCPassword" envconfig:"ECO_SYSTEM_TESTS_BMC_PASSWORD"`
	BMCHosts    []string      `yaml:"BMCHosts" envconfig:"ECO_SYSTEM_TESTS_BMC_HOSTS"`
	BMCTimeout  time.Duration `yaml:"BMCTimeout" envconfig:"ECO_SYSTEM_TESTS_BMC_TIMEOUT"`
}

// NewDiskEncryptionConfig returns instance of DiskEncrptionConfig config type.
func NewDiskEncryptionConfig() *DiskEncrptionConfig {
	log.Print("Creating new DiskEncrptionConfig struct")

	var diskEncryptionConf DiskEncrptionConfig

	diskEncryptionConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultDiskEncryptionParamsFile)

	err := readFile(&diskEncryptionConf, confFile)
	if err != nil {
		log.Printf("Error reading config file %s", confFile)

		return nil
	}

	err = readEnv(&diskEncryptionConf)
	if err != nil {
		log.Print("Error reading environment variables")

		return nil
	}

	if len(diskEncryptionConf.BMCHosts) > 0 &&
		diskEncryptionConf.BMCUsername != "" &&
		diskEncryptionConf.BMCPassword != "" {
		bmcHost := diskEncryptionConf.BMCHosts[0]
		if len(diskEncryptionConf.BMCHosts) > 1 {
			klog.V(tsparams.LogLevel).Infof("Found more than one BMC host, using the first one: %s", bmcHost)
		}

		diskEncryptionConf.Spoke1BMC = bmc.New(bmcHost).
			WithRedfishUser(diskEncryptionConf.BMCUsername, diskEncryptionConf.BMCPassword).
			WithRedfishTimeout(diskEncryptionConf.BMCTimeout).
			WithSSHUser(diskEncryptionConf.BMCUsername, diskEncryptionConf.BMCPassword)
	}

	return &diskEncryptionConf
}

func readFile(diskEncrptionConfig *DiskEncrptionConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&diskEncrptionConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(diskEncrptionConfig *DiskEncrptionConfig) error {
	err := envconfig.Process("", diskEncrptionConfig)
	if err != nil {
		return err
	}

	return nil
}
