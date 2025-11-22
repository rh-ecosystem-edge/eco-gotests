package spkconfig

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
	// PathToDefaultSPKParamsFile path to config file with default SPK parameters.
	PathToDefaultSPKParamsFile = "./default.yaml"
)

// SPKConfig type keeps SPK configuration.
type SPKConfig struct {
	*systemtestsconfig.SystemTestsConfig
	Namespace         string `yaml:"spk_workload_ns" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_NS"`
	IngressTCPIPv4URL string `yaml:"spk_ingress_tcp_ipv4_url" envconfig:"ECO_SYSTEM_SPK_INGRESS_TCP_IPV4_URL"`
	IngressUDPIPv4URL string `yaml:"spk_ingress_udp_ipv4_url" envconfig:"ECO_SYSTEM_SPK_INGRESS_UDP_IPV4_URL"`
	IngressTCPIPv6URL string `yaml:"spk_ingress_tcp_ipv6_url" envconfig:"ECO_SYSTEM_SPK_INGRESS_TCP_IPV6_URL"`
	IngressUDPIPv6URL string `yaml:"spk_ingress_udp_ipv6_url" envconfig:"ECO_SYSTEM_SPK_INGRESS_UDP_IPV6_URL"`
	//nolint:lll
	WorkloadDCIDeploymentName string      `yaml:"spk_dci_workload_deployment_name" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_DCI_DEPLOYEMNT_NAME"`
	NodesCredentialsMap       NodesBMCMap `yaml:"spk_nodes_bmc_map" envconfig:"ECO_SYSTEM_SPK_NODES_CREDENTIALS_MAP"`
	//nolint:lll
	WorkloadContainerImage string `yaml:"spk_workload_deployment_image" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_DEPLOYMENT_IMAGE"`
	BackendContainerImage  string `yaml:"spk_backend_deployment_image" envconfig:"ECO_SYSTEM_SPK_BACKEND_DEPLOYMENT_IMAGE"`
	WorkloadDeploymentName string `yaml:"spk_workload_deployment_name" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_DEPLOYMENT_NAME"`
	WorkloadTestURL        string `yaml:"spk_workload_test_url" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_TEST_URL"`
	WorkloadTestPort       string `yaml:"spk_workload_test_port" envconfig:"ECO_SYSTEM_SPK_WORKLOAD_TEST_PORT"`
	SPKDataNS              string `yaml:"spk_data_ns" envconfig:"ECO_SYSTEM_SPK_DATA_NS"`
	SPKDnsNS               string `yaml:"spk_dns_ns" envconfig:"ECO_SYSTEM_SPK_DNS_NS"`
	SPKUtilitiesNS         string `yaml:"spk_utilities_ns" envconfig:"ECO_SYSTEM_SPK_UTILITIES_NS"`
	SPKCoreDNSNS           string `yaml:"spk_coredns_ns" envconfig:"ECO_SYSTEM_COREDNS_NS"`
	SPKDataTMMDeployName   string `yaml:"spk_data_tmm_deploy_name" envconfig:"ECO_SYSTEM_SPK_DATA_TMM_DEPLOY_NAME"`
	SPKDnsTMMDeployName    string `yaml:"spk_dns_tmm_deploy_name" envconfig:"ECO_SYSTEM_SPK_DNS_TMM_DEPLOY_NAME"`
	//nolint:lll
	SPKDataIngressDeployName string `yaml:"spk_data_ingress_deploy_name" envconfig:"ECO_SYSTEM_SPK_DATA_INGRESS_DEPLOY_NAME"`
	SPKDnsIngressDeployName  string `yaml:"spk_dns_ingress_deploy_name" envconfig:"ECO_SYSTEM_SPK_DNS_INGRESS_DEPLOY_NAME"`
	//nolint:lll
	BackendUDPContainerImage string `yaml:"spk_backend_udp_deployment_image" envconfig:"ECO_SYSTEM_SPK_BACKEND_UDP_DEPLOYMENT_IMAGE"`
}

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

	for _, record := range strings.Split(value, ";") {
		log.Printf("Processing: %v", record)

		parsedRecord := strings.Split(record, ",")
		if len(parsedRecord) != 4 {
			log.Printf("Error to parse data %v", value)
			log.Printf("Expected 4 entries, found %d", len(parsedRecord))

			return fmt.Errorf("error parsing data %v", value)
		}

		nodesAuthMap[parsedRecord[0]] = BMCDetails{
			Username:   parsedRecord[1],
			Password:   parsedRecord[2],
			BMCAddress: parsedRecord[3],
		}
	}

	*nad = nodesAuthMap

	return nil
}

// NewSPKConfig returns instance of SPKConfig config type.
func NewSPKConfig() *SPKConfig {
	log.Print("Creating new SPKConfig struct")

	var spkConf SPKConfig

	spkConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	var confFile string

	if fileFromEnv, exists := os.LookupEnv("ECO_SYSTEM_SPK_CONFIG_FILE_PATH"); !exists {
		_, filename, _, _ := runtime.Caller(0)
		baseDir := filepath.Dir(filename)
		confFile = filepath.Join(baseDir, PathToDefaultSPKParamsFile)
	} else {
		confFile = fileFromEnv
	}

	log.Printf("Open config file %s", confFile)

	err := readFile(&spkConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&spkConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &spkConf
}

func readFile(spkConfig *SPKConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&spkConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(spkConfig *SPKConfig) error {
	err := envconfig.Process("", spkConfig)
	if err != nil {
		return err
	}

	return nil
}
