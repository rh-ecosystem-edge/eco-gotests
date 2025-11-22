package get

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

// PodRestartCount get pod's reset count.
func PodRestartCount(apiClient *clients.Settings, nsname, podName string) (int32, error) {
	podWorker, err := pod.Pull(apiClient, podName, nsname)
	if err != nil {
		return 0, err
	}

	return podWorker.Object.Status.ContainerStatuses[0].RestartCount, nil
}
