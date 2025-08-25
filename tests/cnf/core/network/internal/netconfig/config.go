package netconfig

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultCnfCoreNetParamsFile path to config file with default network parameters.
	PathToDefaultCnfCoreNetParamsFile = "./default.yaml"
)

// NetworkConfig type keeps network configuration.
type NetworkConfig struct {
	*coreconfig.CoreConfig
	CnfNetTestContainer            string `yaml:"cnf_net_test_container" envconfig:"ECO_CNF_CORE_NET_TEST_CONTAINER"`
	DpdkTestContainer              string `yaml:"dpdk_test_container" envconfig:"ECO_CNF_CORE_NET_DPDK_TEST_CONTAINER"`
	MlbOperatorNamespace           string `yaml:"metal_lb_operator_namespace" envconfig:"ECO_CNF_CORE_NET_MLB_OPERATOR_NAMESPACE"` //nolint:lll
	Frrk8sNamespace                string `yaml:"frr-k8s_namespace" envconfig:"ECO_CNF_CORE_NET_FRR-K8S_NAMESPACE"`
	PFStatusRelayOperatorNamespace string `yaml:"pf_status_relay_operator_namespace" envconfig:"ECO_CNF_CORE_NET_PF_STATUS_RELAY_OPERATOR_NAMESPACE"` //nolint:lll
	CnfMcpLabel                    string `yaml:"cnf_mcp_label" envconfig:"ECO_CNF_CORE_NET_CNF_MCP_LABEL"`
	MultusNamesapce                string `yaml:"multus_namespace" envconfig:"ECO_CNF_CORE_NET_MULTUS_NAMESPACE"`
	SwitchUser                     string `envconfig:"ECO_CNF_CORE_NET_SWITCH_USER"`
	SwitchPass                     string `envconfig:"ECO_CNF_CORE_NET_SWITCH_PASS"`
	SwitchIP                       string `envconfig:"ECO_CNF_CORE_NET_SWITCH_IP"`
	SwitchInterfaces               string `envconfig:"ECO_CNF_CORE_NET_SWITCH_INTERFACES"`
	PrimarySwitchInterfaces        string `envconfig:"ECO_CNF_CORE_NET_PRIMARY_SWITCH_INTERFACES"`
	SwitchLagNames                 string `envconfig:"ECO_CNF_CORE_NET_SWITCH_LAGS"`
	ClusterVlan                    string `envconfig:"ECO_CNF_CORE_NET_CLUSTER_VLAN"`
	//nolint:lll
	PrometheusOperatorNamespace string `yaml:"prometheus_operator_namespace" envconfig:"ECO_CNF_CORE_NET_PROMETHEUS_OPERATOR_NAMESPACE"`
	MlbAddressPoolIP            string `envconfig:"ECO_CNF_CORE_NET_MLB_ADDR_LIST"`
	SriovInterfaces             string `envconfig:"ECO_CNF_CORE_NET_SRIOV_INTERFACE_LIST"`
	FrrImage                    string `yaml:"frr_image" envconfig:"ECO_CNF_CORE_NET_FRR_IMAGE"`
	VLAN                        string `envconfig:"ECO_CNF_CORE_NET_VLAN"`
	BMCHostNames                string `envconfig:"ECO_CNF_CORE_NET_BMC_HOST_NAMES"`
	BMCHostUser                 string `envconfig:"ECO_CNF_CORE_NET_BMC_HOST_USER"`
	BMCHostPass                 string `envconfig:"ECO_CNF_CORE_NET_BMC_HOST_PASS"`
}

// NewNetConfig returns instance of NetworkConfig config type.
func NewNetConfig() *NetworkConfig {
	log.Print("Creating new NetworkConfig struct")

	var netConf NetworkConfig
	netConf.CoreConfig = coreconfig.NewCoreConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultCnfCoreNetParamsFile)
	err := readFile(&netConf, confFile)

	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&netConf)

	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &netConf
}

// GetMetalLbVirIP IPv4 checks the metalLbIP environmental variable and returns the list of give ip addresses.
func (netConfig *NetworkConfig) GetMetalLbVirIP() ([]string, error) {
	envValue := strings.Split(netConfig.MlbAddressPoolIP, ",")

	if len(envValue) < 2 {
		return nil, fmt.Errorf(
			"the number of virtial metalLb ip address is less than 2, check ECO_CNF_CORE_NET_MLB_ADDR_LIST env var")
	}

	for _, v := range envValue {
		if net.ParseIP(v) == nil {
			return nil, fmt.Errorf("the environment IP variable is not a valid IP")
		}
	}

	return envValue, nil
}

// GetSriovInterfaces checks the ECO_CNF_CORE_NET_SRIOV_INTERFACE_LIST env var
// and returns required number of SR-IOV interfaces.
func (netConfig *NetworkConfig) GetSriovInterfaces(requestedNumber int) ([]string, error) {
	requestedInterfaceList := strings.Split(netConfig.SriovInterfaces, ",")
	if len(requestedInterfaceList) < requestedNumber {
		return nil, fmt.Errorf(
			"the number of SR-IOV interfaces is less than %d,"+
				" check ECO_CNF_CORE_NET_SRIOV_INTERFACE_LIST env var", requestedNumber)
	}

	return requestedInterfaceList, nil
}

// GetVLAN reads environment variable ECO_CNF_CORE_NET_VLAN and returns preconfigured vlanID.
func (netConfig *NetworkConfig) GetVLAN() (uint16, error) {
	if netConfig.VLAN == "" {
		return 0, fmt.Errorf("VLAN is empty. Please check ECO_CNF_CORE_NET_VLAN env var")
	}

	vlanInt, err := strconv.Atoi(netConfig.VLAN)

	if err != nil {
		return 0, err
	}

	return uint16(vlanInt), nil
}

// GetSwitchUser checks the environmental variable ECO_CNF_CORE_NET_SWITCH_USER and returns the value in string.
func (netConfig *NetworkConfig) GetSwitchUser() (string, error) {
	if netConfig.SwitchUser == "" {
		return "", fmt.Errorf("the username for a switch is empty, check ECO_CNF_CORE_NET_SWITCH_USER env var")
	}

	return netConfig.SwitchUser, nil
}

// GetSwitchPass checks the environmental variable ECO_CNF_CORE_NET_SWITCH_PASS and returns the value in string.
func (netConfig *NetworkConfig) GetSwitchPass() (string, error) {
	if netConfig.SwitchPass == "" {
		return "", fmt.Errorf("the password for a switch is empty, check ECO_CNF_CORE_NET_SWITCH_PASS env var")
	}

	return netConfig.SwitchPass, nil
}

// GetSwitchIP checks the environmental variable ECO_CNF_CORE_NET_SWITCH_IP and returns the value in string.
func (netConfig *NetworkConfig) GetSwitchIP() (string, error) {
	if net.ParseIP(netConfig.SwitchIP) == nil {
		return "", fmt.Errorf("the environment switch IP variable is not a valid IP," +
			" check ECO_CNF_CORE_NET_SWITCH_IP env var")
	}

	return netConfig.SwitchIP, nil
}

// GetSwitchInterfaces checks the environmental variable ECO_CNF_CORE_NET_SWITCH_INTERFACES
// and returns the value in []string.
func (netConfig *NetworkConfig) GetSwitchInterfaces() ([]string, error) {
	envValue := strings.Split(netConfig.SwitchInterfaces, ",")

	if len(envValue) != 4 {
		return nil, fmt.Errorf("the number of the switch interfaces is not equal to 4," +
			" check ECO_CNF_CORE_NET_SWITCH_INTERFACES env var")
	}

	return envValue, nil
}

// GetPrimarySwitchInterfaces checks the environmental variable ECO_CNF_CORE_NET_PRIMARY_SWITCH_INTERFACES
// and returns the value in []string.
func (netConfig *NetworkConfig) GetPrimarySwitchInterfaces() ([]string, error) {
	envValue := strings.Split(netConfig.PrimarySwitchInterfaces, ",")

	if len(envValue) != 4 {
		return nil, fmt.Errorf("the number of the switch interfaces is not equal to 4," +
			" check ECO_CNF_CORE_NET_PRIMARY_SWITCH_INTERFACES env var")
	}

	return envValue, nil
}

// GetSwitchLagNames checks the environmental variable ECO_CNF_CORE_NET_SWITCH_LAGS
// and returns the value in []string.
func (netConfig *NetworkConfig) GetSwitchLagNames() ([]string, error) {
	envValue := strings.Split(netConfig.SwitchLagNames, ",")

	if len(envValue) != 2 {
		return nil, fmt.Errorf("the number of the switch lag names is not equal to 2," +
			" check ECO_CNF_CORE_NET_SWITCH_LAGS env var")
	}

	return envValue, nil
}

// GetBMCHostNames checks the environmental variable ECO_CNF_CORE_NET_BMC_HOST_NAMES
// and returns the value in []string.
func (netConfig *NetworkConfig) GetBMCHostNames() ([]string, error) {
	envValue := strings.Split(netConfig.BMCHostNames, ",")

	if len(envValue) == 0 || envValue[0] == "" {
		return nil, fmt.Errorf("the number of BMC host names is less than 1," +
			" ECO_CNF_CORE_NET_BMC_HOST_NAMES env var")
	}

	return envValue, nil
}

// GetBMCHostUser checks the environmental variable ECO_CNF_CORE_NET_BMC_HOST_USER.
func (netConfig *NetworkConfig) GetBMCHostUser() (string, error) {
	if netConfig.BMCHostUser == "" {
		return "", fmt.Errorf("the user name for a BMC is empty, check ECO_CNF_CORE_NET_BMC_HOST_USER env var")
	}

	return netConfig.BMCHostUser, nil
}

// GetBMCHostPass checks the environmental variable ECO_CNF_CORE_NET_BMC_HOST_PASS.
func (netConfig *NetworkConfig) GetBMCHostPass() (string, error) {
	if netConfig.BMCHostPass == "" {
		return "", fmt.Errorf("the password for a BMC is empty, check ECO_CNF_CORE_NET_BMC_HOST_PASS env var")
	}

	return netConfig.BMCHostPass, nil
}

// GetClusterVlan checks the environmental variable ECO_CNF_CORE_NET_CLUSTER_VLAN
// and returns the value in string.
func (netConfig *NetworkConfig) GetClusterVlan() (string, error) {
	if netConfig.ClusterVlan == "" {
		return "", fmt.Errorf("the cluster vlan is empty, check ECO_CNF_CORE_NET_CLUSTER_VLAN env var")
	}

	return netConfig.ClusterVlan, nil
}

func readFile(netConfig *NetworkConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)
	err = decoder.Decode(&netConfig)

	if err != nil {
		return err
	}

	return nil
}

func readEnv(netConfig *NetworkConfig) error {
	err := envconfig.Process("", netConfig)
	if err != nil {
		return err
	}

	return nil
}
