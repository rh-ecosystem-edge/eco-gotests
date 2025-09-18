package amdgpuhelpers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

// WaitForClusterStabilityAfterDeviceConfig waits for the cluster to stabilize after DeviceConfig creation.
func WaitForClusterStabilityAfterDeviceConfig(apiClients *clients.Settings) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster stability after DeviceConfig creation")

	WaitForClusterStabilityErr := WaitForClusterStability(apiClients, amdgpuparams.ClusterStabilityTimeout)
	if WaitForClusterStabilityErr != nil {
		return fmt.Errorf("cluster stability check after DeviceConfig creation failed: %w", WaitForClusterStabilityErr)
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster is stable after DeviceConfig creation")

	return nil
}

// WaitForClusterStability efficiently waits for all nodes to be ready and all cluster operators to be stable.
func WaitForClusterStability(
	apiClients *clients.Settings,
	timeout time.Duration) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster to stabilize...")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var waitgroup sync.WaitGroup

	errChan := make(chan error, 2)

	waitgroup.Add(1)

	waitForNodesReadinessFunc := func() {
		waitForNodesReadiness(ctx, &waitgroup, apiClients, errChan)
	}

	go waitForNodesReadinessFunc()

	waitgroup.Add(1)

	waitForClientConfigFunc := func() {
		waitForClientConfig(ctx, &waitgroup, apiClients, errChan)
	}

	go waitForClientConfigFunc()

	waitgroup.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster stability check passed.")

	return nil
}

func waitForNodesReadiness(ctx context.Context, wg *sync.WaitGroup, apiClients *clients.Settings, errChan chan error) {
	defer wg.Done()
	glog.V(amdgpuparams.AMDGPULogLevel).Info("Setting up watch for Node readiness...")

	_, err := watchtools.UntilWithSync(ctx,

		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return apiClients.CoreV1Interface.Nodes().List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return apiClients.CoreV1Interface.Nodes().Watch(ctx, options)
			},
		},

		&corev1.Node{},

		nil,

		func(event watch.Event) (bool, error) {
			nodeList, err := nodes.List(apiClients, metav1.ListOptions{})
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to list nodes during watch: %v", err)

				return false, nil
			}

			for _, node := range nodeList {
				if ready, err := node.IsReady(); err != nil || !ready {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("Node %s is not ready yet error:%v", node.Object.Name, err)

					return false, nil
				}
			}

			glog.V(amdgpuparams.AMDGPULogLevel).Info("All nodes are ready.")

			return true, nil
		},
	)
	if err != nil {
		errChan <- fmt.Errorf("failed waiting for nodes to become ready: %w", err)
	}
}

func waitForClientConfig(ctx context.Context, wg *sync.WaitGroup, apiClients *clients.Settings, errChan chan error) {
	defer wg.Done()
	glog.V(amdgpuparams.AMDGPULogLevel).Info("Setting up watch for ClusterOperator stability...")

	configClient, err := configclient.NewForConfig(apiClients.Config)

	if err != nil {
		errChan <- fmt.Errorf("could not create config client: %w", err)

		return
	}

	_, err = watchtools.UntilWithSync(ctx,
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return configClient.ConfigV1().ClusterOperators().List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return configClient.ConfigV1().ClusterOperators().Watch(ctx, options)
			},
		},
		&configv1.ClusterOperator{},
		nil,
		func(event watch.Event) (bool, error) {
			coList, err := configClient.ConfigV1().ClusterOperators().List(ctx, metav1.ListOptions{})
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to list clusteroperators during watch: %v", err)

				return false, nil
			}

			for _, configClient := range coList.Items {
				isAvailable := false
				isProgressing := true
				isDegraded := true

				for _, condition := range configClient.Status.Conditions {
					if condition.Type == configv1.OperatorAvailable && condition.Status == configv1.ConditionTrue {
						isAvailable = true
					}

					if condition.Type == configv1.OperatorProgressing && condition.Status == configv1.ConditionFalse {
						isProgressing = false
					}

					if condition.Type == configv1.OperatorDegraded && condition.Status == configv1.ConditionFalse {
						isDegraded = false
					}
				}

				if !isAvailable || isProgressing || isDegraded {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("ClusterOperator %s is not stable yet", configClient.Name)

					return false, nil
				}
			}

			glog.V(amdgpuparams.AMDGPULogLevel).Info("✅ All cluster operators are stable.")

			return true, nil
		},
	)

	if err != nil {
		errChan <- fmt.Errorf("failed waiting for clusteroperators to become stable: %w", err)
	}
}
