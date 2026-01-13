// Package ocpsriovconfig provides SR-IOV specific configuration for OCP tests.
package ocpsriovconfig

import (
	"fmt"
	"io"
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
	PathToDefaultOcpSriovParamsFile = "default.yaml"
	// defaultSupportsMinTxRate is the default value for SupportsMinTxRate when parsing from env.
	defaultSupportsMinTxRate = true
)

// DeviceConfig represents a SR-IOV device configuration.
type DeviceConfig struct {
	Name              string `yaml:"name"`
	DeviceID          string `yaml:"device_id"`
	Vendor            string `yaml:"vendor"`
	InterfaceName     string `yaml:"interface_name"`
	SupportsMinTxRate bool   `yaml:"supports_min_tx_rate"`
}

// SriovOcpConfig type keeps sriov configuration.
type SriovOcpConfig struct {
	*ocpconfig.OcpConfig
	OcpSriovOperatorNamespace string         `yaml:"sriov_operator_namespace" envconfig:"ECO_OCP_SRIOV_OPERATOR_NAMESPACE"`
	OcpSriovTestContainer     string         `yaml:"ocp_sriov_test_container" envconfig:"ECO_OCP_SRIOV_TEST_CONTAINER"`
	SriovInterfaces           string         `envconfig:"ECO_OCP_SRIOV_INTERFACE_LIST"`
	Devices                   []DeviceConfig `yaml:"devices"`
	DevicesEnv                string         `envconfig:"ECO_OCP_SRIOV_DEVICES"`
	VFNum                     int            `yaml:"vf_num" envconfig:"ECO_OCP_SRIOV_VF_NUM"`
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
		log.Printf("Error to read config file %s: %v", confFile, err)

		return nil
	}

	err = readEnv(&sriovOcpConf)
	if err != nil {
		log.Printf("Error to read environment variables: %v", err)

		return nil
	}

	return &sriovOcpConf
}

// GetSriovDevices returns configured SR-IOV devices.
// If ECO_OCP_SRIOV_DEVICES env var is set, parses devices from it.
// Otherwise returns devices from YAML configuration.
func (sriovOcpConfig *SriovOcpConfig) GetSriovDevices() ([]DeviceConfig, error) {
	// Check for ECO_OCP_SRIOV_DEVICES environment variable override
	if sriovOcpConfig.DevicesEnv != "" {
		devices, err := parseSriovDevicesEnv(sriovOcpConfig.DevicesEnv)
		if err != nil {
			return nil, err
		}

		if len(devices) > 0 {
			return devices, nil
		}
	}

	// Return devices from YAML configuration
	if len(sriovOcpConfig.Devices) == 0 {
		return nil, fmt.Errorf("no SR-IOV devices configured, check ECO_OCP_SRIOV_DEVICES env var or devices in YAML")
	}

	return sriovOcpConfig.Devices, nil
}

// parseSriovDevicesEnv parses device configuration from ECO_OCP_SRIOV_DEVICES environment variable.
// Format: "name1:deviceid1:vendor1:interface1[:minTxRate],name2:deviceid2:vendor2:interface2[:minTxRate],..."
// Example: "e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2".
// Note: SupportsMinTxRate defaults to true when omitted from env var.
// For YAML-defined devices, ensure supports_min_tx_rate is explicitly set.
func parseSriovDevicesEnv(envDevices string) ([]DeviceConfig, error) {
	var devices []DeviceConfig

	entries := strings.Split(envDevices, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 4 && len(parts) != 5 {
			return nil, fmt.Errorf(
				"invalid ECO_OCP_SRIOV_DEVICES entry %q - expected format: name:deviceid:vendor:interface[:minTxRate]",
				entry)
		}

		supportsMinTxRate := defaultSupportsMinTxRate

		if len(parts) == 5 {
			val := strings.ToLower(strings.TrimSpace(parts[4]))
			supportsMinTxRate = val == "true" || val == "1" || val == "yes"
		}

		devices = append(devices, DeviceConfig{
			Name:              strings.TrimSpace(parts[0]),
			DeviceID:          strings.TrimSpace(parts[1]),
			Vendor:            strings.TrimSpace(parts[2]),
			InterfaceName:     strings.TrimSpace(parts[3]),
			SupportsMinTxRate: supportsMinTxRate,
		})
	}

	return devices, nil
}

// GetVFNum returns the configured number of virtual functions.
// Returns an error if VFNum is not properly configured.
func (sriovOcpConfig *SriovOcpConfig) GetVFNum() (int, error) {
	if sriovOcpConfig.VFNum <= 0 {
		return 0, fmt.Errorf(
			"no SR-IOV VFs configured, check env var ECO_OCP_SRIOV_VF_NUM")
	}

	return sriovOcpConfig.VFNum, nil
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
	if err != nil && err != io.EOF {
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
