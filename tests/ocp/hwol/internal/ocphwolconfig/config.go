// Package ocphwolconfig provides HWOL-specific configuration for OCP tests.
package ocphwolconfig

import (
	"errors"
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
	// PathToDefaultOcpHwolParamsFile path to config file with default ocp hwol parameters.
	PathToDefaultOcpHwolParamsFile = "default.yaml"
)

// DeviceConfig represents a network device configuration for HWOL tests.
type DeviceConfig struct {
	Name          string `yaml:"name"`
	DeviceID      string `yaml:"device_id"`
	Vendor        string `yaml:"vendor"`
	InterfaceName string `yaml:"interface_name"`
}

// HwolOcpConfig type keeps HWOL configuration.
type HwolOcpConfig struct {
	*ocpconfig.OcpConfig
	OcpHwolOperatorNamespace string `yaml:"sriov_operator_namespace" envconfig:"ECO_OCP_HWOL_OPERATOR_NAMESPACE"`
	OcpHwolTestContainer     string `yaml:"ocp_hwol_test_container" envconfig:"ECO_OCP_HWOL_TEST_CONTAINER"`
	MCPLabel                 string `yaml:"mcp_label" envconfig:"ECO_OCP_HWOL_MCP_LABEL"`
	VFNum                    int    `yaml:"vf_num" envconfig:"ECO_OCP_HWOL_VF_NUM"`
	RecoverOnCleanupFailure  bool   `envconfig:"ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE"`
	DevicesEnv               string `envconfig:"ECO_OCP_HWOL_DEVICES"`
}

// NewHwolOcpConfig returns instance of HwolOcpConfig.
func NewHwolOcpConfig() *HwolOcpConfig {
	log.Print("Creating new HwolOcpConfig struct")

	var hwolOcpConf HwolOcpConfig

	hwolOcpConf.OcpConfig = ocpconfig.NewOcpConfig()

	if hwolOcpConf.OcpConfig == nil {
		log.Print("Error to initialize OcpConfig")

		return nil
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Print("Error: unable to determine config file path")

		return nil
	}

	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultOcpHwolParamsFile)

	err := readFile(&hwolOcpConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s: %v", confFile, err)

		return nil
	}

	err = readEnv(&hwolOcpConf)
	if err != nil {
		log.Printf("Error to read environment variables: %v", err)

		return nil
	}

	return &hwolOcpConf
}

// GetHwolDevices returns configured HWOL devices from ECO_OCP_HWOL_DEVICES.
// Devices must be supplied via environment variable; YAML device lists are not supported.
func (hwolOcpConfig *HwolOcpConfig) GetHwolDevices() ([]DeviceConfig, error) {
	if hwolOcpConfig.DevicesEnv == "" {
		return nil, fmt.Errorf("no HWOL devices configured, set ECO_OCP_HWOL_DEVICES " +
			"(format: name:deviceID:vendor:interface[,...])")
	}

	devices, err := parseHwolDevicesEnv(hwolOcpConfig.DevicesEnv)
	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no HWOL devices configured, set ECO_OCP_HWOL_DEVICES " +
			"(format: name:deviceID:vendor:interface[,...])")
	}

	return devices, nil
}

// parseHwolDevicesEnv parses device configuration from ECO_OCP_HWOL_DEVICES.
// Format: "name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,...".
func parseHwolDevicesEnv(envDevices string) ([]DeviceConfig, error) {
	var devices []DeviceConfig

	entries := strings.Split(envDevices, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 4 {
			return nil, fmt.Errorf(
				"invalid ECO_OCP_HWOL_DEVICES entry %q - expected format: name:deviceid:vendor:interface",
				entry)
		}

		devices = append(devices, DeviceConfig{
			Name:          strings.TrimSpace(parts[0]),
			DeviceID:      strings.TrimSpace(parts[1]),
			Vendor:        strings.TrimSpace(parts[2]),
			InterfaceName: strings.TrimSpace(parts[3]),
		})

		dev := &devices[len(devices)-1]
		if dev.Name == "" || dev.DeviceID == "" || dev.Vendor == "" || dev.InterfaceName == "" {
			return nil, fmt.Errorf(
				"invalid ECO_OCP_HWOL_DEVICES entry %q - name, deviceid, vendor, and interface must be non-empty",
				entry)
		}
	}

	return devices, nil
}

// GetVFNum returns the configured number of virtual functions.
// CreateSwitchdevPolicy reserves VF0 and needs at least two workload VFs.
func (hwolOcpConfig *HwolOcpConfig) GetVFNum() (int, error) {
	if hwolOcpConfig.VFNum < 2 {
		return 0, fmt.Errorf(
			"ECO_OCP_HWOL_VF_NUM must be >= 2 (got %d)", hwolOcpConfig.VFNum)
	}

	return hwolOcpConfig.VFNum, nil
}

func readFile(hwolOcpConfig *HwolOcpConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(hwolOcpConfig)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}

func readEnv(hwolOcpConfig *HwolOcpConfig) error {
	err := envconfig.Process("", hwolOcpConfig)
	if err != nil {
		return err
	}

	return nil
}
