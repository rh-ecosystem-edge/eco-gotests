package mgmtconfig

import (
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/internal/ibiconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/seedimage"
	"k8s.io/klog/v2"

	"gopkg.in/yaml.v2"
)

// Cluster contains resources information that make up the cluster to be installed.
type Cluster struct {
	Info struct {
		ClusterName     string `yaml:"cluster_name"`
		BaseDomain      string `yaml:"base_domain"`
		PrimaryIPFamily string `yaml:"primary_ip_family"`
		MachineCIDR     struct {
			IPv4 string `yaml:"ipv4"`
			IPv6 string `yaml:"ipv6"`
		} `yaml:"machine_cidr"`
		Hosts map[string]struct {
			Network struct {
				Interfaces map[string]struct {
					MACAddress string `yaml:"mac_address"`
				} `yaml:"interfaces"`
				BondConfig struct {
					Interfaces string `yaml:"interfaces"`
					Options    struct {
						Mode   string `yaml:"mode"`
						MIIMON string `yaml:"miimon"`
					} `yaml:"options"`
				} `yaml:"bond_config"`
				Address struct {
					IPv4 string `yaml:"ipv4"`
					IPv6 string `yaml:"ipv6"`
				} `yaml:"address"`
				Gateway struct {
					IPv4 string `yaml:"ipv4"`
					IPv6 string `yaml:"ipv6"`
				} `yaml:"default_gateway"`
				DNS struct {
					IPv4 string `yaml:"ipv4"`
					IPv6 string `yaml:"ipv6"`
				} `yaml:"dns"`
			} `yaml:"network"`
			BMC struct {
				User       string `yaml:"user"`
				Password   string `yaml:"pass"`
				URLv4      string `yaml:"urlv4"`
				URLv6      string `yaml:"urlv6"`
				MACAddress string `yaml:"mac_address"`
			} `yaml:"bmc"`
		} `yaml:"hosts"`
	} `yaml:"spoke_cluster_info"`
}

// MGMTConfig type contains mgmt configuration.
//
//nolint:lll
type MGMTConfig struct {
	*ibiconfig.IBIConfig
	Cluster                  *Cluster
	ClusterInfoPath          string `envconfig:"ECO_LCA_IBI_MGMT_CLUSTER_INFO"`
	SeedImage                string `envconfig:"ECO_LCA_IBI_MGMT_SEED_IMAGE" default:"quay.io/ocp-edge-qe/ib-seedimage-public:ci"`
	SeedClusterInfo          *seedimage.SeedImageContent
	SSHKeyPath               string `envconfig:"ECO_LCA_IBI_MGMT_SSHKEY_PATH"`
	PublicSSHKey             string
	StaticNetworking         bool   `envconfig:"ECO_LCA_IBI_MGMT_STATIC_NETWORK" default:"false"`
	ExtraManifests           bool   `envconfig:"ECO_LCA_IBI_EXTRA_MANIFESTS" default:"true"`
	CABundle                 bool   `envconfig:"ECO_LCA_IBI_CA_BUNDLE" default:"true"`
	SiteConfig               bool   `envconfig:"ECO_LCA_IBI_SITECONFIG" default:"true"`
	ExtraPartName            string `envconfig:"ECO_LCA_IBI_MGMT_EXTRA_PARTITION_NAME" default:""`
	ExtraPartSizeMib         string `envconfig:"ECO_LCA_IBI_MGMT_EXTRA_PARTITION_SIZE" default:"50000"`
	ReinstallGenerationLabel string `envconfig:"ECO_LCA_IBI_REINSTALL_GENERATION" default:"generate1"`
}

// ReinstallConfig is used to collect info for performing resinstall test.
type ReinstallConfig struct {
	ClusterIdentity           string `json:"clusterIdentity"`
	AdminKubeConfigSecretFile string `json:"adminKubeConfigSecretFile"`
	AdminPasswordSecretFile   string `json:"adminPasswordSecretFile"`
	SeedRecertSecretFile      string `json:"seedRecertSecretFile"`
}

// NewMGMTConfig returns instance of MGMTConfig type.
func NewMGMTConfig() *MGMTConfig {
	klog.V(mgmtparams.MGMTLogLevel).Info("Creating new MGMTConfig struct")

	var mgmtConfig MGMTConfig

	mgmtConfig.IBIConfig = ibiconfig.NewIBIConfig()

	err := envconfig.Process("eco_lca_ibi_mgmt_", &mgmtConfig)
	if err != nil {
		return nil
	}

	if mgmtConfig.ClusterInfoPath != "" {
		content, err := os.ReadFile(mgmtConfig.ClusterInfoPath)
		if err != nil {
			return &mgmtConfig
		}

		err = yaml.Unmarshal(content, mgmtConfig.Cluster)
		if err != nil {
			return &mgmtConfig
		}
	}

	if mgmtConfig.SSHKeyPath != "" {
		content, err := os.ReadFile(mgmtConfig.SSHKeyPath)
		if err != nil {
			return &mgmtConfig
		}

		mgmtConfig.PublicSSHKey = string(content)
	}

	return &mgmtConfig
}
