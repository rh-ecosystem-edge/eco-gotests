package ipcconfig

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcparams"
	"k8s.io/klog/v2"
)

// IPCConfig type contains ipchange configuration.
type IPCConfig struct {
	*config.GeneralConfig
	TargetSNOKubeConfig        string `envconfig:"ECO_LCA_IPC_KUBECONFIG_TARGET_SNO"`
	ExpectedStage              string `envconfig:"ECO_LCA_IPC_EXPECTED_STAGE"`
	ExpectedIPv4Address        string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV4_ADDRESS"`
	ExpectedIPv4Gateway        string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV4_GATEWAY"`
	ExpectedIPv4MachineNetwork string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV4_MACHINE_NETWORK"`
	ExpectedIPv6Address        string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV6_ADDRESS"`
	ExpectedIPv6Gateway        string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV6_GATEWAY"`
	ExpectedIPv6MachineNetwork string `envconfig:"ECO_LCA_IPC_EXPECTED_IPV6_MACHINE_NETWORK"`
	ExpectedDNSServers         string `envconfig:"ECO_LCA_IPC_EXPECTED_DNS_SERVERS"`
}

// NewIPCConfig returns instance of IPCConfig type.
func NewIPCConfig() *IPCConfig {
	klog.V(ipcparams.IPCLogLevel).Info("Creating new IPCConfig struct")

	var ipcConfig IPCConfig

	ipcConfig.GeneralConfig = config.NewConfig()
	if ipcConfig.GeneralConfig == nil {
		klog.V(ipcparams.IPCLogLevel).Info("Failed to initialize GeneralConfig")

		return nil
	}

	err := envconfig.Process("", &ipcConfig)
	if err != nil {
		klog.V(ipcparams.IPCLogLevel).Infof("Error reading environment variables: %v", err)

		return nil
	}

	return &ipcConfig
}

// GetTargetSNOAPIClient returns the API client for the target SNO cluster.
func (config *IPCConfig) GetTargetSNOAPIClient() *clients.Settings {
	if config == nil || config.TargetSNOKubeConfig == "" {
		return nil
	}

	return clients.New(config.TargetSNOKubeConfig)
}
