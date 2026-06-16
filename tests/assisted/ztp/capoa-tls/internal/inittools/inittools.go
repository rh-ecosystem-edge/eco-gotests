package inittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"k8s.io/klog/v2"
)

var (
	// HubAPIClient provides API access to hub cluster via KUBECONFIG env var.
	HubAPIClient *clients.Settings
	// GeneralConfig provides access to general configuration parameters.
	GeneralConfig *config.GeneralConfig
)

func init() {
	if GeneralConfig = config.NewConfig(); GeneralConfig == nil {
		klog.Fatalf("failed to initialize GeneralConfig: config.NewConfig returned nil")
	}

	if HubAPIClient = clients.New(""); HubAPIClient == nil {
		if GeneralConfig.DryRun {
			return
		}

		klog.Fatalf("failed to initialize HubAPIClient: clients.New returned nil")
	}
}
