package tsparams

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openshift-kni/k8sreporter"
	corev1 "k8s.io/api/core/v1"
)

// NetworkConfig represents network configuration
type NetworkConfig struct {
	WorkerLabel            string
	CnfNetTestContainer    string
	CnfMcpLabel            string
	SriovOperatorNamespace string
	WorkerLabelMap         map[string]string
}

// GetJunitReportPath returns the junit report path
func (nc *NetworkConfig) GetJunitReportPath() string {
	return "/tmp/junit.xml"
}

// GetReportPath returns the report path
func (nc *NetworkConfig) GetReportPath() string {
	return "/tmp/reports"
}

// TCPrefix returns the test case prefix
func (nc *NetworkConfig) TCPrefix() string {
	return "OCP-SRIOV"
}

// GetNetworkConfig returns the network configuration
func GetNetworkConfig() *NetworkConfig {
	// Allow override via environment variable for multi-arch support (e.g., ARM64)
	testContainer := os.Getenv("ECO_OCP_SRIOV_TEST_CONTAINER")
	if testContainer == "" {
		testContainer = os.Getenv("ECO_SRIOV_TEST_CONTAINER")
		if testContainer == "" {
			testContainer = "quay.io/openshift-kni/cnf-tests:4.16"
		}
	}

	return &NetworkConfig{
		WorkerLabel:            "node-role.kubernetes.io/worker",
		CnfNetTestContainer:    testContainer,
		CnfMcpLabel:            "machineconfiguration.openshift.io/role=worker",
		SriovOperatorNamespace: "openshift-sriov-network-operator",
		WorkerLabelMap:         map[string]string{"node-role.kubernetes.io/worker": ""},
	}
}

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

var (
	// ReporterNamespacesToDump tells the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-sriov-network-operator": "sriov-operator",
		TestNamespaceName:                  "sriov-tests",
	}

	// ReporterCRDsToDump tells the reporter what CRs to dump.
	// Note: SR-IOV CRDs will be added via scheme registration in reporter
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &corev1.EventList{}},
	}
)
