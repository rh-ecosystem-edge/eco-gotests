package rdscoreconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"

	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultRDSCoreParamsFile path to config file with default RDSCore parameters.
	PathToDefaultRDSCoreParamsFile = "./default.yaml"
)

// BMCDetails structure to hold BMC details.
type BMCDetails struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	BMCAddress string `json:"bmc"`
}

// TolerationList used to store tolerations for test workloads.
type TolerationList []corev1.Toleration

// Decode - method for envconfig package to parse environment variable.
func (tl *TolerationList) Decode(value string) error {
	tmpTolerationList := []corev1.Toleration{}

	for _, record := range strings.Split(value, ";") {
		log.Printf("Processing toleration record: %q", record)

		parsedToleration := corev1.Toleration{}

		for _, parsedRecord := range strings.Split(record, ",") {
			switch strings.Split(parsedRecord, "=")[0] {
			case "key":
				parsedToleration.Key = strings.Split(parsedRecord, "=")[1]
			case "value":
				parsedToleration.Value = strings.Split(parsedRecord, "=")[1]
			case "effect":
				parsedToleration.Effect = corev1.TaintEffect(strings.Split(parsedRecord, "=")[1])
			case "operator":
				parsedToleration.Operator = corev1.TolerationOperator(strings.Split(parsedRecord, "=")[1])
			}
		}
		tmpTolerationList = append(tmpTolerationList, parsedToleration)
	}

	*tl = tmpTolerationList

	return nil
}

// EnvMapString holds a map[string]string parsed from environment variable.
type EnvMapString map[string]string

// Decode - method for envconfig package to parse environment variable.
func (ems *EnvMapString) Decode(value string) error {
	resultMap := make(map[string]string)

	for _, record := range strings.Split(value, ";;") {
		log.Printf("Processing record: %q", record)

		key := strings.Split(record, "===")[0]
		val := strings.Split(record, "===")[1]

		multiLine := ""

		if strings.Contains(val, `\n`) {
			for _, line := range strings.Split(val, `\n`) {
				multiLine += fmt.Sprintf("\n%s", line)
			}
		} else {
			multiLine = val
		}

		resultMap[key] = multiLine
	}

	*ems = resultMap

	return nil
}

// EnvSliceString holds a []string parsed from environment variable.
type EnvSliceString []string

// Decode - method for envconfig package to parse environment variable,
// as a separator triple pipe '|||' is used.
func (ess *EnvSliceString) Decode(value string) error {
	resultSlice := []string{}

	log.Printf("EnvSliceString: Processing record: %q", value)

	resultSlice = append(resultSlice, strings.Split(value, "|||")...)

	*ess = resultSlice

	return nil
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

// CoreConfig type keeps RDS Core configuration.
type CoreConfig struct {
	*config.GeneralConfig
	PolicyNS             string `yaml:"rdscore_policy_ns" envconfig:"ECO_RDSCORE_POLICY_NS"`
	WlkdSRIOVOneNS       string `yaml:"rdscore_wlkd_sriov_one_ns" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_NS"`
	WlkdSRIOVTwoNS       string `yaml:"rdscore_wlkd_sriov_two_ns" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_NS"`
	WlkdSRIOV3NS         string `yaml:"rdscore_wlkd_sriov_3_ns" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_NS"`
	WlkdSRIOV4NS         string `yaml:"rdscore_wlkd_sriov_4_ns" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_NS"`
	WlkdNROPOneNS        string `yaml:"rdscore_wlkd_nrop_one_ns" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_NS"`
	WlkdNROPTwoNS        string `yaml:"rdscore_wlkd_nrop_two_ns" envconfig:"ECO_RDSCORE_WLKD_NROP_TWO_NS"`
	MCVlanNSOne          string `yaml:"rdscore_mcvlan_ns_one" envconfig:"ECO_RDSCORE_MCVLAN_NS_ONE"`
	MCVlanNSTwo          string `yaml:"rdscore_mcvlan_ns_two" envconfig:"ECO_RDSCORE_MCVLAN_NS_TWO"`
	MCVlanDeployImageOne string `yaml:"rdscore_mcvlan_deploy_img_one" envconfig:"ECO_SYSTEM_RDSCORE_DEPLOY_IMG_ONE"`
	MCVlanNADOneName     string `yaml:"rdscore_mcvlan_nad_one_name" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_NAD_ONE_NAME"`
	KDumpCPNodeLabel     string `yaml:"rdscore_kdump_cp_node_label" envconfig:"ECO_RDSCORE_KDUMP_CP_NODE_LABEL"`
	KDumpCNFMCPNodeLabel string `yaml:"rdscore_kdump_cnf_node_label" envconfig:"ECO_RDSCORE_KDUMP_CNF_NODE_LABEL"`
	IPVlanNSOne          string `yaml:"rdscore_ipvlan_ns_one" envconfig:"ECO_RDSCORE_IPVLAN_NS_ONE"`
	IPVlanNSTwo          string `yaml:"rdscore_ipvlan_ns_two" envconfig:"ECO_RDSCORE_IPVLAN_NS_TWO"`
	IPVlanDeployImageOne string `yaml:"rdscore_ipvlan_deploy_img_one" envconfig:"ECO_SYSTEM_RDSCORE_DEPLOY_IMG_ONE"`
	IPVlanNADOneName     string `yaml:"rdscore_ipvlan_nad_one_name" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_NAD_ONE_NAME"`
	IPVlanNADTwoName     string `yaml:"rdscore_ipvlan_nad_two_name" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_NAD_TWO_NAME"`
	IPVlanNADThreeName   string `yaml:"rdscore_ipvlan_nad_three_name" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_NAD_THREE_NAME"`
	IPVlanNADFourName    string `yaml:"rdscore_ipvlan_nad_four_name" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_NAD_FOUR_NAME"`
	//nolint:lll
	PerformanceProfileHTName string `yaml:"rdscore_performance_profile_ht_name" envconfig:"ECO_RDS_CORE_PERFORMANCE_PROFILE_HT_NAME"`
	//nolint:lll
	KDumpWorkerMCPNodeLabel string         `yaml:"rdscore_kdump_worker_node_label" envconfig:"ECO_RDSCORE_KDUMP_WORKER_NODE_LABEL"`
	WlkdTolerationList      TolerationList `yaml:"rdscore_tolerations_list" envconfig:"ECO_RDSCORE_TOLERATIONS_LIST"`
	//nolint:lll
	WlkdNROPTolerationList TolerationList `yaml:"rdscore_nrop_tolerations_list" envconfig:"ECO_RDSCORE_NROP_TOLERATIONS_LIST"`
	//nolint:lll,nolintlint
	MCVlanCMDataOne map[string]string `yaml:"rdscore_mcvlan_cm_data_one" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_CM_DATA_ONE"`
	//nolint:lll,nolintlint
	IPVlanCMDataOne map[string]string `yaml:"rdscore_ipvlan_cm_data_one" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_CM_DATA_ONE"`
	//nolint:lll
	StorageODFWorkloadImage string      `yaml:"rdscore_storage_storage_wlkd_image" envconfig:"ECO_RDSCORE_STORAGE_WLKD_IMAGE"`
	NodesCredentialsMap     NodesBMCMap `yaml:"rdscore_nodes_bmc_map" envconfig:"ECO_RDSCORE_NODES_CREDENTIALS_MAP"`
	WlkdSRIOVDeployOneImage string      `yaml:"rdscore_wlkd_sriov_one_image" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_IMG"`
	WlkdSRIOVDeployTwoImage string      `yaml:"rdscore_wlkd_sriov_two_image" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_IMG"`
	WlkdSRIOVDeploy3Image   string      `yaml:"rdscore_wlkd_sriov_3_image" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_IMG"`
	WlkdSRIOVDeploy4Image   string      `yaml:"rdscore_wlkd_sriov_4_image" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_IMG"`
	WlkdNROPDeployOneImage  string      `yaml:"rdscore_wlkd_nrop_one_image" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_IMG"`
	WlkdNROPDeployTwoImage  string      `yaml:"rdscore_wlkd_nrop_two_image" envconfig:"ECO_RDSCORE_WLKD_NROP_TWO_IMG"`
	WlkdSRIOVNetOne         string      `yaml:"rdscore_wlkd_sriov_net_one" envconfig:"ECO_RDSCORE_WLKD_SRIOV_NET_ONE"`
	WlkdSRIOVNetTwo         string      `yaml:"rdscore_wlkd_sriov_net_two" envconfig:"ECO_RDSCORE_WLKD_SRIOV_NET_TWO"`
	WlkdSRIOVTwoSa          string      `yaml:"rdscore_wlkd_sriov_two_sa" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_SA"`
	NROPSchedulerName       string      `yaml:"rdscore_nrop_scheduler_name" envconfig:"ECO_RDSCORE_NROP_SCHEDULER_NAME"`
	//nolint:lll
	MetalLBFRRTestURLIPv4 string `yaml:"rdscore_metallb_frr_test_url_ipv4" envconfig:"ECO_RDSCORE_METALLB_FRR_TEST_URL_IPV4"`
	//nolint:lll
	MetalLBFRRTestURLIPv6 string `yaml:"rdscore_metallb_frr_test_url_ipv6" envconfig:"ECO_RDSCORE_METALLB_FRR_TEST_URL_IPV6"`
	//nolint:lll
	WlkdSRIOVConfigMapDataOne EnvMapString `yaml:"rdscore_wlkd_sriov_cm_data_one" envconfig:"ECO_RDSCORE_SRIOV_CM_DATA_ONE"`
	//nolint:lll
	WlkdSRIOVConfigMapDataTwo EnvMapString `yaml:"rdscore_wlkd_sriov_cm_data_two" envconfig:"ECO_RDSCORE_SRIOV_CM_DATA_TWO"`
	WlkdSRIOVConfigMapData3   EnvMapString `yaml:"rdscore_wlkd_sriov_cm_data_3" envconfig:"ECO_RDSCORE_SRIOV_CM_DATA_3"`
	WlkdSRIOVConfigMapData4   EnvMapString `yaml:"rdscore_wlkd_sriov_cm_data_4" envconfig:"ECO_RDSCORE_SRIOV_CM_DATA_4"`
	//nolint:lll
	StorageODFDeployOneSelector EnvMapString `yaml:"rdscore_wlkd_odf_one_selector" envconfig:"ECO_RDSCORE_WLKD_ODF_ONE_SELECTOR"`
	//nolint:lll
	StorageODFDeployTwoSelector EnvMapString `yaml:"rdscore_wlkd_odf_two_selector" envconfig:"ECO_RDSCORE_WLKD_ODF_TWO_SELECTOR"`
	//nolint:lll
	WlkdNROPDeployOneSelector EnvMapString `yaml:"rdscore_wlkd_nrop_one_selector" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeployOneSelector EnvMapString `yaml:"rdscore_wlkd_sriov_one_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeployTwoSelector EnvMapString `yaml:"rdscore_wlkd_sriov_two_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeploy3OneSelector EnvMapString `yaml:"rdscore_wlkd_sriov_3_0_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_0_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeploy4OneSelector EnvMapString `yaml:"rdscore_wlkd_sriov_4_0_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_0_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeploy4TwoSelector EnvMapString `yaml:"rdscore_wlkd_sriov_4_1_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_1_SELECTOR"`
	//nolint:lll
	WlkdSRIOVDeploy3TwoSelector EnvMapString `yaml:"rdscore_wlkd_sriov_3_1_selector" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_1_SELECTOR"`
	//nolint:lll
	WldkNROPDeployOneResRequests EnvMapString `yaml:"rdscore_wlkd_nrop_one_res_requests" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeployOneResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_one_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeployTwoResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_two_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeploy3OneResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_3_0_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_0_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeploy3TwoResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_3_1_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_1_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeploy4OneResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_4_0_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_0_RES_REQUESTS"`
	//nolint:lll
	WldkSRIOVDeploy4TwoResRequests EnvMapString `yaml:"rdscore_wlkd_sriov_4_1_res_requests" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_1_RES_REQUESTS"`
	//nolint:lll
	WldkNROPDeployOneResLimits EnvMapString `yaml:"rdscore_wlkd_nrop_one_res_limits" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeployOneResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_one_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeployTwoResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_two_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeploy3OneResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_3_0_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_0_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeploy3TwoResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_3_1_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_1_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeploy4OneResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_4_0_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_0_RES_LIMITS"`
	//nolint:lll
	WldkSRIOVDeploy4TwoResLimits EnvMapString `yaml:"rdscore_wlkd_sriov_4_1_res_limits" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_1_RES_LIMITS"`
	//nolint:lll,nolintlint
	NodeSelectorHTNodes EnvMapString `yaml:"rdscore_node_selector_ht_nodes" envconfig:"ECO_RDSCORE_NODE_SELECTOR_HT_NODES"`
	//nolint:lll
	WlkdSRIOVDeployOneTargetAddress string `yaml:"rdscore_wlkd_sriov_deploy_one_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_ONE_TARGET"`
	//nolint:lll
	WlkdSRIOVDeployOneTargetAddressIPv6 string `yaml:"rdscore_wlkd_sriov_deploy_one_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_ONE_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy3OneTargetAddress string `yaml:"rdscore_wlkd3_sriov_deploy_one_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_3_ONE_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy3OneTargetAddressIPv6 string `yaml:"rdscore_wlkd3_sriov_deploy_one_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_3_ONE_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy4OneTargetAddress string `yaml:"rdscore_wlkd4_sriov_deploy_one_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_4_ONE_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy4OneTargetAddressIPv6 string `yaml:"rdscore_wlkd4_sriov_deploy_one_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_4_ONE_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeployTwoTargetAddress string `yaml:"rdscore_wlkd_sriov_deploy_two_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_TWO_TARGET"`
	//nolint:lll
	WlkdSRIOVDeployTwoTargetAddressIPv6 string `yaml:"rdscore_wlkd_sriov_deploy_two_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy3TwoTargetAddress string `yaml:"rdscore_wlkd3_sriov_deploy_two_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_3_DEPLOY_TWO_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy3TwoTargetAddressIPv6 string `yaml:"rdscore_wlkd3_sriov_deploy_two_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_3_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy4TwoTargetAddress string `yaml:"rdscore_wlkd4_sriov_deploy_two_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD_4_DEPLOY_TWO_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy4TwoTargetAddressIPv6 string `yaml:"rdscore_wlkd4_sriov_deploy_two_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD_4_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy2OneTargetAddress string `yaml:"rdscore_wlkd2_sriov_deploy_one_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_ONE_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy2OneTargetAddressIPv6 string `yaml:"rdscore_wlkd2_sriov_deploy_one_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_ONE_TARGET_IPV6"`
	//nolint:lll
	WlkdSRIOVDeploy2TwoTargetAddress string `yaml:"rdscore_wlkd2_sriov_deploy_two_target" envconfig:"ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_TWO_TARGET"`
	//nolint:lll
	WlkdSRIOVDeploy2TwoTargetAddressIPv6 string `yaml:"rdscore_wlkd2_sriov_deploy_two_target_ipv6" envconfig:"ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	MCVlanDeployNodeSelectorOne EnvMapString `yaml:"rdscore_mcvlan_1_node_selector" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_1_NODE_SELECTOR"`
	//nolint:lll
	MCVlanDeployNodeSelectorTwo EnvMapString `yaml:"rdscore_mcvlan_2_node_selector" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_2_NODE_SELECTOR"`
	//nolint:lll
	MCVlanDeploy1TargetAddress string `yaml:"rdscore_macvlan_deploy_1_target" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_ONE_TARGET"`
	//nolint:lll
	MCVlanDeploy1TargetAddressIPv6 string `yaml:"rdscore_macvlan_deploy_1_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_ONE_TARGET_IPV6"`
	//nolint:lll
	MCVlanDeploy2TargetAddress string `yaml:"rdscore_macvlan_deploy_2_target" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_TWO_TARGET"`
	//nolint:lll
	MCVlanDeploy2TargetAddressIPv6 string `yaml:"rdscore_macvlan_deploy_2_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	MCVlanDeploy3TargetAddress string `yaml:"rdscore_macvlan_deploy_3_target" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_3_TARGET"`
	//nolint:lll
	MCVlanDeploy3TargetAddressIPv6 string `yaml:"rdscore_macvlan_deploy_3_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_3_TARGET_IPV6"`
	//nolint:lll
	MCVlanDeploy4TargetAddress string `yaml:"rdscore_macvlan_deploy_4_target" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_4_TARGET"`
	//nolint:lll
	MCVlanDeploy4TargetAddressIPv6 string `yaml:"rdscore_macvlan_deploy_4_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_4_TARGET_IPV6"`
	//nolint:lll
	IPVlanDeployNodeSelectorOne EnvMapString `yaml:"rdscore_ipvlan_1_node_selector" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_1_NODE_SELECTOR"`
	//nolint:lll
	IPVlanDeployNodeSelectorTwo EnvMapString `yaml:"rdscore_ipvlan_2_node_selector" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_2_NODE_SELECTOR"`
	//nolint:lll
	IPVlanDeploy1TargetAddress string `yaml:"rdscore_ipvlan_deploy_1_target" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_ONE_TARGET"`
	//nolint:lll
	IPVlanDeploy1TargetAddressIPv6 string `yaml:"rdscore_ipvlan_deploy_1_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_ONE_TARGET_IPV6"`
	//nolint:lll
	IPVlanDeploy2TargetAddress string `yaml:"rdscore_ipvlan_deploy_2_target" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_TWO_TARGET"`
	//nolint:lll
	IPVlanDeploy2TargetAddressIPv6 string `yaml:"rdscore_ipvlan_deploy_2_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_TWO_TARGET_IPV6"`
	//nolint:lll
	IPVlanDeploy3TargetAddress string `yaml:"rdscore_ipvlan_deploy_3_target" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_TARGET"`
	//nolint:lll
	IPVlanDeploy3TargetAddressIPv6 string `yaml:"rdscore_ipvlan_deploy_3_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_TARGET_IPV6"`
	//nolint:lll
	IPVlanDeploy4TargetAddress string `yaml:"rdscore_ipvlan_deploy_4_target" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_TARGET"`
	//nolint:lll
	IPVlanDeploy4TargetAddressIPv6 string `yaml:"rdscore_ipvlan_deploy_4_target_ipv6" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_TARGET_IPV6"`
	//nolint:lll,nolintlint
	WlkdNROPDeployOneCmd EnvSliceString `yaml:"rdscore_wlkd_nrop_one_cmd" envconfig:"ECO_RDSCORE_WLKD_NROP_ONE_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeployOneCmd EnvSliceString `yaml:"rdscore_wlkd_sriov_one_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_ONE_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeployTwoCmd EnvSliceString `yaml:"rdscore_wlkd_sriov_two_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_TWO_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy2OneCmd EnvSliceString `yaml:"rdscore_wlkd2_sriov_one_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_2_ONE_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy2TwoCmd EnvSliceString `yaml:"rdscore_wlkd2_sriov_two_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_2_TWO_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy3OneCmd EnvSliceString `yaml:"rdscore_wlkd3_sriov_one_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_ONE_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy3TwoCmd EnvSliceString `yaml:"rdscore_wlkd3_sriov_two_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_3_TWO_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy4OneCmd EnvSliceString `yaml:"rdscore_wlkd4_sriov_one_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_ONE_CMD"`
	//nolint:lll,nolintlint
	WlkdSRIOVDeploy4TwoCmd EnvSliceString `yaml:"rdscore_wlkd4_sriov_two_cmd" envconfig:"ECO_RDSCORE_WLKD_SRIOV_4_TWO_CMD"`
	//nolint:lll,nolintlint
	MCVlanDeplonOneCMD EnvSliceString `yaml:"rdscore_mcvlan_deploy_1_cmd" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_1_CMD"`
	//nolint:lll,nolintlint
	MCVlanDeplonTwoCMD EnvSliceString `yaml:"rdscore_mcvlan_deploy_2_cmd" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_2_CMD"`
	//nolint:lll,nolintlint
	MCVlanDeplon3CMD EnvSliceString `yaml:"rdscore_mcvlan_deploy_3_cmd" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_3_CMD"`
	//nolint:lll,nolintlint
	MCVlanDeplon4CMD EnvSliceString `yaml:"rdscore_mcvlan_deploy_4_cmd" envconfig:"ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_4_CMD"`
	//nolint:lll,nolintlint
	IPVlanDeplonOneCMD EnvSliceString `yaml:"rdscore_ipvlan_deploy_1_cmd" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_1_CMD"`
	//nolint:lll,nolintlint
	IPVlanDeplonTwoCMD EnvSliceString `yaml:"rdscore_ipvlan_deploy_2_cmd" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_2_CMD"`
	//nolint:lll,nolintlint
	IPVlanDeplon3CMD EnvSliceString `yaml:"rdscore_ipvlan_deploy_3_cmd" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_CMD"`
	//nolint:lll,nolintlint
	IPVlanDeplon4CMD     EnvSliceString `yaml:"rdscore_ipvlan_deploy_4_cmd" envconfig:"ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_CMD"`
	StorageCephFSSCName  string         `yaml:"rdscore_sc_cephfs_name" envconfig:"ECO_RDSCORE_SC_CEPHFS_NAME"`
	StorageCephRBDSCName string         `yaml:"rdscore_sc_cephrbd_name" envconfig:"ECO_RDSCORE_SC_CEPHRBD_NAME"`
	EgressServiceNS      string         `yaml:"rdscore_egress_service_ns" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_NS"`
	//nolint:lll,nolintlint
	EgressServiceRemoteIP string `yaml:"rdscore_egress_service_remote_ip" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_REMOTE_IP"`
	//nolint:lll,nolintlint
	EgressServiceRemoteIPv6 string `yaml:"rdscore_egress_service_remote_ipv6" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_REMOTE_IPV6"`
	//nolint:lll,nolintlint
	EgressServiceRemotePort string `yaml:"rdscore_egress_service_remote_port" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_REMOTE_PORT"`
	//nolint:lll,nolintlint
	EgressServiceDeploy1CMD EnvSliceString `yaml:"rdscore_egress_service_deploy_1_cmd" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_1_CMD"`
	//nolint:lll,nolintlint
	EgressServiceDeploy1Image string `yaml:"rdscore_egress_service_deploy_1_img" envconfig:"ECO_RDSCORE_EGRESS_SVC_DEPLOY_1_IMG"`
	EgressServiceVRF1Network  string `yaml:"rdscore_egress_service_vrf_1_net" envconfig:"ECO_RDSCORE_EGRESS_SVC_VRF_1_NET"`
	//nolint:lll,nolintlint
	EgressServiceDeploy1IPAddrPool string `yaml:"rdscore_egress_service_deploy_1_ipaddr_pool" envconfig:"ECO_RDSCORE_EGRESS_SVC_DEPLOY_1_IPADDR_POOL"`
	//nolint:lll,nolintlint
	EgressServiceDeploy1NodeSelector EnvMapString `yaml:"rdscore_egress_service_1_node_selector" envconfig:"ECO_RDSCORE_EGRESS_SVC_1_NODE_SELECTOR"`
	//nolint:lll,nolintlint
	EgressServiceDeploy2CMD EnvSliceString `yaml:"rdscore_egress_service_deploy_2_cmd" envconfig:"ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_2_CMD"`
	//nolint:lll,nolintlint
	EgressServiceDeploy2Image string `yaml:"rdscore_egress_service_deploy_2_img" envconfig:"ECO_RDSCORE_EGRESS_SVC_DEPLOY_2_IMG"`
	EgressServiceVRF2Network  string `yaml:"rdscore_egress_service_vrf_2_net" envconfig:"ECO_RDSCORE_EGRESS_SVC_VRF_2_NET"`
	//nolint:lll,nolintlint
	EgressServiceDeploy2NodeSelector EnvMapString `yaml:"rdscore_egress_service_2_node_selector" envconfig:"ECO_RDSCORE_EGRESS_SVC_2_NODE_SELECTOR"`
	//nolint:lll,nolintlint
	EgressServiceDeploy2IPAddrPool string `yaml:"rdscore_egress_service_deploy_2_ipaddr_pool" envconfig:"ECO_RDSCORE_EGRESS_SVC_DEPLOY_2_IPADDR_POOL"`
	EgressIPName                   string `yaml:"rdscore_egressip_name" envconfig:"ECO_RDSCORE_EGRESSIP_NAME"`
	//nolint:lll,nolintlint
	EgressIPDeploymentImage string `yaml:"rdscore_wlkd_egressip_image" envconfig:"ECO_RDSCORE_EGRESSIP_DEPLOY_IMG"`
	//nolint:lll,nolintlint
	EgressIPNodeOne string `yaml:"rdscore_wlkd_egressip_node_one" envconfig:"ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_ONE"`
	//nolint:lll,nolintlint
	EgressIPNodeTwo string `yaml:"rdscore_wlkd_egressip_node_two" envconfig:"ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_TWO"`
	//nolint:lll,nolintlint
	EgressIPNodeThree string `yaml:"rdscore_wlkd_egressip_node_three" envconfig:"ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_THREE"`
	//nolint:lll,nolintlint
	NonEgressIPNode string `yaml:"rdscore_wlkd_non_egressip_node" envconfig:"ECO_SYSTEM_RDSCORE_NON_EGRESSIP_NODE"`
	//nolint:lll,nolintlint
	EgressIPNamespaceLabel string `yaml:"rdscore_egressip_ns_label" envconfig:"ECO_RDSCORE_EGRESSIP_NS_LABEL"`
	//nolint:lll,nolintlint
	EgressIPPodLabel string `yaml:"rdscore_egressip_pod_label" envconfig:"ECO_RDSCORE_EGRESSIP_POD_LABEL"`
	//nolint:lll,nolintlint
	EgressIPNamespaceOne string `yaml:"rdscore_egressip_ns_one" envconfig:"ECO_RDSCORE_EGRESSIP_NS_ONE"`
	//nolint:lll,nolintlint
	EgressIPNamespaceTwo string `yaml:"rdscore_egressip_ns_two" envconfig:"ECO_RDSCORE_EGRESSIP_NS_TWO"`
	//nolint:lll,nolintlint
	EgressIPv4 string `yaml:"rdscore_egressip_ipv4" envconfig:"ECO_RDSCORE_EGRESSIP_IPV4"`
	//nolint:lll,nolintlint
	EgressIPv6 string `yaml:"rdscore_egressip_ipv6" envconfig:"ECO_RDSCORE_EGRESSIP_IPV6"`
	//nolint:lll,nolintlint
	EgressIPRemoteIPv4 string `yaml:"rdscore_egressip_remote_ipv4" envconfig:"ECO_RDSCORE_EGRESSIP_REMOTE_IPV4"`
	//nolint:lll,nolintlint
	EgressIPRemoteIPv6 string `yaml:"rdscore_egressip_remote_ipv6" envconfig:"ECO_RDSCORE_EGRESSIP_REMOTE_IPV6"`
	//nolint:lll,nolintlint
	EgressIPTcpPort string `yaml:"rdscore_egressip_tcp_port_number" envconfig:"ECO_RDSCORE_EGRESSIP_TCP_PORT_NUMBER"`
}

// NewCoreConfig returns instance of CoreConfig config type.
func NewCoreConfig() *CoreConfig {
	log.Print("Creating new CoreConfig struct")

	var rdsCoreConf CoreConfig
	rdsCoreConf.GeneralConfig = config.NewConfig()

	var confFile string

	if fileFromEnv, exists := os.LookupEnv("ECO_RDS_CORE_CONFIG_FILE_PATH"); !exists {
		_, filename, _, _ := runtime.Caller(0)
		baseDir := filepath.Dir(filename)
		confFile = filepath.Join(baseDir, PathToDefaultRDSCoreParamsFile)
	} else {
		confFile = fileFromEnv
	}

	log.Printf("Open config file %s", confFile)

	err := readFile(&rdsCoreConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&rdsCoreConf)

	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &rdsCoreConf
}

func readFile(rdsConfig *CoreConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)
	err = decoder.Decode(&rdsConfig)

	if err != nil {
		return err
	}

	return nil
}

func readEnv(rdsConfig *CoreConfig) error {
	err := envconfig.Process("", rdsConfig)
	if err != nil {
		return err
	}

	return nil
}
