package ipsecconfig

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
	// PathToDefaultIpsecParamsFile path to config file with default ipsec parameters.
	PathToDefaultIpsecParamsFile = "./default.yaml"
)

// IpsecConfig type keeps ipsec configuration.
type IpsecConfig struct {
	*systemtestsconfig.SystemTestsConfig
	Iperf3ToolImage string `yaml:"iperf3tool_image" envconfig:"ECO_IPSEC_TESTS_IPERF3_IMAGE"`
	TestWorkload    struct {
		Namespace      string `yaml:"namespace" envconfig:"ECO_IPSEC_TESTWORKLOAD_NAMESPACE"`
		CreateMethod   string `yaml:"create_method" envconfig:"ECO_IPSEC_TESTWORKLOAD_CREATE_METHOD"`
		CreateShellCmd string `yaml:"create_shell_cmd" envconfig:"ECO_IPSEC_TESTWORKLOAD_CREATE_SHELLCMD"`
		DeleteShellCmd string `yaml:"delete_shell_cmd" envconfig:"ECO_IPSEC_TESTWORKLOAD_DELETE_SHELLCMD"`
	} `yaml:"ipsec_test_workload"`
	// This is the Host IP for ssh
	SecGwHostIP string `yaml:"secgw_host_ip" envconfig:"ECO_IPSEC_SECGW_HOST_IP"`
	// This is the SecGW IPSec tunnel IP
	SecGwServerIP       string `yaml:"secgw_server_ip" envconfig:"ECO_IPSEC_SECGW_SERVER_IP"`
	Iperf3ServerOcpIPs  string `yaml:"iperf3_server_ocp_ips" envconfig:"ECO_IPSEC_IPERF3_SERVER_OCP_IPS"`
	Iperf3ClientTxBytes string `yaml:"iperf3_client_tx_bytes" envconfig:"ECO_IPSEC_IPERF3_CLIENT_TX_BYTES"`
	NodePort            string `yaml:"node_port" envconfig:"ECO_IPSEC_NODE_PORT"`
	NodePortIncrement   string `yaml:"node_port_increment" envconfig:"ECO_IPSEC_NODE_PORT_INCREMENT"`
	SSHUser             string `yaml:"ssh_user" envconfig:"ECO_SSH_USER"`
	SSHPrivateKey       string `yaml:"ssh_private_key" envconfig:"ECO_SSH_PRIVATE_KEY"`
	SSHPort             string `yaml:"ssh_port" envconfig:"ECO_SSH_PORT"`
}

// NewIpsecConfig returns instance of IpsecConfig config type.
func NewIpsecConfig() *IpsecConfig {
	log.Print("Creating new IpsecConfig struct")

	var ipsecConf IpsecConfig

	ipsecConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultIpsecParamsFile)

	err := readFile(&ipsecConf, confFile)
	if err != nil {
		log.Printf("Error reading config file %s", confFile)

		return nil
	}

	err = readEnv(&ipsecConf)
	if err != nil {
		log.Print("Error reading environment variables")

		return nil
	}

	return &ipsecConf
}

func readFile(ipsecConfig *IpsecConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&ipsecConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(ipsecConfig *IpsecConfig) error {
	err := envconfig.Process("", ipsecConfig)
	if err != nil {
		return err
	}

	return nil
}
