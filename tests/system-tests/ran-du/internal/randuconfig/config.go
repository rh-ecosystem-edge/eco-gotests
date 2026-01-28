package randuconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultRanDuParamsFile path to config file with default ran du parameters.
	PathToDefaultRanDuParamsFile = "./default.yaml"
)

// BMCDetails structure to hold BMC details.
type BMCDetails struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	BMCAddress string `json:"bmc"`
}

// NodesBMCMap holds info about BMC connection for a specific node.
type NodesBMCMap map[string]BMCDetails

// Decode - method for envconfig package to parse JSON encoded environment variables.
func (nad *NodesBMCMap) Decode(value string) error {
	nodesAuthMap := make(map[string]BMCDetails)

	// Handle empty or whitespace-only values gracefully
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		log.Print("No BMC credentials provided, using empty map")

		*nad = nodesAuthMap

		return nil
	}

	for _, record := range strings.Split(trimmedValue, ";") {
		// Skip empty records (e.g., from trailing semicolons)
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		parsedRecord := strings.Split(record, ",")
		if len(parsedRecord) != 4 {
			log.Printf("Error parsing BMC record: expected 4 entries, found %d", len(parsedRecord))
			log.Print("Expected format: nodename,username,password,bmcaddress")

			return fmt.Errorf("error parsing BMC record: expected 4 entries, found %d", len(parsedRecord))
		}

		log.Printf("Processing BMC credentials for node: %s, user: %s, address: %s",
			parsedRecord[0], parsedRecord[1], parsedRecord[3])

		nodesAuthMap[parsedRecord[0]] = BMCDetails{
			Username:   parsedRecord[1],
			Password:   parsedRecord[2],
			BMCAddress: parsedRecord[3],
		}
	}

	*nad = nodesAuthMap

	return nil
}

// RanDuConfig type keeps ran du configuration.
type RanDuConfig struct {
	*systemtestsconfig.SystemTestsConfig
	TestWorkload struct {
		Namespace      string `yaml:"namespace" envconfig:"ECO_RANDU_TESTWORKLOAD_NAMESPACE"`
		CreateMethod   string `yaml:"create_method" envconfig:"ECO_RANDU_TESTWORKLOAD_CREATE_METHOD"`
		CreateShellCmd string `yaml:"create_shell_cmd" envconfig:"ECO_RANDU_TESTWORKLOAD_CREATE_SHELLCMD"`
		DeleteShellCmd string `yaml:"delete_shell_cmd" envconfig:"ECO_RANDU_TESTWORKLOAD_DELETE_SHELLCMD"`
	} `yaml:"randu_test_workload"`
	LaunchWorkloadIterations   int `yaml:"launch_workload_iterations" envconfig:"ECO_RANDU_LAUNCH_WORKLOAD_ITERATIONS"`
	SoftRebootIterations       int `yaml:"soft_reboot_iterations" envconfig:"ECO_RANDU_SOFT_REBOOT_ITERATIONS"`
	HardRebootIterations       int `yaml:"hard_reboot_iterations" envconfig:"ECO_RANDU_HARD_REBOOT_ITERATIONS"`
	StabilityWorkloadDurMins   int `yaml:"stability_workload_duration_mins" envconfig:"ECO_RANDU_STAB_W_DUR_MINS"`
	StabilityWorkloadIntMins   int `yaml:"stability_workload_interval_mins" envconfig:"ECO_RANDU_STAB_W_INT_MINS"`
	StabilityNoWorkloadDurMins int `yaml:"stability_no_workload_duration_mins" envconfig:"ECO_RANDU_STAB_NW_DUR_MINS"`
	//nolint:lll
	StabilityNoWorkloadIntMins int         `yaml:"stability_no_workload_interval_mins" envconfig:"ECO_RANDU_STAB_NW_INT_MINS"`
	StabilityOutputPath        string      `yaml:"stability_output_path" envconfig:"ECO_RANDU_STABILITY_OUTPUT_PATH"`
	StabilityPoliciesCheck     bool        `yaml:"stability_policies_check" envconfig:"ECO_RANDU_STABILITY_POLICIES_CHECK"`
	PtpEnabled                 bool        `yaml:"ptp_enabled" envconfig:"ECO_RANDU_PTP_ENABLED"`
	RebootRecoveryTime         int         `yaml:"reboot_recovery_time" envconfig:"ECO_RANDU_RECOVERY_TIME"`
	NodesCredentialsMap        NodesBMCMap `yaml:"randu_nodes_bmc_map" envconfig:"ECO_RANDU_NODES_CREDENTIALS_MAP"`
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
