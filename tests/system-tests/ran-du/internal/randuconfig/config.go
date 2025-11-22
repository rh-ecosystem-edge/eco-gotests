package randuconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultRanDuParamsFile path to config file with default ran du parameters.
	PathToDefaultRanDuParamsFile = "./default.yaml"
)

// RanDuConfig type keeps ran du configuration.
type RanDuConfig struct {
	*systemtestsconfig.SystemTestsConfig
	TestWorkload struct {
		Namespace      string `yaml:"namespace" envconfig:"ECO_RANDU_TESTWORKLOAD_NAMESPACE"`
		CreateMethod   string `yaml:"create_method" envconfig:"ECO_RANDU_TESTWORKLOAD_CREATE_METHOD"`
		CreateShellCmd string `yaml:"create_shell_cmd" envconfig:"ECO_RANDU_TESTWORKLOAD_CREATE_SHELLCMD"`
		DeleteShellCmd string `yaml:"delete_shell_cmd" envconfig:"ECO_RANDU_TESTWORKLOAD_DELETE_SHELLCMD"`
	} `yaml:"randu_test_workload"`
	LaunchWorkloadIterations   int    `yaml:"launch_workload_iterations" envconfig:"ECO_RANDU_LAUNCH_WORKLOAD_ITERATIONS"`
	SoftRebootIterations       int    `yaml:"soft_reboot_iterations" envconfig:"ECO_RANDU_SOFT_REBOOT_ITERATIONS"`
	HardRebootIterations       int    `yaml:"hard_reboot_iterations" envconfig:"ECO_RANDU_HARD_REBOOT_ITERATIONS"`
	StabilityWorkloadDurMins   int    `yaml:"stability_workload_duration_mins" envconfig:"ECO_RANDU_STAB_W_DUR_MINS"`
	StabilityWorkloadIntMins   int    `yaml:"stability_workload_interval_mins" envconfig:"ECO_RANDU_STAB_W_INT_MINS"`
	StabilityNoWorkloadDurMins int    `yaml:"stability_no_workload_duration_mins" envconfig:"ECO_RANDU_STAB_NW_DUR_MINS"`
	StabilityNoWorkloadIntMins int    `yaml:"stability_no_workload_interval_mins" envconfig:"ECO_RANDU_STAB_NW_INT_MINS"`
	StabilityOutputPath        string `yaml:"stability_output_path" envconfig:"ECO_RANDU_STABILITY_OUTPUT_PATH"`
	StabilityPoliciesCheck     bool   `yaml:"stability_policies_check" envconfig:"ECO_RANDU_STABILITY_POLICIES_CHECK"`
	PtpEnabled                 bool   `yaml:"ptp_enabled" envconfig:"ECO_RANDU_PTP_ENABLED"`
	RebootRecoveryTime         int    `yaml:"reboot_recovery_time" envconfig:"ECO_RANDU_RECOVERY_TIME"`
}

// NewRanDuConfig returns instance of RanDuConfig config type.
func NewRanDuConfig() *RanDuConfig {
	log.Print("Creating new RanDuConfig struct")

	var randuConf RanDuConfig

	randuConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultRanDuParamsFile)

	err := readFile(&randuConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&randuConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &randuConf
}

func readFile(randuConfig *RanDuConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&randuConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(randuConfig *RanDuConfig) error {
	err := envconfig.Process("", randuConfig)
	if err != nil {
		return err
	}

	return nil
}
