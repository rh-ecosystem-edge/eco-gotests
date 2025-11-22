package await

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lso"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitUntilAllDeploymentsReady waits for the duration of the defined timeout or until all deployments
// in the namespace reach the Ready condition.
func WaitUntilAllDeploymentsReady(apiClient *clients.Settings, nsname string, timeout time.Duration) (bool, error) {
	deployments, err := deployment.List(apiClient, nsname, metav1.ListOptions{})
	if err != nil {
		klog.V(100).Infof("deployment list error: %s", err)

		return false, err
	}

	for _, testDeployment := range deployments {
		if !testDeployment.IsReady(timeout) {
			return false, fmt.Errorf("deployment %s not ready in time. available replicas: %d",
				testDeployment.Definition.Name, testDeployment.Object.Status.AvailableReplicas)
		}
	}

	return true, nil
}

// WaitUntilDeploymentReady waits for the duration of the defined timeout or until specific deployment
// in the namespace reach the Ready condition.
func WaitUntilDeploymentReady(apiClient *clients.Settings, name, nsname string, timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*2,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			irDeployment, err := deployment.Pull(apiClient, name, nsname)
			if err != nil {
				return false, nil
			}

			isReady := irDeployment.IsReady(time.Second * 2)

			if isReady {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		irDeployment, err := deployment.Pull(apiClient, name, nsname)
		if err != nil {
			klog.V(100).Infof("deployment %s in namespace %s not exists; %v", name, nsname, err)

			return err
		}

		klog.V(100).Infof("deployment %s in namespace %s not ready in time. available replicas: %d",
			irDeployment.Definition.Name,
			irDeployment.Definition.Namespace,
			irDeployment.Object.Status.AvailableReplicas)

		return err
	}

	return nil
}

// WaitUntilAllStatefulSetsReady waits for the duration of the defined timeout or until all deployments
// in the namespace reach the Ready condition.
func WaitUntilAllStatefulSetsReady(apiClient *clients.Settings, nsname string, timeout time.Duration) (bool, error) {
	statefulsets, err := statefulset.List(apiClient, nsname, metav1.ListOptions{})
	if err != nil {
		klog.V(100).Infof("statefulsets list error: %s", err)

		return false, err
	}

	for _, testStatefulset := range statefulsets {
		if !testStatefulset.IsReady(timeout) {
			return false, fmt.Errorf("statefulset %s not ready in time. available replicas: %d",
				testStatefulset.Definition.Name, testStatefulset.Object.Status.AvailableReplicas)
		}
	}

	return true, nil
}

// WaitUntilAllPodsReady waits for the duration of the defined timeout or until all deployments
// in the namespace reach the Ready condition.
func WaitUntilAllPodsReady(apiClient *clients.Settings, nsname string, timeout time.Duration) (bool, error) {
	pods, err := pod.List(apiClient, nsname, metav1.ListOptions{})
	if err != nil {
		klog.V(100).Infof("pods list error: %s", err)

		return false, err
	}

	for _, testPod := range pods {
		err = testPod.WaitUntilReady(timeout)
		if err != nil {
			klog.V(100).Infof("pod %s did not become ready in time: %s", testPod.Object.Name, err)

			return false, err
		}
	}

	return true, nil
}

// WaitUntilNodeIsUnreachable waits until a hostname is not reachble via its IPv4 or IPv6 address.
func WaitUntilNodeIsUnreachable(hostname string, timeout time.Duration) error {
	ip4Unreachable := false
	ip6Unreachable := false
	// Get the IPv4 and IPv6 addresses of the node
	ipv4Addr, err := net.ResolveIPAddr("ip4", hostname)
	if err != nil {
		ip4Unreachable = true
	}

	ipv6Addr, err := net.ResolveIPAddr("ip6", hostname)
	if err != nil {
		ip6Unreachable = true
	}

	// Set the timeout for the function
	deadline := time.Now().Add(timeout)

	// Wait for the node to become unreachable
	for {
		// Ping the node's IPv4 address
		if !ip4Unreachable {
			cmd := exec.Command("ping", "-c", "1", ipv4Addr.String())

			_, err := cmd.CombinedOutput()
			if err != nil {
				ip4Unreachable = true
			}
		}

		// Ping the node's IPv6 address
		if !ip6Unreachable {
			cmd := exec.Command("ping", "-c", "1", ipv6Addr.String())

			_, err = cmd.CombinedOutput()
			if err != nil {
				ip6Unreachable = true
			}
		}

		if (ip4Unreachable) && (ip6Unreachable) {
			klog.V(100).Infof("Node is unreachable.")

			return nil
		}

		// If both pings succeed, the node is still reachable
		klog.V(100).Infof("Node is still reachable. Waiting...")

		// Check if the deadline has been reached
		if time.Now().After(deadline) {
			// If the timeout is reached, the node is considered to be reachable
			return fmt.Errorf("node has not become unreachable in time")
		}

		// Wait for a second
		time.Sleep(time.Second)
	}
}

// WaitUntilDaemonSetIsRunning waits until the new daemonset is in Ready state.
func WaitUntilDaemonSetIsRunning(apiClient *clients.Settings, name, nsname string, timeout time.Duration) error {
	klog.V(90).Infof("Verifying if daemonset %s is running in namespace %s", name, nsname)

	var daemonsetObj *daemonset.Builder

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			daemonsetObj, err = daemonset.Pull(apiClient, name, nsname)
			if err != nil {
				klog.V(90).Infof("Error to pull daemonset %s in namespace %s, retry", name, nsname)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting until the new daemonset %s in namespace %s is in Ready state.", name, nsname)

	if daemonsetObj.IsReady(timeout) {
		return nil
	}

	return fmt.Errorf("daemonSet %s in namespace %s is not ready", name, nsname)
}

// WaitUntilDaemonSetDeleted waits until the daemonset is deleted.
func WaitUntilDaemonSetDeleted(apiClient *clients.Settings, name, nsname string, timeout time.Duration) error {
	klog.V(90).Infof("Wait until daemonset %s in namespace %s is deleted", name, nsname)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := daemonset.Pull(apiClient, name, nsname)
			if err == nil {
				klog.V(90).Infof("daemonset %s in namespace %s still exists, retry", name, nsname)

				return false, nil
			}

			return true, nil
		})
	if err == nil {
		return nil
	}

	return fmt.Errorf("daemonSet %s in namespace %s is not deleted during timeout %v", name, nsname, timeout)
}

// WaitUntilConfigMapCreated waits until the configMap is created.
func WaitUntilConfigMapCreated(apiClient *clients.Settings, name, nsname string, timeout time.Duration) error {
	klog.V(90).Infof("Wait until configMap %s in namespace %s is created", name, nsname)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := configmap.Pull(apiClient, name, nsname)
			if err != nil {
				klog.V(90).Infof("configMap %s in namespace %s not created yet, retry", name, nsname)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("configMap %s in namespace %s is not created during timeout %v; %w",
			name, nsname, timeout, err)
	}

	return nil
}

// WaitForThePodReplicasCountInNamespace waiting for the specific pod replicas count in
// the namespace that match options.
func WaitForThePodReplicasCountInNamespace(
	apiClient *clients.Settings,
	nsname string,
	options metav1.ListOptions,
	replicasCount int,
	timeout time.Duration,
) (bool, error) {
	klog.V(100).Infof("Waiting for %d pod replicas count in namespace %s with options %v"+
		" are in running state", replicasCount, nsname, options)

	if nsname == "" {
		klog.V(100).Infof("'nsname' parameter can not be empty")

		return false, fmt.Errorf("failed to list pods, 'nsname' parameter is empty")
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			podsList, err := pod.List(apiClient, nsname, options)
			if err != nil {
				klog.V(100).Infof("Failed to list all pods due to %s", err.Error())

				return false, nil
			}

			if len(podsList) != replicasCount {
				klog.V(100).Infof("pod replicas count not equal to the expected: "+
					"current %d, expected: %d", len(podsList), replicasCount)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return false, err
	}

	return true, nil
}

// WaitUntilPersistentVolumeCreated waits until required count of the persistentVolumes are created.
func WaitUntilPersistentVolumeCreated(apiClient *clients.Settings,
	pvCnt int,
	timeout time.Duration,
	options ...metav1.ListOptions) error {
	klog.V(90).Infof("Wait until %d persistentVolumes with option %v are created", pvCnt, options)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pvList, err := storage.ListPV(apiClient, metav1.ListOptions{})
			if err != nil {
				klog.V(90).Infof("no persistentVolumes with option %v was found, retry", options)

				return false, nil
			}

			if len(pvList) < pvCnt {
				klog.V(90).Infof("persistentVolumes count not equal to the expected: %d; found: %d",
					pvCnt, len(pvList))

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("persistentVolumes with option %v were not found or count is not as expected %d; %w",
			options, timeout, err)
	}

	return nil
}

// WaitUntilPersistentVolumeClaimCreated waits until required count of the persistentVolumeClaims are created.
func WaitUntilPersistentVolumeClaimCreated(apiClient *clients.Settings,
	nsname string,
	pvcCnt int,
	timeout time.Duration,
	options ...metav1.ListOptions) error {
	klog.V(90).Infof("Wait until %d persistentVolumeClaims with option %v are created", pvcCnt, options)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pvList, err := storage.ListPVC(apiClient, nsname, metav1.ListOptions{})
			if err != nil {
				klog.V(90).Infof("no persistentVolumeClaims with option %v was found, retry", options)

				return false, nil
			}

			if len(pvList) < pvcCnt {
				klog.V(90).Infof("persistentVolumeClaims count not equal to the expected: %d; found: %d",
					pvcCnt, len(pvList))

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("persistentVolumeClaims with option %v were not found or count is not as expected %d; %w",
			options, timeout, err)
	}

	return nil
}

// WaitUntilLVDIsDiscovering waits until the localVolumeDiscovery is Discovering.
func WaitUntilLVDIsDiscovering(apiClient *clients.Settings,
	lvdName string,
	nsname string,
	timeout time.Duration) error {
	klog.V(90).Infof("Wait until localVolumeDiscovery %s from namespace %s is Discovering", lvdName, nsname)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			lvdObj, err := lso.PullLocalVolumeDiscovery(apiClient, lvdName, nsname)
			if err != nil {
				klog.V(90).Infof("no localVolumeDiscovery %s found in namespace %s, retry", lvdName, nsname)

				return false, nil
			}

			if lvdObj.Object.Status.Phase != "Discovering" {
				klog.V(90).Infof("localVolumeDiscovery %s in namespace %s phase not as expected yet, retry",
					lvdName, nsname)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("localVolumeDiscovery %s in namespace %s phase not as expected after %v; %w",
			lvdName, nsname, timeout, err)
	}

	return nil
}
