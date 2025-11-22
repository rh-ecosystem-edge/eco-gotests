package ranhelper

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// DoesContainerExistInPod checks if a given container exists in a given pod.
func DoesContainerExistInPod(pod *pod.Builder, containerName string) bool {
	containers := pod.Object.Status.ContainerStatuses

	for _, container := range containers {
		if container.Name == containerName {
			klog.V(ranparam.LogLevel).Infof("found %s container", containerName)

			return true
		}
	}

	return false
}

// UnmarshalRaw converts raw bytes for a K8s CR into the actual type.
func UnmarshalRaw[T any](raw []byte) (*T, error) {
	untyped := &unstructured.Unstructured{}

	err := untyped.UnmarshalJSON(raw)
	if err != nil {
		return nil, err
	}

	var typed T

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(untyped.UnstructuredContent(), &typed)
	if err != nil {
		return nil, err
	}

	return &typed, nil
}

// ExecLocalCommand runs the provided command with the provided args locally, cancelling execution if it exceeds
// timeout.
func ExecLocalCommand(timeout time.Duration, command string, args ...string) (string, error) {
	klog.V(90).Infof("Locally executing command '%s' with args '%v'", command, args)

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)

	defer cancel()

	output, err := exec.CommandContext(ctx, command, args...).Output()

	return string(output), err
}

// GetPodNameWithLabel returns a pod name matching pod label selector in a given namespace.
func GetPodNameWithLabel(client *clients.Settings, podNamespace, podLabelSelector string) (string, error) {
	podList, err := pod.List(client, podNamespace, metav1.ListOptions{LabelSelector: podLabelSelector})
	if err != nil {
		return "", err
	}

	klog.V(ranparam.LogLevel).Infof("Length of podList matching podLabelSelector is '%v'", len(podList))

	if len(podList) < 1 {
		return "", fmt.Errorf("no pod found with label %s under namespace %s", podLabelSelector, podNamespace)
	}

	klog.V(ranparam.LogLevel).Infof("podList[0] matching podLabelSelector is '%v'",
		podList[0].Definition.Name)

	return podList[0].Definition.Name, nil
}
