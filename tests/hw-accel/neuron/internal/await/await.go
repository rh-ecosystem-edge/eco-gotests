package await

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	kmmNamespace            = "openshift-kmm"
	kmmControllerDeployment = "kmm-operator-controller"
	kmmWebhookDeployment    = "kmm-operator-webhook-server"
)

// KMMOperatorReady waits for the KMM controller and webhook deployments to be
// ready. This is useful after changing the KMM Subscription, which can cause
// OLM to roll the operator while a test is reconciling a Module.
func KMMOperatorReady(apiClient *clients.Settings, timeout time.Duration) error {
	return kmmOperatorReady(apiClient, "", "", timeout)
}

// KMMOperatorReadyWithToleration waits for KMM to be ready and for the
// controller pod template to contain the requested toleration. Checking the
// pod template prevents the caller from racing an asynchronous OLM rollout by
// observing the old, still-ready deployment.
func KMMOperatorReadyWithToleration(apiClient *clients.Settings, key string,
	effect corev1.TaintEffect, timeout time.Duration) error {
	return kmmOperatorReady(apiClient, key, effect, timeout)
}

func kmmOperatorReady(apiClient *clients.Settings, tolerationKey string,
	tolerationEffect corev1.TaintEffect, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			for _, deploymentName := range []string{kmmControllerDeployment, kmmWebhookDeployment} {
				kmmDeployment, err := deployment.Pull(apiClient, deploymentName, kmmNamespace)
				if err != nil {
					return false, nil
				}

				if !kmmDeployment.IsReady(10 * time.Second) {
					return false, nil
				}

				if deploymentName == kmmControllerDeployment && tolerationKey != "" {
					hasToleration := false

					for _, toleration := range kmmDeployment.Definition.Spec.Template.Spec.Tolerations {
						if toleration.Key == tolerationKey && toleration.Effect == tolerationEffect {
							hasToleration = true

							break
						}
					}

					if !hasToleration {
						return false, nil
					}
				}
			}

			return true, nil
		})
}

// DeviceConfigDriverUpdated waits until DeviceConfig contains the requested
// driver version and, when supplied, the requested driver image.
func DeviceConfigDriverUpdated(apiClient *clients.Settings, name, namespace,
	expectedVersion, expectedImage string, timeout time.Duration) error {
	var observedVersion, observedImage string

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			deviceConfig, err := neuron.Pull(apiClient, name, namespace)
			if err != nil {
				return false, nil
			}

			observedVersion = deviceConfig.Definition.Spec.DriverVersion
			observedImage = deviceConfig.Definition.Spec.DriversImage

			return observedVersion == expectedVersion &&
				(expectedImage == "" || observedImage == expectedImage), nil
		})
	if err != nil {
		return fmt.Errorf(
			"DeviceConfig %s in namespace %s did not reach driver version %q and image %q "+
				"(observed version %q, image %q): %w",
			name, namespace, expectedVersion, expectedImage, observedVersion, observedImage, err)
	}

	return nil
}

// ModuleDriverImageUpdated waits until KMM's Module references the requested
// driver image. KMM appends the node kernel version to pre-built driver images.
func ModuleDriverImageUpdated(apiClient *clients.Settings, name, namespace,
	expectedImage string, timeout time.Duration) error {
	var observedImage string

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			module, err := kmm.Pull(apiClient, name, namespace)
			if err != nil || module.Definition.Spec.ModuleLoader == nil ||
				len(module.Definition.Spec.ModuleLoader.Container.KernelMappings) == 0 {
				return false, nil
			}

			observedImage = module.Definition.Spec.ModuleLoader.Container.KernelMappings[0].ContainerImage

			return observedImage == expectedImage ||
				strings.HasPrefix(observedImage, expectedImage+"-"), nil
		})
	if err != nil {
		return fmt.Errorf(
			"Module %s in namespace %s did not reach driver image %q (observed %q): %w",
			name, namespace, expectedImage, observedImage, err)
	}

	return nil
}

// DRADeviceConfig waits for a DeviceConfig with a configured DRA driver image.
func DRADeviceConfig(apiClient *clients.Settings, name, namespace string,
	timeout time.Duration) error {
	var lastErr error

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			deviceConfig, getErr := neuron.Pull(apiClient, name, namespace)

			lastErr = getErr
			if lastErr != nil {
				klog.V(params.NeuronLogLevel).Infof(
					"DeviceConfig %s in namespace %s is not available yet: %v",
					name, namespace, lastErr)

				return false, nil
			}

			if deviceConfig.Definition.Spec.DRADriverImage == "" {
				lastErr = fmt.Errorf("DeviceConfig does not have spec.draDriverImage configured")

				return false, nil
			}

			lastErr = nil

			return true, nil
		})
	if err != nil {
		if lastErr != nil {
			return fmt.Errorf(
				"DRA DeviceConfig %s in namespace %s is not ready: %w",
				name, namespace, lastErr)
		}

		return fmt.Errorf(
			"DRA DeviceConfig %s in namespace %s is not ready: %w",
			name, namespace, err)
	}

	return nil
}

// DeviceConfigDeleted waits until the named DeviceConfig no longer exists.
func DeviceConfigDeleted(apiClient *clients.Settings, name, namespace string,
	timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := apiClient.Resource(neuronparams.DeviceConfigGVR).
				Namespace(namespace).
				Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			if err != nil {
				return false, err
			}

			return false, nil
		})
}

// ResourceClaimAllocatedAndReserved waits for a ResourceClaim in the namespace
// to be allocated and reserved for a consumer.
func ResourceClaimAllocatedAndReserved(
	apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			return check.ResourceClaimAllocatedAndReserved(apiClient, namespace)
		})
}

// DRAResourcesAvailable waits until ResourceSlices advertise at least one
// Neuron device on a node.
func DRAResourcesAvailable(apiClient *clients.Settings, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			_, deviceCount, err := check.SmallestDRANode(apiClient)
			if err != nil {
				return false, nil
			}

			return deviceCount > 0, nil
		})
}

// VLLMPodsUseDefaultScheduler waits for all matching vLLM pods to be scheduled
// by the Kubernetes default scheduler.
func VLLMPodsUseDefaultScheduler(
	apiClient *clients.Settings, namespace string, podLabels map[string]string,
	timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(context.Context) (bool, error) {
			return check.VLLMPodsUseDefaultScheduler(apiClient, namespace, podLabels)
		})
}

// VLLMDeploymentReady waits for a vLLM deployment to become ready while tolerating
// transient API errors and reporting a crashing container's previous logs.
func VLLMDeploymentReady(
	apiClient *clients.Settings, name, namespace string, podLabels map[string]string,
	timeout time.Duration) error {
	lastState := "deployment has not been observed"

	err := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			vllmDeployment, getErr := apiClient.K8sClient.AppsV1().Deployments(namespace).
				Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				if apierrors.IsForbidden(getErr) || apierrors.IsUnauthorized(getErr) {
					return false, getErr
				}

				lastState = fmt.Sprintf("deployment API request failed: %v", getErr)
				klog.V(params.NeuronLogLevel).Infof(
					"Failed to inspect vLLM deployment %s in namespace %s; retrying: %v",
					name, namespace, getErr)

				return false, nil
			}

			lastState = fmt.Sprintf(
				"deployment replicas=%d ready=%d available=%d unavailable=%d",
				vllmDeployment.Status.Replicas,
				vllmDeployment.Status.ReadyReplicas,
				vllmDeployment.Status.AvailableReplicas,
				vllmDeployment.Status.UnavailableReplicas)

			if vllmDeployment.Status.Replicas > 0 &&
				vllmDeployment.Status.ReadyReplicas == vllmDeployment.Status.Replicas {
				return true, nil
			}

			podList, listErr := apiClient.CoreV1Interface.Pods(namespace).List(
				ctx, metav1.ListOptions{LabelSelector: labels.Set(podLabels).String()})
			if listErr != nil {
				if apierrors.IsForbidden(listErr) || apierrors.IsUnauthorized(listErr) {
					return false, listErr
				}

				lastState += fmt.Sprintf("; pod API request failed: %v", listErr)

				return false, nil
			}

			podState, podErr := vllmPodState(ctx, apiClient, namespace, podList.Items)
			lastState += "; " + podState

			if podErr != nil {
				return false, podErr
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf(
			"vLLM deployment %s in namespace %s did not become ready; last observed state: %s: %w",
			name, namespace, lastState, err)
	}

	return nil
}

func vllmPodState(
	ctx context.Context, apiClient *clients.Settings, namespace string,
	pods []corev1.Pod) (string, error) {
	if len(pods) == 0 {
		return "no matching vLLM pods found", nil
	}

	podStates := make([]string, 0, len(pods))

	for _, vllmPod := range pods {
		containerStates := make([]string, 0, len(vllmPod.Status.ContainerStatuses))

		for _, containerStatus := range vllmPod.Status.ContainerStatuses {
			state := fmt.Sprintf(
				"container=%s ready=%t restarts=%d",
				containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)

			if waiting := containerStatus.State.Waiting; waiting != nil {
				state += fmt.Sprintf(" waiting=%s", waiting.Reason)
				if waiting.Message != "" {
					state += fmt.Sprintf(" message=%q", waiting.Message)
				}

				terminated := containerStatus.LastTerminationState.Terminated
				if waiting.Reason == "CrashLoopBackOff" && terminated != nil && terminated.ExitCode != 0 {
					logs := vllmContainerLogs(
						ctx, apiClient, namespace, vllmPod.Name, containerStatus.Name, true)

					return state, fmt.Errorf(
						"vLLM pod %s container %s is crash-looping after exit code %d (%s); previous logs:\n%s",
						vllmPod.Name, containerStatus.Name, terminated.ExitCode, terminated.Reason, logs)
				}
			}

			if terminated := containerStatus.State.Terminated; terminated != nil {
				state += fmt.Sprintf(" terminated=%s exitCode=%d", terminated.Reason, terminated.ExitCode)
				if terminated.ExitCode != 0 {
					logs := vllmContainerLogs(
						ctx, apiClient, namespace, vllmPod.Name, containerStatus.Name, false)

					return state, fmt.Errorf(
						"vLLM pod %s container %s exited with code %d (%s); logs:\n%s",
						vllmPod.Name, containerStatus.Name, terminated.ExitCode, terminated.Reason, logs)
				}
			}

			containerStates = append(containerStates, state)
		}

		podStates = append(podStates, fmt.Sprintf(
			"pod=%s phase=%s [%s]",
			vllmPod.Name, vllmPod.Status.Phase, strings.Join(containerStates, ", ")))
	}

	return strings.Join(podStates, "; "), nil
}

func vllmContainerLogs(
	ctx context.Context, apiClient *clients.Settings, namespace, podName, containerName string,
	previous bool) string {
	tailLines := int64(80)

	logs, err := apiClient.CoreV1Interface.Pods(namespace).GetLogs(
		podName, &corev1.PodLogOptions{
			Container: containerName,
			Previous:  previous,
			TailLines: &tailLines,
		}).DoRaw(ctx)
	if err != nil {
		return fmt.Sprintf("failed to retrieve container logs: %v", err)
	}

	const maxLogBytes = 12_000
	if len(logs) > maxLogBytes {
		logs = logs[len(logs)-maxLogBytes:]
	}

	return strings.TrimSpace(string(logs))
}

// DevicePluginDeployment waits for the device plugin DaemonSet to be ready.
func DevicePluginDeployment(apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron device plugin deployment in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing daemonsets: %v", err)

				return false, nil
			}

			for _, daemonSet := range dsList.Items {
				// Only consider DaemonSets with the Neuron device plugin prefix
				if !strings.HasPrefix(daemonSet.Name, params.DevicePluginDaemonSetPrefix) {
					continue
				}

				if daemonSet.Status.DesiredNumberScheduled > 0 &&
					daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
					klog.V(params.NeuronLogLevel).Infof(
						"Device plugin DaemonSet %s is ready: %d/%d",
						daemonSet.Name,
						daemonSet.Status.NumberReady,
						daemonSet.Status.DesiredNumberScheduled)

					return true, nil
				}
			}

			return false, nil
		})
}

// MetricsDaemonSet waits for the metrics DaemonSet to be ready.
func MetricsDaemonSet(apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron metrics DaemonSet in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing daemonsets: %v", err)

				return false, nil
			}

			for _, daemonSet := range dsList.Items {
				if daemonSet.Name == params.MetricsDaemonSetPrefix ||
					neuronparams.HasPrefix(daemonSet.Name, params.MetricsDaemonSetPrefix) {
					if daemonSet.Status.DesiredNumberScheduled > 0 &&
						daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
						klog.V(params.NeuronLogLevel).Infof(
							"Metrics DaemonSet %s is ready", daemonSet.Name)

						return true, nil
					}
				}
			}

			return false, nil
		})
}

// SchedulerDeployment waits for the custom scheduler deployment to be ready.
func SchedulerDeployment(apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for Neuron scheduler deployment in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deploy, err := deployment.Pull(apiClient, params.SchedulerDeploymentName, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Scheduler deployment not found yet: %v", err)

				return false, nil
			}

			if deploy.IsReady(10 * time.Second) {
				klog.V(params.NeuronLogLevel).Info("Scheduler deployment is ready")

				return true, nil
			}

			return false, nil
		})
}

// PodRunning waits for a specific pod to be running.
func PodRunning(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to be running", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Pod %s not found: %v", name, err)

				return false, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodRunning {
				klog.V(params.NeuronLogLevel).Infof("Pod %s is running", name)

				return true, nil
			}

			klog.V(params.NeuronLogLevel).Infof("Pod %s phase: %s", name, podBuilder.Object.Status.Phase)

			return false, nil
		})
}

// PodReady waits for a specific pod to be ready (all containers pass readiness probes).
func PodReady(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to be ready (readiness probe passed)", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 30*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Pod %s not found: %v", name, err)

				return false, nil
			}

			// Check if pod is running first
			if podBuilder.Object.Status.Phase != corev1.PodRunning {
				klog.V(params.NeuronLogLevel).Infof("Pod %s phase: %s (waiting for Running)",
					name, podBuilder.Object.Status.Phase)

				return false, nil
			}

			// Check if all containers are ready
			for _, cond := range podBuilder.Object.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					klog.V(params.NeuronLogLevel).Infof("Pod %s is ready", name)

					return true, nil
				}
			}

			// Log container statuses for debugging
			for _, cs := range podBuilder.Object.Status.ContainerStatuses {
				klog.V(params.NeuronLogLevel).Infof("Pod %s container %s: ready=%v, restarts=%d",
					name, cs.Name, cs.Ready, cs.RestartCount)
			}

			return false, nil
		})
}

// PodCompleted waits for a pod to complete successfully.
func PodCompleted(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for pod %s in namespace %s to complete", name, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			podBuilder, err := pod.Pull(apiClient, name, namespace)
			if err != nil {
				return false, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodSucceeded {
				return true, nil
			}

			if podBuilder.Object.Status.Phase == corev1.PodFailed {
				return false, fmt.Errorf("pod %s failed", name)
			}

			return false, nil
		})
}

// PodsDeleted waits for all pods with given label to be deleted from a namespace.
func PodsDeleted(apiClient *clients.Settings, namespace string,
	labelSelector map[string]string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for pods to be deleted from namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, namespace, metav1.ListOptions{
				LabelSelector: labels.Set(labelSelector).String(),
			})
			if err != nil {
				return false, nil
			}

			if len(pods) == 0 {
				klog.V(params.NeuronLogLevel).Info("All pods deleted")

				return true, nil
			}

			klog.V(params.NeuronLogLevel).Infof("Still %d pods remaining", len(pods))

			return false, nil
		})
}

// NeuronNodesLabeled waits for at least one node to be labeled with Neuron label.
func NeuronNodesLabeled(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for nodes to be labeled with Neuron feature")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s",
					params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
			})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing nodes: %v", err)

				return false, nil
			}

			if len(nodeList) > 0 {
				klog.V(params.NeuronLogLevel).Infof("Found %d Neuron-labeled nodes", len(nodeList))

				return true, nil
			}

			return false, nil
		})
}

// NodeResourceAvailable waits for a node to have the Neuron resource available.
func NodeResourceAvailable(apiClient *clients.Settings, nodeName string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for Neuron resources on node %s", nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", nodeName),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			node := nodeList[0]
			capacity := node.Object.Status.Capacity

			if quantity, ok := capacity[params.NeuronCapacityID]; ok {
				if quantity.Value() > 0 {
					klog.V(params.NeuronLogLevel).Infof("Node %s has Neuron capacity: %d",
						nodeName, quantity.Value())

					return true, nil
				}
			}

			return false, nil
		})
}

// AllNeuronNodesResourceAvailable waits for all Neuron-labeled nodes to have resources.
func AllNeuronNodesResourceAvailable(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for all Neuron nodes to have resources available")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nodeList, err := nodes.List(apiClient, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s",
					params.NeuronNFDLabelKey, params.NeuronNFDLabelValue),
			})
			if err != nil || len(nodeList) == 0 {
				return false, nil
			}

			for _, node := range nodeList {
				capacity := node.Object.Status.Capacity
				if quantity, ok := capacity[params.NeuronCapacityID]; !ok || quantity.Value() == 0 {
					klog.V(params.NeuronLogLevel).Infof("Node %s does not have Neuron resources yet",
						node.Object.Name)

					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Infof(
				"All %d Neuron nodes have resources available", len(nodeList))

			return true, nil
		})
}

// BuildConfigMapCreated waits for the Dockerfile ConfigMap to be created by the operator.
func BuildConfigMapCreated(apiClient *clients.Settings, namespace, deviceConfigName string,
	timeout time.Duration) error {
	cmName := params.BuildConfigMapPrefix + deviceConfigName

	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for build ConfigMap %s in namespace %s", cmName, namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := apiClient.K8sClient.CoreV1().ConfigMaps(namespace).Get(
				ctx, cmName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					klog.V(params.NeuronLogLevel).Infof("ConfigMap %s not found yet", cmName)

					return false, nil
				}

				return false, err
			}

			klog.V(params.NeuronLogLevel).Infof("ConfigMap %s exists", cmName)

			return true, nil
		})
}

// DRADaemonSet waits for the DRA DaemonSet to be ready.
func DRADaemonSet(apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for DRA DaemonSet in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s",
						params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
				})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing DRA daemonsets: %v", err)

				return false, nil
			}

			for _, ds := range dsList.Items {
				if ds.Status.DesiredNumberScheduled > 0 &&
					ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
					klog.V(params.NeuronLogLevel).Infof(
						"DRA DaemonSet %s is ready: %d/%d",
						ds.Name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)

					return true, nil
				}
			}

			return false, nil
		})
}

// DRADaemonSetGone waits for all DRA DaemonSets to be deleted from a namespace.
func DRADaemonSetGone(apiClient *clients.Settings, namespace string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for DRA DaemonSets to be deleted from namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s",
						params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
				})
			if err != nil {
				return false, nil
			}

			if len(dsList.Items) == 0 {
				klog.V(params.NeuronLogLevel).Info("All DRA DaemonSets deleted")

				return true, nil
			}

			return false, nil
		})
}

// DeviceClassExists waits for a DeviceClass to exist.
func DeviceClassExists(apiClient *clients.Settings, name string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for DeviceClass %s to exist", name)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := apiClient.K8sClient.ResourceV1().DeviceClasses().Get(
				ctx, name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}

				return false, err
			}

			klog.V(params.NeuronLogLevel).Infof("DeviceClass %s exists", name)

			return true, nil
		})
}

// DeviceClassGone waits for a DeviceClass to be deleted.
func DeviceClassGone(apiClient *clients.Settings, name string, timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof("Waiting for DeviceClass %s to be deleted", name)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := apiClient.K8sClient.ResourceV1().DeviceClasses().Get(
				ctx, name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					klog.V(params.NeuronLogLevel).Infof("DeviceClass %s deleted", name)

					return true, nil
				}

				return false, err
			}

			return false, nil
		})
}

// NoSchedulerDeployments waits until no scheduler-related deployments exist.
func NoSchedulerDeployments(apiClient *clients.Settings, namespace string,
	timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for scheduler deployments to be absent from namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deployList, err := apiClient.K8sClient.AppsV1().Deployments(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, deploy := range deployList.Items {
				if strings.Contains(deploy.Name, "scheduler") {
					klog.V(params.NeuronLogLevel).Infof(
						"Scheduler deployment %s still exists", deploy.Name)

					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Info("No scheduler deployments found")

			return true, nil
		})
}

// SchedulerDeploymentBySubstring waits for the custom-scheduler deployment to be ready.
// The operator creates "<name>-custom-scheduler", not the constant SchedulerDeploymentName.
// Matches "custom-scheduler" but excludes "custom-scheduler-extension".
func SchedulerDeploymentBySubstring(apiClient *clients.Settings, namespace string,
	timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for custom-scheduler deployment in namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deployList, err := apiClient.K8sClient.AppsV1().Deployments(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, deploy := range deployList.Items {
				if strings.Contains(deploy.Name, "custom-scheduler") &&
					!strings.Contains(deploy.Name, "scheduler-extension") &&
					deploy.Status.ReadyReplicas > 0 {
					klog.V(params.NeuronLogLevel).Infof(
						"Scheduler deployment %s is ready", deploy.Name)

					return true, nil
				}
			}

			return false, nil
		})
}

// DevicePluginDaemonSetGone waits for all device-plugin DaemonSets to be deleted from a namespace.
func DevicePluginDaemonSetGone(apiClient *clients.Settings, namespace string,
	timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for device-plugin DaemonSets to be deleted from namespace %s", namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(namespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for _, ds := range dsList.Items {
				if strings.HasPrefix(ds.Name, params.DevicePluginDaemonSetPrefix) {
					klog.V(params.NeuronLogLevel).Infof(
						"Device-plugin DaemonSet %s still exists", ds.Name)

					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Info("All device-plugin DaemonSets deleted")

			return true, nil
		})
}

// ResourceSlicesGone waits for all ResourceSlices with the given driver to be deleted.
func ResourceSlicesGone(apiClient *clients.Settings, driverName string,
	timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for ResourceSlices with driver %s to be deleted", driverName)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			sliceList, err := apiClient.K8sClient.ResourceV1().ResourceSlices().List(
				ctx, metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			for idx := range sliceList.Items {
				if sliceList.Items[idx].Spec.Driver == driverName {
					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Infof(
				"All ResourceSlices for driver %s deleted", driverName)

			return true, nil
		})
}

// DevicePluginRunningOnNode waits for the device plugin pod to be running on a specific node.
func DevicePluginRunningOnNode(apiClient *clients.Settings, nodeName string,
	timeout time.Duration) error {
	klog.V(params.NeuronLogLevel).Infof(
		"Waiting for device plugin pod to be running on node %s", nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, params.NeuronNamespace, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
			})
			if err != nil {
				return false, fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
			}

			for _, currentPod := range pods {
				if neuronparams.HasPrefix(currentPod.Object.Name, params.DevicePluginDaemonSetPrefix) {
					if currentPod.Object.Status.Phase == corev1.PodRunning {
						klog.V(params.NeuronLogLevel).Infof(
							"Device plugin pod running on node %s", nodeName)

						return true, nil
					}
				}
			}

			return false, nil
		})
}
