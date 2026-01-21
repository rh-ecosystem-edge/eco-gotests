package do

import (
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateTestWorkloadPod creates a simple test workload pod that uses Neuron resources.
func CreateTestWorkloadPod(name, namespace, nodeName, containerName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			NodeName:      nodeName,
			Containers: []corev1.Container{
				{
					Name:    containerName,
					Image:   "public.ecr.aws/amazonlinux/amazonlinux:latest",
					Command: []string{"/bin/sh", "-c", "while true; do sleep 3600; done"},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName(params.NeuronCapacityID): resource.MustParse("1"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceName(params.NeuronCapacityID): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}
}

// IsDevicePluginPod checks if a pod is a device plugin pod by comparing its name prefix.
func IsDevicePluginPod(podName string) bool {
	prefix := params.DevicePluginDaemonSetPrefix

	return len(podName) >= len(prefix) && podName[:len(prefix)] == prefix
}
