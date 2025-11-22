package ocloudconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultOCloudParamsFile path to config file with o-cloud parameters.
	PathToDefaultOCloudParamsFile = "./default.yaml"
)

// OCloudConfig type keeps o-cloud configuration.
type OCloudConfig struct {
	*systemtestsconfig.SystemTestsConfig

	// GenerateSeedImage true to generate the seed image
	GenerateSeedImage bool `yaml:"ibi_generate_seed_image" envconfig:"ECO_OCLOUD_IBI_GENERATE_SEED_IMAGE"`
	// IbiBaseImagePath base image path
	IbiBaseImagePath string `yaml:"ibi_base_image_path" envconfig:"ECO_OCLOUD_IBI_BASE_IMAGE_PATH"`
	// IbiBaseImageURL base image URL
	IbiBaseImageURL string `yaml:"ibi_base_image_url" envconfig:"ECO_OCLOUD_IBI_BASE_IMAGE_URL"`
	// VirtualMediaID virtual media ID
	VirtualMediaID string `yaml:"virtual_media_id" envconfig:"ECO_OCLOUD_VIRTUAL_MEDIA_ID"`

	// LocalRegistryAuth local registry auth information
	LocalRegistryAuth string `yaml:"local_registry_auth" envconfig:"ECO_OCLOUD_LOCAL_REGISTRY_AUTH"`
	// SeedImage seed image
	SeedImage string `yaml:"seed_image" envconfig:"ECO_OCLOUD_SEED_IMAGE"`
	// SeedVersion seed version
	SeedVersion string `yaml:"seed_version" envconfig:"ECO_OCLOUD_SEED_VERSION"`
	// Registry5000 url for registry in port 500
	Registry5000 string `yaml:"registry_5000" envconfig:"ECO_OCLOUD_REGISTRY_5000"`
	// Registry5005 url for registry in port 5005
	Registry5005 string `yaml:"registry_5005" envconfig:"ECO_OCLOUD_REGISTRY_5005"`
	// SSHKey ssh key
	SSHKey string `yaml:"ssh_key" envconfig:"ECO_OCLOUD_SSH_KEY"`
	// PullSecret pull secret
	PullSecret string `yaml:"pull_secret" envconfig:"ECO_OCLOUD_PULL_SECRET"`
	// BaseImageName base image name
	BaseImageName string `yaml:"base_image_name" envconfig:"ECO_OCLOUD_BASE_IMAGE_NAME"`
	// InterfaceName interface name
	InterfaceName string `yaml:"interface_name" envconfig:"ECO_OCLOUD_INTERFACE_NAME"`
	// InterfaceIpv6_1 IPv6 address of the interface for the first cluster
	InterfaceIpv6_1 string `yaml:"interface_ipv6_1" envconfig:"ECO_OCLOUD_INTERFACE_IPV6_1"`
	// InterfaceIpv6_2 IPv6 address of the interface for the second cluster
	InterfaceIpv6_2 string `yaml:"interface_ipv6_2" envconfig:"ECO_OCLOUD_INTERFACE_IPV6_2"`
	// DNSIpv6 IPv6 address of the DNS server
	DNSIpv6 string `yaml:"dns_ipv6" envconfig:"ECO_OCLOUD_DNS_IPV6"`
	// NextHopIpv6 IPv6 address of the next hop
	NextHopIpv6 string `yaml:"next_hop_ipv6" envconfig:"ECO_OCLOUD_NEXT_HOP_IPV6"`
	// NextHopInterface interface of the next hop
	NextHopInterface string `yaml:"next_hop_interface" envconfig:"ECO_OCLOUD_NEXT_HOP_INTERFACE"`
	// Spoke1BMC BMC configuration for spoke 1
	Spoke1BMC *bmc.BMC
	// Spoke1BMCUsername BMC username for spoke 1
	Spoke1BMCUsername string `yaml:"spoke1_bmc_username" envconfig:"ECO_OCLOUD_SPOKE1_BMC_USERNAME"`
	// Spoke1BMCPassword BMC password for spoke 1
	Spoke1BMCPassword string `yaml:"spoke1_bmc_password" envconfig:"ECO_OCLOUD_SPOKE1_BMC_PASSWORD"`
	// Spoke1BMCHost BMC IP address for spoke 1
	Spoke1BMCHost string `yaml:"spoke1_bmc_host" envconfig:"ECO_OCLOUD_SPOKE1_BMC_HOST"`
	// Spoke1BMCTimeout timeout for BMC for spoke 1
	Spoke1BMCTimeout time.Duration `yaml:"spoke1_bmc_timeout" envconfig:"ECO_OCLOUD_SPOKE1_BMC_TIMEOUT"`

	// Spoke2BMC BMC configuration for spoke 2
	Spoke2BMC *bmc.BMC
	// Spoke2BMCUsername BMC username for spoke 2
	Spoke2BMCUsername string `yaml:"spoke2_bmc_username" envconfig:"ECO_OCLOUD_SPOKE2_BMC_USERNAME"`
	// Spoke2BMCPassword BMC password for spoke 2
	Spoke2BMCPassword string `yaml:"spoke2_bmc_password" envconfig:"ECO_OCLOUD_SPOKE2_BMC_PASSWORD"`
	// Spoke2BMCHost BMC IP address for spoke 2
	Spoke2BMCHost string `yaml:"spoke2_bmc_host" envconfig:"ECO_OCLOUD_SPOKE2_BMC_HOST"`
	// Spoke2BMCTimeout timeout for BMC for spoke 2
	Spoke2BMCTimeout time.Duration `yaml:"spoke2_bmc_timeout" envconfig:"ECO_OCLOUD_SPOKE2_BMC_TIMEOUT"`

	// InventoryPoolNamespace is the namespace of the inventory pool.
	InventoryPoolNamespace string `yaml:"inventory_pool_namespace" envconfig:"ECO_OCLOUD_INVENTORY_POOL_NAMESPACE"`
	// BmhSpoke1 is the BMH for the first spoke.
	BmhSpoke1 string `yaml:"bmh_spoke1" envconfig:"ECO_OCLOUD_BMH_SPOKE1"`
	// BmhSpoke2 is the BMH for the second spoke.
	BmhSpoke2 string `yaml:"bmh_spoke2" envconfig:"ECO_OCLOUD_BMH_SPOKE2"`

	// TemplateName defines the base name of the referenced ClusterTemplate.
	TemplateName string `yaml:"template_name" envconfig:"ECO_OCLOUD_TEMPLATE_NAME"`
	//nolint:lll
	// TemplateVersionAISuccess defines the version of the referenced ClusterTemplate used for the successful SNO provisioning using AI.
	TemplateVersionAISuccess string `yaml:"template_version_ai_success" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_AI_SUCCESS"`
	//nolint:lll
	// TemplateVersionAIFailure defines the version of the referenced ClusterTemplate used for the failing SNO provisioning using AI.
	TemplateVersionAIFailure string `yaml:"template_version_ai_failure" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_AI_FAILURE"`
	//nolint:lll
	// TemplateVersionSimultaneous1 defines the version of the referenced ClusterTemplate used for the multicluster provisioning with different templates.
	TemplateVersionSimultaneous1 string `yaml:"template_version_simultaneous_1" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_SIMULTANEOUS_1"`
	//nolint:lll
	// TemplateVersionSimultaneous2 defines the version of the referenced ClusterTemplate used for the multicluster provisioning with different templates.
	TemplateVersionSimultaneous2 string `yaml:"template_version_simultaneous_2" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_SIMULTANEOUS_2"`
	//nolint:lll
	// TemplateVersionIBISuccess defines the version of the referenced ClusterTemplate used for the successful SNO provisioning using IBI.
	TemplateVersionIBISuccess string `yaml:"template_version_ibi_success" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_IBI_SUCCESS"`
	//nolint:lll
	// TemplateVersionIBIFailure defines the version of the referenced ClusterTemplate used for the failing SNO provisioning using IBI.
	TemplateVersionIBIFailure string `yaml:"template_version_ibi_failure" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_IBI_FAILURE"`
	// TemplateVersionDay2 defines the version of the referenced ClusterTemplate used for the Day 2 operations.
	TemplateVersionDay2 string `yaml:"template_version_day2" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_DAY2"`
	//nolint:lll
	// TemplateVersionSeed defines the version of the referenced ClusterTemplate used for the provisioning of the seed cluster for IBI.
	TemplateVersionSeed string `yaml:"template_version_seed" envconfig:"ECO_OCLOUD_TEMPLATE_VERSION_SEED"`

	// NodeClusterName1 is the name of the first ORAN Node Cluster.
	NodeClusterName1 string `yaml:"node_cluster_name_1" envconfig:"ECO_OCLOUD_NODE_CLUSTER_NAME_1"`
	// NodeClusterName2 is the name of the second ORAN Node Cluster.
	NodeClusterName2 string `yaml:"node_cluster_name_2" envconfig:"ECO_OCLOUD_NODE_CLUSTER_NAME_2"`
	// OCloudSiteID is the ID of the of the ORAN O-Cloud Site.
	OCloudSiteID string `yaml:"ocloud_site_id" envconfig:"ECO_OCLOUD_OCLOUD_SITE_ID"`
	// ClusterName1 name of the first cluster.
	ClusterName1 string `yaml:"cluster_name_1" envconfig:"ECO_OCLOUD_CLUSTER_NAME_1"`
	// ClusterName2 name of the second cluster.
	ClusterName2 string `yaml:"cluster_name_2" envconfig:"ECO_OCLOUD_CLUSTER_NAME_2"`

	// SSHCluster2 is the address to ssh the second cluster.
	SSHCluster2 string `yaml:"ssh_address_cluster_2" envconfig:"ECO_OCLOUD_SSH_CLUSTER_2"`
	// HostName1 is the hostname of the second cluster.
	HostName1 string `yaml:"hostname1" envconfig:"ECO_OCLOUD_HOSTNAME_1"`
	// HostName2 is the hostname of the second cluster.
	HostName2 string `yaml:"hostname2" envconfig:"ECO_OCLOUD_HOSTNAME_2"`

	// AuthfilePath path to the Authfile for Skopeo commands
	AuthfilePath string `yaml:"authfile_path" envconfig:"ECO_OCLOUD_AUTHFILE_PATH"`

	// SubscriberURL is the URL of the subscriber.
	SubscriberURL string `yaml:"subscriber_url" envconfig:"ECO_OCLOUD_SUBSCRIBER_URL"`
	// SubscriberDomain is the domain of the subscriber.
	SubscriberDomain string `yaml:"subscriber_domain" envconfig:"ECO_OCLOUD_SUBSCRIBER_DOMAIN"`
	// O2IMSBaseURL is the base URL for the O2IMS API.
	O2IMSBaseURL string `yaml:"o2ims_base_url" envconfig:"ECO_OCLOUD_O2IMS_BASE_URL"`
}

// NewOCloudConfig returns instance of OCloudConfig config type.
func NewOCloudConfig() *OCloudConfig {
	log.Print("Creating new OCloudConfig struct")

	var ocloudConf OCloudConfig

	ocloudConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultOCloudParamsFile)

	err := readFile(&ocloudConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&ocloudConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	ocloudConf.Spoke1BMC = bmc.New(ocloudConf.Spoke1BMCHost).
		WithRedfishUser(ocloudConf.Spoke1BMCUsername, ocloudConf.Spoke1BMCPassword).
		WithRedfishTimeout(ocloudConf.Spoke1BMCTimeout).
		WithSSHUser(ocloudConf.Spoke1BMCUsername, ocloudConf.Spoke1BMCPassword)

	ocloudConf.Spoke2BMC = bmc.New(ocloudConf.Spoke2BMCHost).
		WithRedfishUser(ocloudConf.Spoke2BMCUsername, ocloudConf.Spoke2BMCPassword).
		WithRedfishTimeout(ocloudConf.Spoke2BMCTimeout).
		WithSSHUser(ocloudConf.Spoke2BMCUsername, ocloudConf.Spoke2BMCPassword)

	return &ocloudConf
}

func readFile(ocloudConfig *OCloudConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(ocloudConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(ocloudConfig *OCloudConfig) error {
	err := envconfig.Process("", ocloudConfig)
	if err != nil {
		return err
	}

	return nil
}
