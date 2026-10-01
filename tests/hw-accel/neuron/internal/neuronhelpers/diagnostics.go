package neuronhelpers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const kmmBuildNamespace = "openshift-kmm"

// LogDRAInClusterBuildDiagnostics records the KMM Module, build ConfigMap,
// build-related pods, and recent Events after an in-cluster DRA build does not
// converge. It deliberately logs status and messages only; credentials are
// never read or printed.
func LogDRAInClusterBuildDiagnostics(apiClient *clients.Settings) {
	ctx := context.TODO()

	deviceConfig, err := apiClient.K8sClient.CoreV1().ConfigMaps(params.NeuronNamespace).Get(
		ctx, params.BuildConfigMapPrefix+params.DefaultDeviceConfigName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: build ConfigMap lookup failed: %v", err)
	} else {
		klog.Infof("DRA in-cluster build diagnostics: build ConfigMap %s has data keys: %s",
			deviceConfig.Name, strings.Join(sortedKeys(deviceConfig.Data), ", "))
	}

	module, err := kmm.Pull(apiClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: Module lookup failed: %v", err)
	} else {
		logJSON("DRA in-cluster build diagnostics: Module spec", module.Object.Spec)
		logJSON("DRA in-cluster build diagnostics: Module status", module.Object.Status)
	}

	for _, namespace := range []string{params.NeuronNamespace, kmmBuildNamespace} {
		logPodDiagnostics(ctx, apiClient, namespace)
		logEventDiagnostics(ctx, apiClient, namespace)
	}
}

func logPodDiagnostics(ctx context.Context, apiClient *clients.Settings, namespace string) {
	pods, err := apiClient.K8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: pod listing failed in %s: %v", namespace, err)

		return
	}

	for _, pod := range pods.Items {
		if !isBuildRelatedPod(pod) {
			continue
		}

		klog.Infof("DRA in-cluster build diagnostics: pod %s/%s phase=%s reason=%s message=%s",
			namespace, pod.Name, pod.Status.Phase, pod.Status.Reason, pod.Status.Message)

		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			state := containerState(status.State)
			previous := containerState(status.LastTerminationState)
			klog.Infof("DRA in-cluster build diagnostics: pod %s/%s container=%s ready=%t restarts=%d state=%s previous=%s",
				namespace, pod.Name, status.Name, status.Ready, status.RestartCount, state, previous)
		}
	}
}

func logEventDiagnostics(ctx context.Context, apiClient *clients.Settings, namespace string) {
	events, err := apiClient.K8sClient.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: Event listing failed in %s: %v", namespace, err)

		return
	}

	for _, event := range events.Items {
		klog.Infof("DRA in-cluster build diagnostics: event %s/%s type=%s reason=%s involved=%s/%s message=%s",
			namespace, event.Name, event.Type, event.Reason, event.InvolvedObject.Kind,
			event.InvolvedObject.Name, event.Message)
	}
}

func isBuildRelatedPod(pod corev1.Pod) bool {
	name := strings.ToLower(pod.Name)
	return strings.Contains(name, "build") || strings.Contains(name, "kmm") ||
		strings.Contains(name, params.DefaultDeviceConfigName)
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Waiting != nil:
		return fmt.Sprintf("waiting(reason=%s message=%s)", state.Waiting.Reason, state.Waiting.Message)
	case state.Running != nil:
		return "running"
	case state.Terminated != nil:
		return fmt.Sprintf("terminated(reason=%s exit=%d message=%s)",
			state.Terminated.Reason, state.Terminated.ExitCode, state.Terminated.Message)
	default:
		return "unknown"
	}
}

func logJSON(label string, value interface{}) {
	encoded, err := json.Marshal(value)
	if err != nil {
		klog.Errorf("%s: JSON encoding failed: %v", label, err)

		return
	}

	klog.Infof("%s: %s", label, encoded)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	// ConfigMap key order is not semantically meaningful; keep diagnostics
	// stable without exposing ConfigMap values.
	sort.Strings(keys)

	return keys
}
