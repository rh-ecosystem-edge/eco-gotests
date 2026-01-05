package amdgpuhelpers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"
)

// WaitForClusterStabilityAfterDeviceConfig waits for the cluster to stabilize after DeviceConfig creation.
func WaitForClusterStabilityAfterDeviceConfig(apiClients *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster stability after DeviceConfig creation")

	WaitForClusterStabilityErr := WaitForClusterStability(apiClients, amdgpuparams.ClusterStabilityTimeout)
	if WaitForClusterStabilityErr != nil {
		return fmt.Errorf("cluster stability check after DeviceConfig creation failed: %w", WaitForClusterStabilityErr)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster is stable after DeviceConfig creation")

	return nil
}

// WaitForClusterStability efficiently waits for all nodes to be ready and all cluster operators to be stable.
func WaitForClusterStability(
	apiClients *clients.Settings,
	timeout time.Duration) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster to stabilize...")

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

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Cluster stability check passed.")

	return nil
}

func waitForNodesReadiness(ctx context.Context, wg *sync.WaitGroup, apiClients *clients.Settings, errChan chan error) {
	defer wg.Done()

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Setting up watch for Node readiness...")

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
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to list nodes during watch: %v", err)

				return false, nil
			}

			for _, node := range nodeList {
				if ready, err := node.IsReady(); err != nil || !ready {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof("Node %s is not ready yet error:%v", node.Object.Name, err)

					return false, nil
				}
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("All nodes are ready.")

			return true, nil
		},
	)
	if err != nil {
		errChan <- fmt.Errorf("failed waiting for nodes to become ready: %w", err)
	}
}

func waitForClientConfig(ctx context.Context, wg *sync.WaitGroup, apiClients *clients.Settings, errChan chan error) {
	defer wg.Done()

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Setting up watch for ClusterOperator stability...")

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
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to list clusteroperators during watch: %v", err)

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
					klog.V(amdgpuparams.AMDGPULogLevel).Infof("ClusterOperator %s is not stable yet", configClient.Name)

					return false, nil
				}
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("✅ All cluster operators are stable.")

			return true, nil
		},
	)
	if err != nil {
		errChan <- fmt.Errorf("failed waiting for clusteroperators to become stable: %w", err)
	}
}

// isConnectionError checks if the error is a connection-related error.
// These errors typically occur when the API server is unavailable during node reboot.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for common connection error patterns
	connectionPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"network is unreachable",
		"EOF",
		"context deadline exceeded",
		"TLS handshake timeout",
		"dial tcp",
		"connect: connection timed out",
	}

	for _, pattern := range connectionPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	// Check for net.Error interface (timeout errors)
	var netErr net.Error

	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

// WaitForPodRunningResilient waits for a pod to be in Running state with resilient retry logic.
// It handles connection errors that may occur during SNO node reboots by retrying with backoff.
// Parameters:
//   - podBuilder: The pod to wait for
//   - timeout: Total timeout for the operation
//   - isSNO: Whether this is a Single Node OpenShift environment (uses longer timeouts)
func WaitForPodRunningResilient(podBuilder *pod.Builder, timeout time.Duration, isSNO bool) error {
	if isSNO {
		klog.V(amdgpuparams.AMDGPULogLevel).Info("SNO environment detected - using extended timeout for pod running check")

		if timeout < amdgpuparams.SNOPodRunningTimeout {
			timeout = amdgpuparams.SNOPodRunningTimeout
		}
	}

	startTime := time.Now()
	retryCount := 0
	lastErr := fmt.Errorf("no attempts made")

	for time.Since(startTime) < timeout {
		// Try to wait for pod to be running with a shorter sub-timeout
		subTimeout := amdgpuparams.DefaultTimeout
		if isSNO {
			subTimeout = 2 * time.Minute // Shorter sub-timeout for individual checks
		}

		err := podBuilder.WaitUntilRunning(subTimeout)
		if err == nil {
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Pod %s is now running after %d retries",
				podBuilder.Object.Name, retryCount)

			return nil
		}

		lastErr = err

		// Check if this is a connection error (node might be rebooting)
		if isConnectionError(err) {
			retryCount++
			klog.V(amdgpuparams.AMDGPULogLevel).Infof(
				"Connection error while waiting for pod %s (retry %d/%d): %v. "+
					"Node may be rebooting, will retry in %v",
				podBuilder.Object.Name, retryCount, amdgpuparams.MaxConnectionRetries,
				err, amdgpuparams.ConnectionRetryInterval)

			if retryCount >= amdgpuparams.MaxConnectionRetries {
				return fmt.Errorf("max retries (%d) exceeded waiting for pod %s: %w",
					amdgpuparams.MaxConnectionRetries, podBuilder.Object.Name, err)
			}

			// Wait before retrying
			time.Sleep(amdgpuparams.ConnectionRetryInterval)

			continue
		}

		// For non-connection errors, check if pod exists and is in an error state
		klog.V(amdgpuparams.AMDGPULogLevel).Infof(
			"Non-connection error waiting for pod %s: %v. Will retry...",
			podBuilder.Object.Name, err)

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout (%v) waiting for pod %s to be running: %w",
		timeout, podBuilder.Object.Name, lastErr)
}

// WaitForPodsRunningResilient waits for multiple pods to be in Running state with resilient retry logic.
// It first waits for cluster stability (if SNO) then waits for each pod.
func WaitForPodsRunningResilient(
	apiClient *clients.Settings,
	podBuilders []*pod.Builder,
	isSNO bool,
) error {
	if len(podBuilders) == 0 {
		return fmt.Errorf("no pods provided to wait for")
	}

	// For SNO, first wait for cluster stability since node may have rebooted
	if isSNO {
		klog.V(amdgpuparams.AMDGPULogLevel).Info(
			"SNO environment: waiting for cluster stability before checking pods")

		err := WaitForClusterStability(apiClient, amdgpuparams.SNOClusterStabilityTimeout)
		if err != nil {
			klog.V(amdgpuparams.AMDGPULogLevel).Infof(
				"Cluster stability check had issues (continuing anyway): %v", err)
		}
	}

	// Wait for each pod with resilient retry
	for _, podBuilder := range podBuilders {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof(
			"Waiting for pod %s to be running (SNO=%t)", podBuilder.Object.Name, isSNO)

		timeout := amdgpuparams.DefaultTimeout
		if isSNO {
			timeout = amdgpuparams.SNOPodRunningTimeout
		}

		err := WaitForPodRunningResilient(podBuilder, timeout, isSNO)
		if err != nil {
			return fmt.Errorf("failed waiting for pod %s: %w", podBuilder.Object.Name, err)
		}
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("All pods are now running")

	return nil
}

// WaitForClusterStabilityAfterNodeLabeller waits for cluster stability after Node Labeller is enabled.
// This is important for SNO where enabling Node Labeller may trigger driver loading and potential reboot.
func WaitForClusterStabilityAfterNodeLabeller(apiClient *clients.Settings, isSNO bool) error {
	timeout := amdgpuparams.ClusterStabilityTimeout

	if isSNO {
		klog.V(amdgpuparams.AMDGPULogLevel).Info(
			"SNO environment detected - using extended timeout for cluster stability after Node Labeller")

		timeout = amdgpuparams.SNOClusterStabilityTimeout
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for cluster stability after enabling Node Labeller")

	return WaitForClusterStability(apiClient, timeout)
}

// WaitForAMDGPUDriverReady waits for the AMD GPU driver to be built and loaded by KMM.
// This checks for KMM build pods to complete and driver-container pods to be running.
func WaitForAMDGPUDriverReady(apiClient *clients.Settings, isSNO bool) error {
	timeout := amdgpuparams.ClusterStabilityTimeout
	if isSNO {
		timeout = amdgpuparams.SNOClusterStabilityTimeout
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Infof(
		"Waiting for AMD GPU driver to be ready (timeout: %v)...", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// First, wait for any KMM build pods to complete
	err := waitForKMMBuildPodsComplete(ctx, apiClient)
	if err != nil {
		return fmt.Errorf("KMM build pods did not complete: %w", err)
	}

	// Then, wait for driver-container pods to be running
	err = waitForDriverContainerPods(ctx, apiClient, isSNO)
	if err != nil {
		return fmt.Errorf("driver-container pods not ready: %w", err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("✅ AMD GPU driver is ready")

	return nil
}

// waitForKMMBuildPodsComplete waits for KMM build pods to complete (succeed or no longer exist).
func waitForKMMBuildPodsComplete(ctx context.Context, apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Checking for KMM build pods...")

	retryInterval := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Look for build pods in the AMD GPU namespace
			podList, err := apiClient.CoreV1Interface.Pods(amdgpuparams.AMDGPUNamespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				if isConnectionError(err) {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"Connection error checking build pods (retrying): %v", err)
					time.Sleep(retryInterval)

					continue
				}

				return fmt.Errorf("failed to list pods: %w", err)
			}

			buildPodsFound := false
			buildPodsCompleted := true

			for i := range podList.Items {
				podItem := &podList.Items[i]
				// Look for build pods (typically have "build" in the name)
				if strings.Contains(podItem.Name, "build") {
					buildPodsFound = true
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"Found build pod: %s, phase: %s", podItem.Name, podItem.Status.Phase)

					if podItem.Status.Phase == corev1.PodRunning ||
						podItem.Status.Phase == corev1.PodPending {
						buildPodsCompleted = false
					} else if podItem.Status.Phase == corev1.PodFailed {
						return fmt.Errorf("build pod %s failed", podItem.Name)
					}
				}
			}

			if !buildPodsFound {
				klog.V(amdgpuparams.AMDGPULogLevel).Info(
					"No build pods found - driver may already be built or using pre-built image")

				return nil
			}

			if buildPodsCompleted {
				klog.V(amdgpuparams.AMDGPULogLevel).Info("✅ All KMM build pods completed successfully")

				return nil
			}

			klog.V(amdgpuparams.AMDGPULogLevel).Info("Build pods still running, waiting...")
			time.Sleep(retryInterval)
		}
	}
}

// waitForDriverContainerPods waits for device-plugin pods to be running on AMD GPU nodes.
// The AMD GPU operator creates device-plugin pods (not driver-container) after the driver is loaded.
func waitForDriverContainerPods(ctx context.Context, apiClient *clients.Settings, isSNO bool) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for device-plugin pods to be running...")

	retryInterval := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Look for device-plugin or node-labeller pods in the AMD GPU namespace
			podList, err := apiClient.CoreV1Interface.Pods(amdgpuparams.AMDGPUNamespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				if isConnectionError(err) {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"Connection error checking device-plugin pods (retrying): %v", err)
					time.Sleep(retryInterval)

					continue
				}

				return fmt.Errorf("failed to list pods: %w", err)
			}

			devicePluginFound := false
			allPodsRunning := true
			runningCount := 0

			for i := range podList.Items {
				podItem := &podList.Items[i]
				// Look for device-plugin pods (AMD GPU operator creates these after driver is loaded)
				// Also check for node-labeller as it indicates driver is ready
				if strings.Contains(podItem.Name, "device-plugin") ||
					(strings.Contains(podItem.Name, "node-labeller") && !strings.Contains(podItem.Name, "build")) {
					devicePluginFound = true
					if podItem.Status.Phase == corev1.PodRunning {
						runningCount++
					} else {
						klog.V(amdgpuparams.AMDGPULogLevel).Infof(
							"Pod %s is %s", podItem.Name, podItem.Status.Phase)
						allPodsRunning = false
					}
				}
			}

			if devicePluginFound && allPodsRunning && runningCount > 0 {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof(
					"✅ Driver is ready - %d device-plugin/node-labeller pods running", runningCount)

				return nil
			}

			if !devicePluginFound {
				klog.V(amdgpuparams.AMDGPULogLevel).Info(
					"No device-plugin pods found yet, waiting for driver to be loaded...")
			}

			time.Sleep(retryInterval)
		}
	}
}

// VerifyGPUHardwareReady checks if the AMD GPU hardware is properly initialized.
// This validates that the driver loaded successfully by checking node capacity.
// Returns an error with detailed info if GPU is not usable (e.g., PCI passthrough issues).
func VerifyGPUHardwareReady(apiClient *clients.Settings, nodeName string) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Verifying GPU hardware is ready on node %s", nodeName)

	// Check if node has amd.com/gpu capacity > 0
	node, err := apiClient.CoreV1Interface.Nodes().Get(
		context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	gpuCapacity, exists := node.Status.Capacity["amd.com/gpu"]
	if !exists {
		return fmt.Errorf("node %s does not have amd.com/gpu capacity - "+
			"driver may have failed to initialize. Check dmesg for 'amdgpu: Fatal error during GPU init' "+
			"which indicates PCI passthrough reset issues", nodeName)
	}

	gpuCount := gpuCapacity.Value()
	if gpuCount == 0 {
		return fmt.Errorf("node %s has 0 AMD GPUs - device plugin reports no usable GPUs", nodeName)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Infof("✅ Node %s has %d AMD GPU(s) available", nodeName, gpuCount)

	return nil
}

// WaitForNodeLabellerDriverInit waits for the Node Labeller's driver-init container to complete.
// The driver-init container waits for the amdgpu kernel module to be loaded before the main
// node-labeller container can start and apply labels.
// This is critical because KMM driver build + load can take 10-20 minutes.
func WaitForNodeLabellerDriverInit(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Infof(
		"Waiting for Node Labeller driver-init containers to complete (timeout: %v)...", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	retryInterval := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for driver-init containers to complete")
		default:
			podList, err := apiClient.CoreV1Interface.Pods(amdgpuparams.AMDGPUNamespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				if isConnectionError(err) {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"Connection error checking pods (retrying): %v", err)
					time.Sleep(retryInterval)

					continue
				}

				return fmt.Errorf("failed to list pods: %w", err)
			}

			allInitContainersReady := true
			nodeLabellerFound := false

			for i := range podList.Items {
				podItem := &podList.Items[i]

				// Look for node-labeller pods
				if !strings.Contains(podItem.Name, "node-labeller") {
					continue
				}

				nodeLabellerFound = true

				// Check init container statuses
				for _, initStatus := range podItem.Status.InitContainerStatuses {
					if initStatus.Name == "driver-init" {
						if initStatus.State.Terminated != nil {
							if initStatus.State.Terminated.ExitCode == 0 {
								klog.V(amdgpuparams.AMDGPULogLevel).Infof(
									"✅ driver-init completed for pod %s", podItem.Name)
							} else {
								return fmt.Errorf("driver-init failed for pod %s with exit code %d",
									podItem.Name, initStatus.State.Terminated.ExitCode)
							}
						} else if initStatus.State.Running != nil {
							klog.V(amdgpuparams.AMDGPULogLevel).Infof(
								"driver-init still running for pod %s (waiting for amdgpu module)...",
								podItem.Name)
							allInitContainersReady = false
						} else if initStatus.State.Waiting != nil {
							klog.V(amdgpuparams.AMDGPULogLevel).Infof(
								"driver-init waiting for pod %s: %s",
								podItem.Name, initStatus.State.Waiting.Reason)
							allInitContainersReady = false
						}
					}
				}

				// Also check if the main container is running (means init completed)
				for _, containerStatus := range podItem.Status.ContainerStatuses {
					if containerStatus.Name == "node-labeller-container" && containerStatus.Ready {
						klog.V(amdgpuparams.AMDGPULogLevel).Infof(
							"✅ node-labeller-container is ready for pod %s", podItem.Name)
					}
				}
			}

			if !nodeLabellerFound {
				klog.V(amdgpuparams.AMDGPULogLevel).Info(
					"No node-labeller pods found yet, waiting...")
				time.Sleep(retryInterval)

				continue
			}

			if allInitContainersReady {
				klog.V(amdgpuparams.AMDGPULogLevel).Info(
					"✅ All Node Labeller driver-init containers completed")

				// Add extra 30 seconds for labels to be applied after init completes
				klog.V(amdgpuparams.AMDGPULogLevel).Info(
					"Waiting 30s for Node Labeller to apply labels...")
				time.Sleep(30 * time.Second)

				return nil
			}

			time.Sleep(retryInterval)
		}
	}
}
