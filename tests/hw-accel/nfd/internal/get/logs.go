package get

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// PodLogs get a raw log from pod.
func PodLogs(apiClient *clients.Settings, nsname, podName string) (string, error) {
	podWorker, err := pod.Pull(apiClient, podName, nsname)
	podLogOpts := corev1.PodLogOptions{}

	if err != nil {
		return "", err
	}

	podLogs := apiClient.Pods(nsname).GetLogs(podWorker.Definition.Name, &podLogOpts).Do(context.TODO())

	body, err := podLogs.Raw()
	if err != nil {
		klog.V(nfdparams.LogLevel).Infof("Failed to retrieve logs %v", err)
	}

	return string(body), nil
}
