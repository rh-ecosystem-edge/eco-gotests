package netenv

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"k8s.io/klog/v2"
)

// WaitForSriovAndMCPStable waits until SR-IOV and MCP stable.
func WaitForSriovAndMCPStable(
	apiClient *clients.Settings, waitingTime, stableDuration time.Duration, mcpName, sriovOperatorNamespace string) error {
	klog.V(90).Infof("Waiting for SR-IOV and MCP become stable.")
	time.Sleep(10 * time.Second)

	err := WaitForSriovStable(apiClient, waitingTime, sriovOperatorNamespace)
	if err != nil {
		return err
	}

	err = WaitForMcpStable(apiClient, waitingTime, stableDuration, mcpName)
	if err != nil {
		return err
	}

	return nil
}

// WaitForSriovStable waits until all the SR-IOV node states are in sync.
func WaitForSriovStable(apiClient *clients.Settings, waitingTime time.Duration, sriovOperatorNamespace string) error {
	networkNodeStateList, err := sriov.ListNetworkNodeState(apiClient, sriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to fetch nodes state %w", err)
	}

	if len(networkNodeStateList) == 0 {
		return nil
	}

	for _, nodeState := range networkNodeStateList {
		err = nodeState.WaitUntilSyncStatus("Succeeded", waitingTime)
		if err != nil {
			return err
		}
	}

	return nil
}
