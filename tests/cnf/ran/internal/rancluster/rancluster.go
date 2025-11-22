package rancluster

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// AreClustersPresent checks all of the provided clusters and returns false if any are nil.
func AreClustersPresent(clusters []*clients.Settings) bool {
	for _, cluster := range clusters {
		if cluster == nil {
			return false
		}
	}

	return true
}

// GetPlusOneWorkerName gets the name of the one worker in a SNO+1 cluster.
func GetPlusOneWorkerName(client *clients.Settings) (string, error) {
	workers, err := ListNodesByLabel(client, RANConfig.WorkerLabelMap)
	if err != nil {
		return "", err
	}

	for _, worker := range workers {
		if !IsNodeControlPlane(worker) {
			return worker.Definition.Name, nil
		}
	}

	return "", fmt.Errorf("could not find a worker node for cluster")
}

// IsClusterStable checks if the provided cluster does not have any unschedulable nodes.
func IsClusterStable(client *clients.Settings) (bool, error) {
	nodeList, err := nodes.List(client)
	if err != nil {
		return false, err
	}

	for _, node := range nodeList {
		if node.Definition.Spec.Unschedulable {
			return false, nil
		}
	}

	return true, nil
}

// IsNodeControlPlane checks whether the provided node is a control plane node.
func IsNodeControlPlane(node *nodes.Builder) bool {
	_, exists := node.Definition.Labels[RANConfig.ControlPlaneLabel]

	return exists
}

// IsSnoPlusOne checks if the specified cluster has one control plane and one worker node.
func IsSnoPlusOne(client *clients.Settings) (bool, error) {
	controlPlanes, err := ListNodesByLabel(client, RANConfig.ControlPlaneLabelMap)
	if err != nil {
		return false, err
	}

	if len(controlPlanes) != 1 {
		return false, nil
	}

	klog.V(tsparams.LogLevel).Info("Exactly one control plane node found")

	workers, err := ListNodesByLabel(client, RANConfig.WorkerLabelMap)
	if err != nil {
		return false, err
	}

	trueWorkers := 0

	for _, worker := range workers {
		if !IsNodeControlPlane(worker) {
			trueWorkers++
		}
	}

	if trueWorkers != 1 {
		return false, nil
	}

	klog.V(tsparams.LogLevel).Info("Exactly one worker node found")

	return true, nil
}

// IsSNO checks if the specified cluster is a SNO cluster. For it to be a SNO cluster, there must be exactly one node
// that is both a control plane and a worker node.
func IsSNO(client *clients.Settings) (bool, error) {
	workers, err := ListNodesByLabel(client, RANConfig.WorkerLabelMap)
	if err != nil {
		return false, fmt.Errorf("failed to list worker nodes: %w", err)
	}

	controlPlanes, err := ListNodesByLabel(client, RANConfig.ControlPlaneLabelMap)
	if err != nil {
		return false, fmt.Errorf("failed to list control plane nodes: %w", err)
	}

	if len(workers) == 1 && len(controlPlanes) == 1 && IsNodeControlPlane(workers[0]) {
		return true, nil
	}

	return false, nil
}

// ListNodesByLabel returns a list of nodes that have the specified label.
func ListNodesByLabel(client *clients.Settings, labelMap map[string]string) ([]*nodes.Builder, error) {
	return nodes.List(client, metav1.ListOptions{
		LabelSelector: labels.Set(labelMap).String(),
	})
}

// WaitForNumberOfNodes waits up to timeout until the number of nodes on the cluster matches the expected.
func WaitForNumberOfNodes(client *clients.Settings, expected int, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(client)
			if err != nil {
				return false, err
			}

			if len(nodeList) == expected {
				return true, nil
			}

			klog.V(tsparams.LogLevel).Infof("Expected %d nodes but found %d nodes", expected, len(nodeList))

			return false, nil
		})
}

// CheckSpokeClusterType checks and returns a spoke cluster type based on number of control plane nodes.
func CheckSpokeClusterType(client *clients.Settings) (ranparam.ClusterType, error) {
	controlPlaneNodesList, err := ListNodesByLabel(client, RANConfig.ControlPlaneLabelMap)
	if err != nil {
		return "", err
	}

	if len(controlPlaneNodesList) == 1 {
		return ranparam.SNOCluster, nil
	}

	return ranparam.HighlyAvailableCluster, fmt.Errorf("could not determine spoke cluster type")
}

// DoesClusterLabelExist looks for a label on a managed cluster and returns true if it exists.
func DoesClusterLabelExist(client *clients.Settings, clusterName string, label string) (bool, error) {
	managedCluster, err := ocm.PullManagedCluster(client, clusterName)
	if err != nil {
		return false, err
	}

	_, exists := managedCluster.Object.Labels[label]

	return exists, nil
}

// GetManagedClusterID gets the cluster ID from the managed cluster labels. It returns an error if the managed cluster
// or its ID could not be found.
func GetManagedClusterID(client *clients.Settings, clusterName string) (string, error) {
	managedCluster, err := ocm.PullManagedCluster(client, clusterName)
	if err != nil {
		return "", fmt.Errorf("failed to pull managed cluster: %w", err)
	}

	clusterID := managedCluster.Object.GetLabels()["clusterID"]
	if clusterID == "" {
		return "", fmt.Errorf("cluster ID not found in managed cluster labels")
	}

	return clusterID, nil
}

// PowerOffAndWait will trigger a power off and poll every 30 seconds for up to 3 minutes until the system is off.
func PowerOffAndWait(bmcClient *bmc.BMC) error {
	err := bmcClient.SystemPowerOff()
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to trigger system power off: %v", err)

		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), 30*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			powerState, err := bmcClient.SystemPowerState()
			if err != nil {
				klog.V(ranparam.LogLevel).Infof("Failed to get system power state: %v", err)

				return false, err
			}

			if powerState != "Off" {
				klog.V(ranparam.LogLevel).Infof("System power state is not Off: %s", powerState)

				return false, nil
			}

			return true, nil
		})
}

// PowerOffWithRetries will attempt to power off and wait until the system is powered off, trying up to retries times if
// the system does not power off.
func PowerOffWithRetries(bmcClient *bmc.BMC, retries uint) error {
	var err error

	for retry := range retries {
		err = PowerOffAndWait(bmcClient)
		if err == nil {
			return nil
		}

		klog.V(ranparam.LogLevel).Infof("Powering off failed with %d retries left: %v", retries-retry-1, err)
	}

	return err
}

// PowerOnAndWait will trigger a power on and poll every 30 seconds for up to 3 minutes until the system is on.
func PowerOnAndWait(bmcClient *bmc.BMC) error {
	err := bmcClient.SystemPowerOn()
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to trigger system power off: %v", err)

		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), 30*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			powerState, err := bmcClient.SystemPowerState()
			if err != nil {
				klog.V(ranparam.LogLevel).Infof("Failed to get system power state: %v", err)

				return false, err
			}

			if powerState != "On" {
				klog.V(ranparam.LogLevel).Infof("System power state is not On: %s", powerState)

				return false, nil
			}

			return true, nil
		})
}

// PowerOnWithRetries will attempt to power on and wait until the system is powered on, trying up to retries times if
// the system does not power on.
func PowerOnWithRetries(bmcClient *bmc.BMC, retries uint) error {
	var err error

	for retry := range retries {
		err = PowerOnAndWait(bmcClient)
		if err == nil {
			return nil
		}

		klog.V(ranparam.LogLevel).Infof("Powering on failed with %d retries left: %v", retries-retry-1, err)
	}

	return err
}
