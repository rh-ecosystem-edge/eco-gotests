package draenv_test

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/dra/internal/draenv"
	corev1 "k8s.io/api/core/v1"
)

func TestPodReady(t *testing.T) {
	healthy := corev1.Pod{
		Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "wait-for-config"}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "wait-for-config", State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
			}}},
		},
	}

	cases := []struct {
		name    string
		mutate  func(*corev1.Pod)
		wantErr bool
	}{
		{"healthy", func(*corev1.Pod) {}, false},
		{"not ready", func(p *corev1.Pod) { p.Status.Conditions = nil }, true},
		{"not running", func(p *corev1.Pod) { p.Status.Phase = corev1.PodPending }, true},
		{"missing init", func(p *corev1.Pod) { p.Spec.InitContainers = nil }, true},
		{"init not reported", func(p *corev1.Pod) { p.Status.InitContainerStatuses = nil }, true},
		{"init failed", func(p *corev1.Pod) { p.Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1 }, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := healthy.DeepCopy()
			testCase.mutate(candidate)

			if err := draenv.PodReady(*candidate, true); (err != nil) != testCase.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, testCase.wantErr)
			}
		})
	}

	healthy.Spec.InitContainers = nil

	healthy.Status.InitContainerStatuses = nil
	if err := draenv.PodReady(healthy, false); err != nil {
		t.Fatalf("device-plugin readiness should not require init: %v", err)
	}
}
