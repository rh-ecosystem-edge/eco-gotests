package draenv

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// PodReady verifies Running/Ready and, when requested, successful required init containers.
func PodReady(pod corev1.Pod, initRequired bool) error {
	ready := false

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			ready = true
		}
	}

	if !ready || pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod %s not Ready/Running", pod.Name)
	}

	if !initRequired {
		return nil
	}

	if len(pod.Spec.InitContainers) == 0 || len(pod.Status.InitContainerStatuses) != len(pod.Spec.InitContainers) {
		return fmt.Errorf("pod %s init incomplete", pod.Name)
	}

	for _, status := range pod.Status.InitContainerStatuses {
		if status.State.Terminated == nil || status.State.Terminated.ExitCode != 0 {
			return fmt.Errorf("pod %s init unsuccessful", pod.Name)
		}
	}

	return nil
}
