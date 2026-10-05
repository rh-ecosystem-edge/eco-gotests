//go:build unit_test

package consumer

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterActiveConsumerPods(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	runningPod := &pod.Builder{Object: &corev1.Pod{
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}}
	terminatingPod := &pod.Builder{Object: &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}}
	succeededPod := &pod.Builder{Object: &corev1.Pod{
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}}

	active := filterActiveConsumerPods([]*pod.Builder{runningPod, terminatingPod, succeededPod, nil})
	assert.Len(t, active, 1)
	assert.Equal(t, runningPod, active[0])
}

func TestFilterActiveConsumerPodsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, filterActiveConsumerPods(nil))
}

func TestFilterActiveConsumerPodsPendingExcluded(t *testing.T) {
	t.Parallel()

	pendingPod := &pod.Builder{Object: &corev1.Pod{
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}}

	assert.Empty(t, filterActiveConsumerPods([]*pod.Builder{pendingPod}))
}
