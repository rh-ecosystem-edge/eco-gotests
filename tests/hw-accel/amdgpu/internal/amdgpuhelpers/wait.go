package amdgpuhelpers

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// WaitForClusterStabilityAfterDeviceConfig waits for the cluster to stabilize after DeviceConfig creation.
func WaitForClusterStabilityAfterDeviceConfig(apiClients *clients.Settings) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster stability after DeviceConfig creation")

	WaitForClusterStabilityErr := WaitForClusterStability(apiClients)
	if WaitForClusterStabilityErr != nil {
		return fmt.Errorf("cluster stability check after DeviceConfig creation failed: %w", WaitForClusterStabilityErr)
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster is stable after DeviceConfig creation")

	return nil
}

// WaitForClusterStability waits for the cluster to stabilize.
func WaitForClusterStability(apiClients *clients.Settings) error {
	time.Sleep(2 * time.Minute)

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for all nodes to be ready")

	err := wait.PollUntilContextTimeout(context.Background(),
		30*time.Second, 15*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			allNodes, err := nodes.List(apiClients, metav1.ListOptions{})
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("failed to list nodes: %v", err)

				return false, nil
			}

			allReady := true

			for _, node := range allNodes {
				nodeBuilder, nodeErr := nodes.Pull(apiClients, node.Object.Name)
				if nodeErr != nil {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("Error pulling node %s: %v", node.Object.Name, nodeErr)

					allReady = false

					continue
				}

				ready := false

				for _, condition := range nodeBuilder.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						ready = true

						break
					}
				}

				if !ready {
					allReady = false

					glog.V(amdgpuparams.AMDGPULogLevel).Infof("Node %s is not ready yet", node.Object.Name)
				} else {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("Node %s is ready", node.Object.Name)
				}
			}

			return allReady, nil
		})

	if err != nil {
		return fmt.Errorf("nodes are not ready: %w", err)
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("All nodes are ready, waiting 3 minutes for cluster stabilization")
	time.Sleep(1 * time.Minute)

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for all cluster operators to be available")

	available, err := clusteroperator.WaitForAllClusteroperatorsAvailable(apiClients, 15*time.Minute, metav1.ListOptions{})

	if err != nil {
		return fmt.Errorf("cluster operators availability check failed: %w", err)
	}

	if !available {
		return fmt.Errorf("some cluster operators are not available")
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster stability check passed")

	return nil
}
