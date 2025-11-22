package ranconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/cnfconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

const (
	// PathToDefaultCnfRanParamsFile path to config file with default system tests parameters.
	PathToDefaultCnfRanParamsFile = "./default.yaml"
)

// RANConfig contains configuration for the RAN directory.
type RANConfig struct {
	*cnfconfig.CNFConfig
	*HubConfig
	*Spoke1Config
	*Spoke2Config

	MetricSamplingInterval string   `yaml:"metricSamplingInterval" envconfig:"ECO_CNF_RAN_METRIC_SAMPLING_INTERVAL"`
	NoWorkloadDuration     string   `yaml:"noWorkloadDuration" envconfig:"ECO_CNF_RAN_NO_WORKLOAD_DURATION"`
	WorkloadDuration       string   `yaml:"workloadDuration" envconfig:"ECO_CNF_RAN_WORKLOAD_DURATION"`
	StressngTestImage      string   `yaml:"stressngTestImage" envconfig:"ECO_CNF_RAN_STRESSNG_TEST_IMAGE"`
	CnfTestImage           string   `yaml:"cnfTestImage" envconfig:"ECO_CNF_RAN_TEST_IMAGE"`
	OcpUpgradeUpstreamURL  string   `yaml:"ocpUpgradeUpstreamUrl" envconfig:"ECO_CNF_RAN_OCP_UPGRADE_UPSTREAM_URL"`
	PtpOperatorNamespace   string   `yaml:"ptpOperatorNamespace" envconfig:"ECO_CNF_RAN_PTP_OPERATOR_NAMESPACE"`
	TalmPreCachePolicies   []string `yaml:"talmPreCachePolicies" envconfig:"ECO_CNF_RAN_TALM_PRECACHE_POLICIES"`
	ZtpSiteGenerateImage   string   `yaml:"ztpSiteGenerateImage" envconfig:"ECO_CNF_RAN_ZTP_SITE_GENERATE_IMAGE"`
	// PtpEventConsumerImage is the URL of the PTP event consumer image. It should not have a tag, since the
	// expectation is that the program uses v1 or v2 as a tag.
	PtpEventConsumerImage string `yaml:"ptpEventConsumerImage" envconfig:"ECO_CNF_RAN_PTP_EVENT_CONSUMER_IMAGE"`
	// PtpEventConsumerV1Tag is the tag of the PTP event consumer image for v1. It should include the leading colon
	// so that digests may be specified if needed.
	PtpEventConsumerV1Tag string `yaml:"ptpEventConsumerV1Tag" envconfig:"ECO_CNF_RAN_PTP_EVENT_CONSUMER_V1_TAG"`
	// PtpEventConsumerV2Tag is the tag of the PTP event consumer image for v2. It should include the leading colon
	// so that digests may be specified if needed.
	PtpEventConsumerV2Tag string `yaml:"ptpEventConsumerV2Tag" envconfig:"ECO_CNF_RAN_PTP_EVENT_CONSUMER_V2_TAG"`
	// ClusterTemplateAffix is the version-dependent affix used for naming ClusterTemplates and other O-RAN
	// resources.
	ClusterTemplateAffix string `envconfig:"ECO_CNF_RAN_CLUSTER_TEMPLATE_AFFIX"`
}

// HubConfig contains the configuration for the hub cluster, if present.
type HubConfig struct {
	HubAPIClient        *clients.Settings
	HubOCPVersion       string
	ZTPVersion          string
	HubOperatorVersions map[ranparam.HubOperatorName]string
	HubKubeconfig       string `envconfig:"ECO_CNF_RAN_KUBECONFIG_HUB"`

	// HubAppsDomain is the subdomain for the hub cluster's routes. It should be
	// apps.<hub-cluster-name>.<hub-cluster-domain> with no leading or trailing dots.
	//
	// When running the O-RAN suite, it is assumed that the OAuth endpoint is at keycloak.<hub-apps-domain> and the
	// O2IMS API is at o2ims.<hub-apps-domain>.
	HubAppsDomain string `envconfig:"ECO_CNF_RAN_HUB_APPS_DOMAIN"`

	// O2IMSClientCertSecret is the name of the secret in the O2IMSClientCertSecretNamespace namespace that contains
	// the client certificate to use when interacting with the O2IMS and OAuth APIs.
	//
	// It is expected that the secret contains 3 different keys: tls.crt, tls.key, and ca.crt (optional). The
	// tls.crt and tls.key are used for mTLS and certificate-bound tokens (RFC 8705). If the ca.crt is present, it
	// is added as a CA certificate when interacting with the O2IMS and OAuth APIs.
	O2IMSClientCertSecret string `envconfig:"ECO_CNF_RAN_O2IMS_CLIENT_CERT_SECRET"`
	// O2IMSClientCertSecretNamespace is the namespace for the O2IMS client certificate secret.
	O2IMSClientCertSecretNamespace string `envconfig:"ECO_CNF_RAN_O2IMS_CLIENT_CERT_SECRET_NAMESPACE"`

	// O2IMSOAuthClientID is the client ID used to request the access token from the OAuth endpoint using the client
	// credentials grant type.
	O2IMSOAuthClientID string `envconfig:"ECO_CNF_RAN_O2IMS_OAUTH_CLIENT_ID"`
	// O2IMSOAuthClientSecret is a string used to request the access token from the OAuth endpoint using the client
	// credentials grant type.
	O2IMSOAuthClientSecret string `envconfig:"ECO_CNF_RAN_O2IMS_OAUTH_CLIENT_SECRET"`

	// O2IMSToken is the token for the O-RAN suite to authenticate with the O2IMS API. It is only used when OAuth is
	// not configured.
	O2IMSToken string `envconfig:"ECO_CNF_RAN_O2IMS_TOKEN"`
}

// GetAppsURL returns the apps URL for the given subdomain. It should end up being in a form similar to
// <subdomain>.apps.<hub-cluster-name>.<hub-cluster-domain>.
func (hubConfig *HubConfig) GetAppsURL(subdomain string) string {
	return fmt.Sprintf("%s.%s", subdomain, hubConfig.HubAppsDomain)
}

// Spoke1Config contains the configuration for the spoke 1 cluster, which should always be present.
type Spoke1Config struct {
	Spoke1BMC       *bmc.BMC
	Spoke1APIClient *clients.Settings

	Spoke1OCPVersion       string
	Spoke1OperatorVersions map[ranparam.SpokeOperatorName]string

	// Spoke1Name is automatically updated if Spoke1Kubeconfig exists, otherwise it can be provided as an input.
	Spoke1Name string `envconfig:"ECO_CNF_RAN_SPOKE1_NAME"`
	// Spoke1Hostname is not automatically updated but instead used as an input for the O-RAN suite.
	Spoke1Hostname   string `envconfig:"ECO_CNF_RAN_SPOKE1_HOSTNAME"`
	Spoke1Kubeconfig string `envconfig:"KUBECONFIG"`
	// Spoke1Password is the path to the admin password, saved in the O-RAN suite.
	Spoke1Password string `envconfig:"ECO_CNF_RAN_SPOKE1_PASSWORD"`

	BMCUsername string        `envconfig:"ECO_CNF_RAN_BMC_USERNAME"`
	BMCPassword string        `envconfig:"ECO_CNF_RAN_BMC_PASSWORD"`
	BMCHosts    []string      `envconfig:"ECO_CNF_RAN_BMC_HOSTS"`
	BMCTimeout  time.Duration `yaml:"bmcTimeout" envconfig:"ECO_CNF_RAN_BMC_TIMEOUT"`
}

// Spoke2Config contains the configuration for the spoke 2 cluster, if present.
type Spoke2Config struct {
	Spoke2APIClient  *clients.Settings
	Spoke2Name       string
	Spoke2OCPVersion string
	Spoke2Kubeconfig string `envconfig:"ECO_CNF_RAN_KUBECONFIG_SPOKE2"`
}

// NewRANConfig returns an instance of RANConfig.
func NewRANConfig() *RANConfig {
	klog.V(ranparam.LogLevel).Infof("Creating new RANConfig struct")

	var ranConfig RANConfig

	ranConfig.CNFConfig = cnfconfig.NewCNFConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	configFile := filepath.Join(baseDir, PathToDefaultCnfRanParamsFile)

	err := readConfig(&ranConfig, configFile)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Error reading main RAN Config: %v", err)

		return nil
	}

	ranConfig.newHubConfig(configFile)
	ranConfig.newSpoke1Config(configFile)
	ranConfig.newSpoke2Config(configFile)

	return &ranConfig
}

func (ranconfig *RANConfig) newHubConfig(configFile string) {
	klog.V(ranparam.LogLevel).Infof("Creating new HubConfig struct from file %s", configFile)

	ranconfig.HubConfig = new(HubConfig)

	err := readConfig(ranconfig.HubConfig, configFile)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to instantiate HubConfig: %v", err)
	}

	if ranconfig.HubConfig.HubKubeconfig == "" {
		klog.V(ranparam.LogLevel).Info("No kubeconfig found for hub")

		return
	}

	ranconfig.HubConfig.HubAPIClient = clients.New(ranconfig.HubConfig.HubKubeconfig)

	ranconfig.HubConfig.HubOCPVersion, err = version.GetOCPVersion(ranconfig.HubConfig.HubAPIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from hub: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on hub: %s", ranconfig.HubConfig.HubOCPVersion)

	ranconfig.HubConfig.HubOperatorVersions = make(map[ranparam.HubOperatorName]string)

	ranconfig.HubConfig.HubOperatorVersions[ranparam.ACM], err = version.GetOperatorVersionFromCsv(
		ranconfig.HubConfig.HubAPIClient, string(ranparam.ACM), ranparam.AcmOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get ACM version from hub: %v", err)
	}

	ranconfig.HubConfig.HubOperatorVersions[ranparam.GitOps], err = version.GetOperatorVersionFromCsv(
		ranconfig.HubConfig.HubAPIClient, string(ranparam.GitOps), ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get GitOps version from hub: %v", err)
	}

	ranconfig.HubConfig.HubOperatorVersions[ranparam.MCE], err = version.GetOperatorVersionFromCsv(
		ranconfig.HubConfig.HubAPIClient, string(ranparam.MCE), ranparam.MceOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get MCE version from hub: %v", err)
	}

	ranconfig.HubConfig.HubOperatorVersions[ranparam.TALM], err = version.GetOperatorVersionFromCsv(
		ranconfig.HubConfig.HubAPIClient, string(ranparam.TALM), ranparam.OpenshiftOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get TALM version from hub: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found operator versions on hub: %v", ranconfig.HubConfig.HubOperatorVersions)

	ranconfig.HubConfig.ZTPVersion, err = version.GetZTPVersionFromArgoCd(
		ranconfig.HubConfig.HubAPIClient, ranparam.OpenshiftGitopsRepoServer, ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get ZTP version from hub: %v", err)
	}

	ranconfig.ZtpSiteGenerateImage, err = version.GetZTPSiteGenerateImage(ranconfig.HubConfig.HubAPIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get ZTP site generate image from hub: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found ZTP version on hub: %s", ranconfig.HubConfig.ZTPVersion)
}

func (ranconfig *RANConfig) newSpoke1Config(configFile string) {
	klog.V(ranparam.LogLevel).Infof("Creating new Spoke1Config struct from file %s", configFile)

	ranconfig.Spoke1Config = new(Spoke1Config)

	err := readConfig(ranconfig.Spoke1Config, configFile)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to instantiate Spoke1Config: %v", err)
	}

	ranconfig.Spoke1Config.Spoke1APIClient = inittools.APIClient

	if spoke1Kubeconfig := ranconfig.Spoke1Config.Spoke1Kubeconfig; spoke1Kubeconfig != "" {
		spoke1Name, err := version.GetClusterName(spoke1Kubeconfig)
		if err != nil {
			klog.V(ranparam.LogLevel).Infof("Failed to get spoke 1 name from kubeconfig at %s: %v", spoke1Kubeconfig, err)
		} else {
			ranconfig.Spoke1Config.Spoke1Name = spoke1Name
		}
	} else {
		klog.V(ranparam.LogLevel).Infof("No spoke 1 kubeconfig specified in KUBECONFIG environment variable")
	}

	ranconfig.Spoke1Config.Spoke1OCPVersion, err = version.GetOCPVersion(ranconfig.Spoke1Config.Spoke1APIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from spoke 1: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on spoke 1: %s", ranconfig.Spoke1Config.Spoke1OCPVersion)

	ranconfig.Spoke1Config.Spoke1OperatorVersions = make(map[ranparam.SpokeOperatorName]string)

	ranconfig.Spoke1Config.Spoke1OperatorVersions[ranparam.PTP], err = version.GetOperatorVersionFromCsv(
		ranconfig.Spoke1Config.Spoke1APIClient, string(ranparam.PTP), ranparam.PtpOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get PTP version from spoke 1: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found operator versions on spoke 1: %v",
		ranconfig.Spoke1Config.Spoke1OperatorVersions)

	if len(ranconfig.Spoke1Config.BMCHosts) > 0 &&
		ranconfig.Spoke1Config.BMCUsername != "" &&
		ranconfig.Spoke1Config.BMCPassword != "" {
		bmcHost := ranconfig.Spoke1Config.BMCHosts[0]
		if len(ranconfig.Spoke1Config.BMCHosts) > 1 {
			klog.V(ranparam.LogLevel).Infof("Found more than one BMC host, using the first one: %s", bmcHost)
		}

		ranconfig.Spoke1Config.Spoke1BMC = bmc.New(bmcHost).
			WithRedfishUser(ranconfig.Spoke1Config.BMCUsername, ranconfig.Spoke1Config.BMCPassword).
			WithRedfishTimeout(ranconfig.Spoke1Config.BMCTimeout)
	}
}

func (ranconfig *RANConfig) newSpoke2Config(configFile string) {
	klog.V(ranparam.LogLevel).Infof("Creating new Spoke2Config struct from file %s", configFile)

	ranconfig.Spoke2Config = new(Spoke2Config)

	err := readConfig(ranconfig.Spoke2Config, configFile)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to instantiate Spoke2Config: %v", err)
	}

	if ranconfig.Spoke2Config.Spoke2Kubeconfig == "" {
		klog.V(ranparam.LogLevel).Info("No kubeconfig found for spoke 2")

		return
	}

	ranconfig.Spoke2Config.Spoke2APIClient = clients.New(ranconfig.Spoke2Config.Spoke2Kubeconfig)

	ranconfig.Spoke2Config.Spoke2Name, err = version.GetClusterName(ranconfig.Spoke2Config.Spoke2Kubeconfig)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get spoke 2 name from kubeconfig at %s: %v", ranconfig.Spoke2Config.Spoke2Kubeconfig, err)
	}

	ranconfig.Spoke2Config.Spoke2OCPVersion, err = version.GetOCPVersion(ranconfig.Spoke2Config.Spoke2APIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from spoke 2: %v", err)
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on spoke 2: %s", ranconfig.Spoke2Config.Spoke2OCPVersion)
}

func readConfig[C any](config *C, configFile string) error {
	err := readFile(config, configFile)
	if err != nil {
		return err
	}

	return readEnv(config)
}

func readFile[C any](config *C, configFile string) error {
	openedConfigFile, err := os.Open(configFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedConfigFile.Close()
	}()

	decoder := yaml.NewDecoder(openedConfigFile)

	err = decoder.Decode(config)

	return err
}

func readEnv[C any](config *C) error {
	err := envconfig.Process("", config)

	return err
}
