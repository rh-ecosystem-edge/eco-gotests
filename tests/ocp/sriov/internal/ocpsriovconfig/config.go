// Package ocpsriovconfig provides SR-IOV specific configuration for OCP tests.
package ocpsriovconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/internal/ocpconfig"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

const (
	// PathToDefaultOcpSriovParamsFile path to config file with default ocp sriov parameters.
	PathToDefaultOcpSriovParamsFile = "./default.yaml"
	// DefaultVFNum is the default number of virtual functions to create.
	DefaultVFNum = 2
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
	VFNum                     int            `yaml:"vf_num" envconfig:"ECO_OCP_SRIOV_VF_NUM"`
}

// GetDefaultDevices returns default device configurations.
func GetDefaultDevices() []DeviceConfig {
	return []DeviceConfig{
		{Name: "e810xxv", DeviceID: "159b", Vendor: "8086", InterfaceName: "eno12409", SupportsMinTxRate: true},
		{Name: "e810c", DeviceID: "1593", Vendor: "8086", InterfaceName: "ens2f2", SupportsMinTxRate: true},
		{Name: "x710", DeviceID: "1572", Vendor: "8086", InterfaceName: "ens5f0", SupportsMinTxRate: false},
		{Name: "bcm57414", DeviceID: "16d7", Vendor: "14e4", InterfaceName: "ens3f0", SupportsMinTxRate: false},
		{Name: "bcm57508", DeviceID: "1750", Vendor: "14e4", InterfaceName: "ens4f0", SupportsMinTxRate: false},
		{Name: "cx6", DeviceID: "101d", Vendor: "15b3", InterfaceName: "ens6f0np0", SupportsMinTxRate: true},
		{Name: "cx6dx", DeviceID: "101d", Vendor: "15b3", InterfaceName: "ens7f0np0", SupportsMinTxRate: true},
		{Name: "cx7", DeviceID: "1021", Vendor: "15b3", InterfaceName: "ens8f0np0", SupportsMinTxRate: true},
	}
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

	// Parse SRIOV_DEVICES environment variable (legacy format)
	parseDevicesFromEnv(&sriovOcpConf)

	// Parse legacy SRIOV_VF_NUM if ECO_OCP_SRIOV_VF_NUM was not set
	parseLegacyVFNum(&sriovOcpConf)

	return &sriovOcpConf
}

// GetDevices returns the configured devices.
func (c *SriovOcpConfig) GetDevices() []DeviceConfig {
	if len(c.Devices) == 0 {
		return GetDefaultDevices()
	}

	return c.Devices
}

// GetVFNum returns the configured number of virtual functions.
func (c *SriovOcpConfig) GetVFNum() int {
	if c.VFNum <= 0 {
		return DefaultVFNum
	}

	return c.VFNum
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

	err = decoder.Decode(&sriovOcpConfig)
	if err != nil {
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

// parseDevicesFromEnv parses device configuration from environment variable.
// Format: export SRIOV_DEVICES="name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,..."
// Example: export SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2"
// Returns without modification if env var is not set or parsing fails.
func parseDevicesFromEnv(sriovOcpConfig *SriovOcpConfig) {
	envDevices := os.Getenv("SRIOV_DEVICES")
	if envDevices == "" {
		return
	}

	var devices []DeviceConfig

	entries := strings.Split(envDevices, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 4 && len(parts) != 5 {
			klog.Warningf("Skipping invalid SRIOV_DEVICES entry %q - expected format: name:deviceid:vendor:interface[:minTxRate]",
				entry)

			continue
		}

		supportsMinTxRate := true // default

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

	if len(devices) > 0 {
		sriovOcpConfig.Devices = devices
	} else {
		// envDevices is non-empty here (we returned early if empty), but no valid entries parsed
		klog.Warningf(
			"SRIOV_DEVICES is set to %q but no valid entries could be parsed; "+
				"expected format: name:deviceid:vendor:interface. Keeping defaults.", envDevices)
	}
}

// parseLegacyVFNum parses the legacy SRIOV_VF_NUM environment variable.
func parseLegacyVFNum(sriovOcpConfig *SriovOcpConfig) {
	// Only check legacy var if VFNum is still at default
	if sriovOcpConfig.VFNum != DefaultVFNum {
		return
	}

	vfNumStr := os.Getenv("SRIOV_VF_NUM")
	if vfNumStr == "" {
		return
	}

	vfNum, err := strconv.Atoi(vfNumStr)
	if err != nil || vfNum <= 0 {
		klog.Warningf("Invalid SRIOV_VF_NUM value %q, using default %d", vfNumStr, DefaultVFNum)

		return
	}

	sriovOcpConfig.VFNum = vfNum
}
