package ztpconfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/internal/assistedconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/find"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

// ZTPConfig type contains ztp configuration.
type ZTPConfig struct {
	*assistedconfig.AssistedConfig
	*HubConfig
	*SpokeConfig
}

// HubConfig contains environment information related to the hub cluster.
type HubConfig struct {
	HubAPIClient               *clients.Settings
	HubOCPVersion              string
	HubOCPXYVersion            string
	HubAgentServiceConfig      *assisted.AgentServiceConfigBuilder
	hubAssistedServicePod      *pod.Builder
	hubAssistedImageServicePod *pod.Builder
	HubPullSecret              *secret.Builder
	HubInstallConfig           *configmap.Builder
	HubPullSecretOverride      map[string][]byte
	HubPullSecretOverridePath  string `envconfig:"ECO_ASSISTED_ZTP_HUB_PULL_SECRET_OVERRIDE_PATH"`
}

// SpokeConfig contains environment information related to the spoke cluster.
type SpokeConfig struct {
	SpokeAPIClient           *clients.Settings
	SpokeOCPVersion          string
	SpokeOCPXYVersion        string
	SpokeClusterName         string
	SpokeKubeConfig          string `envconfig:"ECO_ASSISTED_ZTP_SPOKE_KUBECONFIG"`
	SpokeClusterImageSet     string `envconfig:"ECO_ASSISTED_ZTP_SPOKE_CLUSTERIMAGESET"`
	SpokeClusterDeployment   *hive.ClusterDeploymentBuilder
	SpokeAgentClusterInstall *assisted.AgentClusterInstallBuilder
	SpokeInfraEnv            *assisted.InfraEnvBuilder
	SpokeInstallConfig       *configmap.Builder
}

// NewZTPConfig returns instance of ZTPConfig type.
func NewZTPConfig() *ZTPConfig {
	klog.V(ztpparams.ZTPLogLevel).Info("Creating new ZTPConfig struct")

	var ztpconfig ZTPConfig

	ztpconfig.AssistedConfig = assistedconfig.NewAssistedConfig()

	ztpconfig.HubConfig = new(HubConfig)
	ztpconfig.SpokeConfig = new(SpokeConfig)

	if err := ztpconfig.newHubConfig(); err != nil {
		ztpconfig.HubConfig.HubAPIClient = nil

		return &ztpconfig
	}

	if err := ztpconfig.newSpokeConfig(); err != nil {
		ztpconfig.SpokeConfig.SpokeAPIClient = nil

		return &ztpconfig
	}

	return &ztpconfig
}

// newHubConfig creates a new HubConfig member for a ZTPConfig.
func (ztpconfig *ZTPConfig) newHubConfig() error {
	klog.V(ztpparams.ZTPLogLevel).Info("Creating new HubConfig struct")

	ztpconfig.HubConfig = new(HubConfig)

	err := envconfig.Process("eco_assisted_ztp_hub_", ztpconfig.HubConfig)
	if err != nil {
		klog.V(ztpparams.ZTPLogLevel).Infof("failed to instantiate HubConfig: %v", err)
	}

	if ztpconfig.HubConfig.HubPullSecretOverridePath != "" {
		content, err := os.ReadFile(ztpconfig.HubConfig.HubPullSecretOverridePath)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to read hub pull-secret override path: %v", err)
		}

		ztpconfig.HubConfig.HubPullSecretOverride = map[string][]byte{
			".dockerconfigjson": content,
		}
	}

	ztpconfig.HubConfig.HubAPIClient = APIClient

	if ztpconfig.HubConfig.HubAPIClient == nil {
		return fmt.Errorf("error: received nil hub apiClient")
	}

	ztpconfig.HubConfig.HubOCPVersion, err = find.ClusterVersion(ztpconfig.HubConfig.HubAPIClient)
	if err != nil {
		return err
	}

	splitVersion := strings.Split(ztpconfig.HubConfig.HubOCPVersion, ".")
	if len(splitVersion) >= 2 {
		ztpconfig.HubConfig.HubOCPXYVersion = fmt.Sprintf("%s.%s", splitVersion[0], splitVersion[1])
	}

	ztpconfig.HubConfig.HubAgentServiceConfig, err = assisted.PullAgentServiceConfig(ztpconfig.HubConfig.HubAPIClient)
	if err != nil {
		return err
	}

	if ztpconfig.HubConfig.HubAgentServiceConfig != nil {
		assistedPod := ztpconfig.HubAssistedServicePod()
		if assistedPod == nil {
			return fmt.Errorf("failed to find hub assisted service pod")
		}

		assistedImagePod := ztpconfig.HubAssistedImageServicePod()
		if assistedImagePod == nil {
			return fmt.Errorf("failed to find hub assisted image service pod")
		}
	}

	ztpconfig.HubConfig.HubPullSecret, err = cluster.GetOCPPullSecret(ztpconfig.HubConfig.HubAPIClient)
	if err != nil {
		return err
	}

	if ztpconfig.DryRun {
		return nil
	}

	ztpconfig.HubConfig.HubInstallConfig, err =
		configmap.Pull(ztpconfig.HubConfig.HubAPIClient, "cluster-config-v1", "kube-system")
	if err != nil {
		return err
	}

	return nil
}

// newSpokeConfig creates a new SpokeConfig member for a ZTPConfig.
func (ztpconfig *ZTPConfig) newSpokeConfig() error {
	klog.V(ztpparams.ZTPLogLevel).Info("Creating new SpokeConfig struct")

	err := envconfig.Process("eco_assisted_ztp_spoke_", ztpconfig.SpokeConfig)
	if err != nil {
		klog.V(ztpparams.ZTPLogLevel).Infof("failed to instantiate SpokeConfig: %v", err)

		return err
	}

	if ztpconfig.SpokeConfig.SpokeKubeConfig != "" {
		klog.V(ztpparams.ZTPLogLevel).Infof("Creating spoke api client from %s", ztpconfig.SpokeConfig.SpokeKubeConfig)

		if ztpconfig.SpokeConfig.SpokeAPIClient = clients.New(
			ztpconfig.SpokeConfig.SpokeKubeConfig); ztpconfig.SpokeConfig.SpokeAPIClient == nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to load provided spoke kubeconfig: %v",
				ztpconfig.SpokeConfig.SpokeKubeConfig)

			return fmt.Errorf("failed to load provided spoke kubeconfig: %v", ztpconfig.SpokeConfig.SpokeKubeConfig)
		}

		ztpconfig.SpokeConfig.SpokeClusterName, err =
			find.SpokeClusterName(ztpconfig.HubConfig.HubAPIClient, ztpconfig.SpokeConfig.SpokeAPIClient)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke cluster name: %v", err)

			return err
		}

		ztpconfig.SpokeConfig.SpokeOCPVersion, err = find.ClusterVersion(ztpconfig.SpokeConfig.SpokeAPIClient)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke cluster version: %v", err)

			return err
		}

		splitVersion := strings.Split(ztpconfig.SpokeConfig.SpokeOCPVersion, ".")
		if len(splitVersion) >= 2 {
			ztpconfig.SpokeConfig.SpokeOCPXYVersion = fmt.Sprintf("%s.%s", splitVersion[0], splitVersion[1])
		}

		ztpconfig.SpokeConfig.SpokeClusterDeployment, err = hive.PullClusterDeployment(ztpconfig.HubConfig.HubAPIClient,
			ztpconfig.SpokeConfig.SpokeClusterName, ztpconfig.SpokeConfig.SpokeClusterName)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke cluster deployment: %v", err)

			return err
		}

		ztpconfig.SpokeConfig.SpokeAgentClusterInstall, err =
			assisted.PullAgentClusterInstall(ztpconfig.HubConfig.HubAPIClient,
				ztpconfig.SpokeConfig.SpokeClusterName, ztpconfig.SpokeConfig.SpokeClusterName)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke agent cluster install: %v", err)

			return err
		}

		ztpconfig.SpokeConfig.SpokeInfraEnv, err = assisted.PullInfraEnvInstall(ztpconfig.HubConfig.HubAPIClient,
			ztpconfig.SpokeConfig.SpokeClusterName, ztpconfig.SpokeConfig.SpokeClusterName)
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke infra env: %v", err)

			return err
		}

		ztpconfig.SpokeConfig.SpokeInstallConfig, err = configmap.Pull(ztpconfig.SpokeConfig.SpokeAPIClient,
			"cluster-config-v1", "kube-system")
		if err != nil {
			klog.V(ztpparams.ZTPLogLevel).Infof("failed to find spoke install config: %v", err)

			return err
		}
	} else {
		ztpconfig.SpokeConfig.SpokeAPIClient = nil
	}

	if ztpconfig.SpokeConfig.SpokeClusterImageSet == "" {
		ztpconfig.SpokeConfig.SpokeClusterImageSet = ztpconfig.HubOCPXYVersion
	}

	return nil
}

// HubAssistedServicePod retrieves the assisted service pod from the hub
// and populates hubAssistedServicePod.
func (ztpconfig *ZTPConfig) HubAssistedServicePod() *pod.Builder {
	if ztpconfig.hubAssistedServicePod == nil || !ztpconfig.hubAssistedServicePod.Exists() {
		ztpconfig.hubAssistedServicePod, _ = find.AssistedServicePod(ztpconfig.HubAPIClient)
	}

	return ztpconfig.hubAssistedServicePod
}

// HubAssistedImageServicePod retrieves the assisted image service pod from the hub
// and populates hubAssistedImageServicePod.
func (ztpconfig *ZTPConfig) HubAssistedImageServicePod() *pod.Builder {
	if ztpconfig.hubAssistedImageServicePod == nil || !ztpconfig.hubAssistedImageServicePod.Exists() {
		ztpconfig.hubAssistedImageServicePod, _ = find.AssistedImageServicePod(ztpconfig.HubAPIClient)
	}

	return ztpconfig.hubAssistedImageServicePod
}
