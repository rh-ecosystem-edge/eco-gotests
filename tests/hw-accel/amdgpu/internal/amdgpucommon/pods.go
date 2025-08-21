package amdgpucommon

import (
	corev1 "k8s.io/api/core/v1"
)

// AreAllContainersReady checks if all containers in a pod are ready.
func AreAllContainersReady(pod corev1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false
		}
	}

	return true
}

// IsPodRunningAndReady checks if a pod is running and all containers are ready.
func IsPodRunningAndReady(pod corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning && AreAllContainersReady(pod)
}
