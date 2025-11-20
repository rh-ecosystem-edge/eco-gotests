package tsparams

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Note: NetworkConfig has been replaced by SriovOcpConfig in ocpsriovconfig package
// This file now only contains DeviceConfig and related functions

// DeviceConfig represents a SR-IOV device configuration
type DeviceConfig struct {
	Name          string
	DeviceID      string
	Vendor        string
	InterfaceName string
}

// GetDefaultDeviceConfig returns default device configurations
func GetDefaultDeviceConfig() []DeviceConfig {
	return []DeviceConfig{
		{"e810xxv", "159b", "8086", "eno12409"},
		{"e810c", "1593", "8086", "ens2f2"},
		{"x710", "1572", "8086", "ens5f0"}, //NO-CARRIER
		{"bcm57414", "16d7", "14e4", "ens4f1np1"},
		{"bcm57508", "1750", "14e4", "ens3f0np0"}, //NO-CARRIER
		{"e810back", "1591", "8086", "ens4f2"},
		{"cx7anl244", "1021", "15b3", "ens2f0np0"},
	}
}

// parseDeviceConfig parses device configuration from environment variable
// Format: export SRIOV_DEVICES="name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,..."
// Example: export SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2"
// Returns empty slice if env var is not set or parsing fails
func parseDeviceConfig() []DeviceConfig {
	envDevices := os.Getenv("SRIOV_DEVICES")
	if envDevices == "" {
		return nil
	}

	var devices []DeviceConfig
	entries := strings.Split(envDevices, ",")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 4 {
			// Cannot use GinkgoLogr in internal package - just skip invalid entries
			continue
		}

		devices = append(devices, DeviceConfig{
			Name:          strings.TrimSpace(parts[0]),
			DeviceID:      strings.TrimSpace(parts[1]),
			Vendor:        strings.TrimSpace(parts[2]),
			InterfaceName: strings.TrimSpace(parts[3]),
		})
	}

	return devices
}

// GetDeviceConfig returns device configuration from environment variable or defaults
func GetDeviceConfig() []DeviceConfig {
	envDevices := os.Getenv("SRIOV_DEVICES")
	if devices := parseDeviceConfig(); len(devices) > 0 {
		return devices
	}
	if envDevices != "" {
		panic(fmt.Sprintf("SRIOV_DEVICES is set to %q but no valid entries could be parsed; expected format: name:deviceid:vendor:interface", envDevices))
	}
	return GetDefaultDeviceConfig()
}

// GetVFNum returns the number of virtual functions to create
func GetVFNum() int {
	if vfNumStr := os.Getenv("ECO_OCP_SRIOV_VF_NUM"); vfNumStr != "" {
		if vfNum, err := strconv.Atoi(vfNumStr); err == nil && vfNum > 0 {
			return vfNum
		}
	}
	if vfNumStr := os.Getenv("SRIOV_VF_NUM"); vfNumStr != "" {
		if vfNum, err := strconv.Atoi(vfNumStr); err == nil && vfNum > 0 {
			return vfNum
		}
	}
	return 2 // default
}

// Note: ReporterNamespacesToDump and ReporterCRDsToDump are now defined in ocpsriovvars.go
// This file focuses on DeviceConfig and VF configuration
