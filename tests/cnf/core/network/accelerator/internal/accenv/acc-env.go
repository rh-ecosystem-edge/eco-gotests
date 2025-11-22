package accenv

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	"k8s.io/klog/v2"
)

// DoesClusterSupportAcceleratorTests verifies if given cluster supports accelerator workload and test cases.
func DoesClusterSupportAcceleratorTests(
	apiClient *clients.Settings, netConfig *netconfig.NetworkConfig) error {
	klog.V(90).Infof("Verifying if cluster supports accelerator tests")

	err := netenv.DoesClusterHasEnoughNodes(apiClient, netConfig, 1, 2)
	if err != nil {
		return err
	}

	return nil
}
