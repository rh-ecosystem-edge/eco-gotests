package sriovoperator

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
)

var (
	// OperatorConfigDaemon defaults SR-IOV network config daemonset.
	OperatorConfigDaemon = "sriov-network-config-daemon"
	// OperatorWebhook defaults SR-IOV webhook daemonset.
	OperatorWebhook = "operator-webhook"
	// OperatorResourceInjector defaults SR-IOV network resource injector daemonset.
	OperatorResourceInjector = "network-resources-injector"
	// OperatorSriovDaemonsets represents all default SR-IOV operator daemonset names.
	OperatorSriovDaemonsets = []string{OperatorConfigDaemon, OperatorWebhook, OperatorResourceInjector}
)

// IsSriovDeployed verifies that the sriov operator is deployed.
func IsSriovDeployed(apiClient *clients.Settings, sriovOperatorNamespace string) error {
	sriovNS := namespace.NewBuilder(apiClient, sriovOperatorNamespace)
	if !sriovNS.Exists() {
		return fmt.Errorf("error SR-IOV namespace %s doesn't exist", sriovNS.Definition.Name)
	}

	for _, sriovDaemonsetName := range OperatorSriovDaemonsets {
		sriovDaemonset, err := daemonset.Pull(
			apiClient, sriovDaemonsetName, sriovOperatorNamespace)
		if err != nil {
			return fmt.Errorf("error to pull SR-IOV daemonset %s from the cluster", sriovDaemonsetName)
		}

		if !sriovDaemonset.IsReady(30 * time.Second) {
			return fmt.Errorf("error SR-IOV daemonset %s is not in ready/ready state",
				sriovDaemonsetName)
		}
	}

	return nil
}
