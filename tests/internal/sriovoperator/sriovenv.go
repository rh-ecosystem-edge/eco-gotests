package sriovoperator

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/klog/v2"
)

const (
	// SriovStabilizationDelay represents the delay before checking SR-IOV and MCP stability.
	SriovStabilizationDelay = 10 * time.Second
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
	klog.V(90).Infof("Checking if SriovOperator is deployed")

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

// WaitForSriovStable waits until all the SR-IOV node states are in sync.
func WaitForSriovStable(apiClient *clients.Settings, waitingTime time.Duration, sriovOperatorNamespace string) error {
	klog.V(90).Infof("Waiting for SR-IOV become stable.")

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

// WaitForSriovAndMCPStable waits until SR-IOV and MCP stable.
func WaitForSriovAndMCPStable(
	apiClient *clients.Settings,
	waitingTime,
	stableDuration time.Duration,
	mcpName,
	sriovOperatorNamespace string) error {
	klog.V(90).Infof("Waiting for SR-IOV and MCP become stable.")
	time.Sleep(SriovStabilizationDelay)

	err := WaitForSriovStable(apiClient, waitingTime, sriovOperatorNamespace)
	if err != nil {
		return err
	}

	err = cluster.WaitForMcpStable(apiClient, waitingTime, stableDuration, mcpName)
	if err != nil {
		return err
	}

	return nil
}

// CreateSriovPolicyAndWaitUntilItsApplied creates SriovNetworkNodePolicy and waits until
// it's successfully applied.
func CreateSriovPolicyAndWaitUntilItsApplied(
	apiClient *clients.Settings,
	workerLabel,
	sriovOperatorNamespace string,
	sriovPolicy *sriov.PolicyBuilder,
	timeout,
	stableDuration time.Duration) error {
	klog.V(90).Infof("Creating SriovNetworkNodePolicy %s and waiting until it's successfully applied.",
		sriovPolicy.Definition.Name)

	_, err := sriovPolicy.Create()
	if err != nil {
		return err
	}

	err = WaitForSriovAndMCPStable(
		apiClient, timeout, stableDuration, workerLabel, sriovOperatorNamespace)
	if err != nil {
		return err
	}

	return nil
}

// RemoveSriovConfigurationAndWaitForSriovAndMCPStable removes all SR-IOV networks
// and policies in SR-IOV operator namespace.
func RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
	apiClient *clients.Settings,
	workerLabel,
	sriovOperatorNamespace string,
	mcoTimeout,
	timeout time.Duration) error {
	klog.V(90).Infof("Removing all SR-IOV networks and policies")

	err := RemoveAllSriovNetworks(apiClient, sriovOperatorNamespace, timeout)
	if err != nil {
		klog.V(90).Infof("Failed to remove all SR-IOV networks")

		return err
	}

	err = RemoveAllPoliciesAndWaitForSriovAndMCPStable(apiClient, workerLabel, sriovOperatorNamespace, mcoTimeout)
	if err != nil {
		klog.V(90).Infof("Failed to remove all SR-IOV policies")

		return err
	}

	return nil
}

// RemoveAllSriovNetworks removes all SR-IOV networks.
func RemoveAllSriovNetworks(apiClient *clients.Settings, sriovOperatorNamespace string, timeout time.Duration) error {
	klog.V(90).Infof("Removing all SR-IOV networks")

	sriovNs, err := namespace.Pull(apiClient, sriovOperatorNamespace)
	if err != nil {
		klog.V(90).Infof("Failed to pull SR-IOV operator namespace")

		return err
	}

	err = sriovNs.CleanObjects(
		timeout,
		sriov.GetSriovNetworksGVR())
	if err != nil {
		klog.V(90).Infof("Failed to remove SR-IOV networks from SR-IOV operator namespace")

		return err
	}

	return nil
}

// RemoveAllPoliciesAndWaitForSriovAndMCPStable removes all  SriovNetworkNodePolicies and waits until
// SR-IOV and MCP become stable.
func RemoveAllPoliciesAndWaitForSriovAndMCPStable(
	apiClient *clients.Settings,
	workerLabel,
	sriovOperatorNamespace string,
	timeout time.Duration) error {
	klog.V(90).Infof("Deleting all SriovNetworkNodePolicies and waiting for SR-IOV and MCP become stable.")

	err := sriov.CleanAllNetworkNodePolicies(apiClient, sriovOperatorNamespace)
	if err != nil {
		return err
	}

	return WaitForSriovAndMCPStable(
		apiClient, timeout, time.Minute,
		workerLabel, sriovOperatorNamespace)
}
