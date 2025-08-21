package amdgpuhelpers

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"

	"github.com/openshift-kni/eco-goinfra/pkg/clusteroperator"

	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	"github.com/openshift-kni/eco-goinfra/pkg/nodes"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// WaitForClusterStabilityAfterDeviceConfig waits for the cluster to stabilize after DeviceConfig creation.
func WaitForClusterStabilityAfterDeviceConfig(apiClients *clients.Settings) error {
	glog.V(amdgpuparams.LogLevel).Info("Waiting for cluster stability after DeviceConfig creation")
	time.Sleep(5 * time.Minute)

	glog.V(amdgpuparams.LogLevel).Info("Waiting for all nodes to be ready")

	err := wait.PollUntilContextTimeout(context.Background(),
		30*time.Second, 15*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			allNodes, err := nodes.List(apiClients, metav1.ListOptions{})
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("failed to list nodes: %v", err)

				return false, nil
			}

			for _, node := range allNodes {
				nodeBuilder, nodeErr := nodes.Pull(apiClients, node.Object.Name)
				if nodeErr != nil {
					glog.V(amdgpuparams.LogLevel).Infof("Error pulling node %s: %v", node.Object.Name, nodeErr)

					return false, nil
				}

				for _, condition := range nodeBuilder.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						glog.V(amdgpuparams.LogLevel).Infof("Node %s is ready", node.Object.Name)

						return true, nil
					}
				}

				glog.V(amdgpuparams.LogLevel).Infof("Node %s is not ready yet", node.Object.Name)
			}

			return false, nil
		})

	if err != nil {
		return fmt.Errorf("nodes are not ready: %w", err)
	}

	glog.V(amdgpuparams.LogLevel).Info("All nodes are ready, waiting 3 minutes for cluster stabilization")
	time.Sleep(3 * time.Minute)

	glog.V(amdgpuparams.LogLevel).Info("Waiting for all cluster operators to be available")

	available, err := clusteroperator.WaitForAllClusteroperatorsAvailable(apiClients, 15*time.Minute, metav1.ListOptions{})

	if err != nil {
		return fmt.Errorf("cluster operators availability check failed: %w", err)
	}

	if !available {
		return fmt.Errorf("some cluster operators are not available")
	}

	glog.V(amdgpuparams.LogLevel).Info("Cluster stability check passed")

	return nil
}
