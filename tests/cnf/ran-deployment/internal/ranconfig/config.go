package ranconfig

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/cnfconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/version"
	"k8s.io/klog/v2"
)

// RANConfig contains configuration for the RAN Deployment directory.
type RANConfig struct {
	*cnfconfig.CNFConfig
	*HubConfig
	*Spoke1Config
	*Spoke2Config
	// Allow skipping TLS verification for the go-git client.
	SkipTLSVerify bool `default:"false" envconfig:"ECO_CNF_RAN_SKIP_TLS_VERIFY"`
}

// HubConfig contains the configuration for the hub cluster, if present.
type HubConfig struct {
	HubAPIClient  *clients.Settings
	HubOCPVersion string
	HubKubeconfig string `envconfig:"ECO_CNF_RAN_KUBECONFIG_HUB"`
}

// Spoke1Config contains the configuration for the spoke 1 cluster, which should always be present.
type Spoke1Config struct {
	Spoke1APIClient  *clients.Settings
	Spoke1OCPVersion string
	Spoke1Name       string `envconfig:"ECO_CNF_RAN_SPOKE1_NAME"`
	Spoke1Kubeconfig string `envconfig:"KUBECONFIG"`
}

// Spoke2Config contains the configuration for the spoke 2 cluster, if present.
type Spoke2Config struct {
	Spoke2APIClient  *clients.Settings
	Spoke2OCPVersion string
	Spoke2Name       string
	Spoke2Kubeconfig string `envconfig:"ECO_CNF_RAN_KUBECONFIG_SPOKE2"`
}

// NewRANConfig returns an instance of RANConfig.
func NewRANConfig() *RANConfig {
	klog.V(ranparam.LogLevel).Infof("Creating new RANConfig struct")

	var ranConfig RANConfig

	ranConfig.HubConfig = new(HubConfig)
	ranConfig.Spoke1Config = new(Spoke1Config)
	ranConfig.Spoke2Config = new(Spoke2Config)
	ranConfig.CNFConfig = cnfconfig.NewCNFConfig()

	err := readEnv(&ranConfig)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Error reading main RAN Environment: %v", err)

		return nil
	}

	ranConfig.newHubConfig()
	ranConfig.newSpoke1Config()
	ranConfig.newSpoke2Config()

	if ranConfig.SkipTLSVerify {
		klog.V(ranparam.LogLevel).Infof("Skip TLS verification is true")
	}

	return &ranConfig
}

func (ranconfig *RANConfig) newHubConfig() {
	var err error

	if ranconfig.HubConfig.HubKubeconfig == "" {
		klog.V(ranparam.LogLevel).Info("No kubeconfig found for hub")

		return
	}

	ranconfig.HubConfig.HubAPIClient = clients.New(ranconfig.HubConfig.HubKubeconfig)

	ranconfig.HubConfig.HubOCPVersion, err = version.GetOCPVersion(ranconfig.HubConfig.HubAPIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from hub: %v", err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on hub: %s", ranconfig.HubConfig.HubOCPVersion)
}

func (ranconfig *RANConfig) newSpoke1Config() {
	var err error

	if ranconfig.Spoke1Config.Spoke1Kubeconfig == "" {
		klog.V(ranparam.LogLevel).Infof("No spoke 1 kubeconfig specified in KUBECONFIG environment variable")

		return
	}

	ranconfig.Spoke1Config.Spoke1APIClient = clients.New(ranconfig.Spoke1Config.Spoke1Kubeconfig)

	if ranconfig.Spoke1APIClient == nil || ranconfig.Spoke1APIClient.KubeconfigPath == "" {
		klog.V(ranparam.LogLevel).Infof("No spoke 1 API Client or KUBECONFIG defined")

		return
	}

	klusterlet, err := ocm.PullKlusterlet(ranconfig.Spoke1Config.Spoke1APIClient, ocm.KlusterletName)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get spoke 1 klusterlet at %s: %v", ranconfig.Spoke1Config.Spoke1Kubeconfig, err)

		return
	}

	ranconfig.Spoke1Config.Spoke1Name = klusterlet.Object.Spec.ClusterName
	if ranconfig.Spoke1Config.Spoke1Name == "" {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get spoke 1 name from klusterlet at %s: %v", ranconfig.Spoke1Config.Spoke1Kubeconfig, err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Found cluster name on spoke 1: %s", ranconfig.Spoke1Config.Spoke1Name)

	ranconfig.Spoke1Config.Spoke1OCPVersion, err = version.GetOCPVersion(ranconfig.Spoke1Config.Spoke1APIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from spoke 1: %v", err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on spoke 1: %s", ranconfig.Spoke1Config.Spoke1OCPVersion)
}

func (ranconfig *RANConfig) newSpoke2Config() {
	var err error

	if ranconfig.Spoke2Config.Spoke2Kubeconfig == "" {
		klog.V(ranparam.LogLevel).Infof(
			"No spoke 2 kubeconfig specified in ECO_CNF_RAN_KUBECONFIG_SPOKE2 environment variable")

		return
	}

	ranconfig.Spoke2Config.Spoke2APIClient = clients.New(ranconfig.Spoke2Config.Spoke2Kubeconfig)

	if ranconfig.Spoke2APIClient == nil || ranconfig.Spoke2APIClient.KubeconfigPath == "" {
		klog.V(ranparam.LogLevel).Infof(
			"No spoke 2 API Client, ECO_CNF_RAN_KUBECONFIG_SPOKE2 not defined, or spoke 2 KUBECONFIG file missing")

		return
	}

	klusterlet, err := ocm.PullKlusterlet(ranconfig.Spoke2Config.Spoke2APIClient, ocm.KlusterletName)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get spoke 2 klusterlet at %s: %v", ranconfig.Spoke2Config.Spoke2Kubeconfig, err)

		return
	}

	ranconfig.Spoke2Config.Spoke2Name = klusterlet.Object.Spec.ClusterName
	if ranconfig.Spoke2Config.Spoke2Name == "" {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get spoke 2 name from klusterlet at %s: %v", ranconfig.Spoke2Config.Spoke2Kubeconfig, err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Found cluster name on spoke 2: %s", ranconfig.Spoke2Config.Spoke2Name)

	ranconfig.Spoke2Config.Spoke2OCPVersion, err = version.GetOCPVersion(ranconfig.Spoke2Config.Spoke2APIClient)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get OCP version from spoke 2: %v", err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Found OCP version on spoke 2: %s", ranconfig.Spoke2Config.Spoke2OCPVersion)
}

func readEnv[C any](config *C) error {
	var err error

	envconfig.MustProcess("", config)

	return err
}
