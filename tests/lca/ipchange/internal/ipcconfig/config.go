package ipcconfig

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcparams"
	"k8s.io/klog/v2"
)

// IPCConfig type contains ipchange configuration.
type IPCConfig struct {
	*lcaconfig.LCAConfig
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

	ipcConfig.LCAConfig = lcaconfig.NewLCAConfig()
	if ipcConfig.LCAConfig == nil {
		klog.V(ipcparams.IPCLogLevel).Info("Failed to initialize LCAConfig")

		return nil
	}

	err := envconfig.Process("", &ipcConfig)
	if err != nil {
		klog.V(ipcparams.IPCLogLevel).Infof("Error reading environment variables: %v", err)

		return nil
	}

	return &ipcConfig
}
