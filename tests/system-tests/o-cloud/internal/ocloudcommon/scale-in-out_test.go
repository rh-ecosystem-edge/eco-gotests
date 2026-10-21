//go:build unit_test

package ocloudcommon

import (
	"testing"
	"time"

	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScaleWorkerHostnamesConfiguredWorker(t *testing.T) {
	useScaleRoleConfig(t)

	for _, role := range []string{"worker", ""} {
		t.Run("PR role "+role, func(t *testing.T) {
			requested := []scaleRequestedNode{{hostname: "node-a", role: role}}
			spokeNodes := scaleTestSpokeNodes(inittools.GeneralConfig.WorkerLabel)

			workers, err := scaleWorkerHostnames(requested, spokeNodes)

			require.NoError(t, err)
			assert.Equal(t, []string{"node-a"}, workers)
		})
	}
}

func TestScaleWorkerHostnamesProtectControlPlane(t *testing.T) {
	useScaleRoleConfig(t)

	testCases := []struct {
		name  string
		role  string
		label string
	}{
		{name: "configured control plane", label: inittools.GeneralConfig.ControlPlaneLabel},
		{name: "configured prefix master", label: "roles.example.com/master"},
		{name: "configured prefix control plane", label: "roles.example.com/control-plane"},
		{name: "standard master", label: "node-role.kubernetes.io/master"},
		{name: "standard control plane", label: "node-role.kubernetes.io/control-plane"},
		{name: "PR master", role: "master"},
		{name: "PR control plane", role: "control-plane"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			requested := []scaleRequestedNode{{hostname: "node-a", role: testCase.role}}
			spokeNodes := scaleTestSpokeNodes(inittools.GeneralConfig.WorkerLabel, testCase.label)

			workers, err := scaleWorkerHostnames(requested, spokeNodes)

			require.NoError(t, err)
			assert.Empty(t, workers, "control-plane nodes must remain protected when they also carry a worker label")
		})
	}
}

func TestScaleWorkerHostnamesRejectAmbiguousRoles(t *testing.T) {
	useScaleRoleConfig(t)

	testCases := []struct {
		name  string
		role  string
		label string
	}{
		{name: "missing role sources"},
		{name: "PR worker without configured label", role: "worker", label: "node-role.kubernetes.io/worker"},
		{name: "conflicting PR role", role: "infra", label: inittools.GeneralConfig.WorkerLabel},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			requested := []scaleRequestedNode{{hostname: "node-a", role: testCase.role}}
			spokeNodes := scaleTestSpokeNodes(testCase.label)

			workers, err := scaleWorkerHostnames(requested, spokeNodes)

			require.Error(t, err)
			assert.Empty(t, workers, "ambiguous roles must not produce removable workers")
		})
	}
}

func TestScalePRFulfilledAfter(t *testing.T) {
	updatedAfter := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	freshUpdate := updatedAfter.Add(time.Second)
	testCases := []struct {
		name       string
		generation int64
		observed   int64
		phase      provisioningv1alpha1.ProvisioningPhase
		updateTime time.Time
		expected   bool
	}{
		{
			name: "fresh fulfillment", generation: 4, observed: 4,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: freshUpdate, expected: true,
		},
		{
			name: "old generation", generation: 3, observed: 3,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: freshUpdate,
		},
		{
			name: "unobserved generation", generation: 4, observed: 3,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: freshUpdate,
		},
		{
			name: "later unobserved generation", generation: 5, observed: 4,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: freshUpdate,
		},
		{
			name: "stale fulfillment", generation: 4, observed: 4,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: updatedAfter.Add(-time.Second),
		},
		{
			name: "equal timestamp", generation: 4, observed: 4,
			phase: provisioningv1alpha1.StateFulfilled, updateTime: updatedAfter,
		},
		{
			name: "progressing phase", generation: 4, observed: 4,
			phase: provisioningv1alpha1.StateProgressing, updateTime: freshUpdate,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := &provisioningv1alpha1.ProvisioningRequest{
				ObjectMeta: metav1.ObjectMeta{Generation: testCase.generation},
				Status: provisioningv1alpha1.ProvisioningRequestStatus{
					ObservedGeneration: testCase.observed,
					ProvisioningStatus: provisioningv1alpha1.ProvisioningStatus{
						ProvisioningPhase: testCase.phase,
						UpdateTime:        metav1.NewTime(testCase.updateTime),
					},
				},
			}

			assert.Equal(t, testCase.expected, scalePRFulfilledAfter(request, 4, updatedAfter))
		})
	}
}

func TestScalePRFulfilledAfterUnchangedRestore(t *testing.T) {
	request := &provisioningv1alpha1.ProvisioningRequest{
		ObjectMeta: metav1.ObjectMeta{Generation: 4},
		Status: provisioningv1alpha1.ProvisioningRequestStatus{
			ObservedGeneration: 4,
			ProvisioningStatus: provisioningv1alpha1.ProvisioningStatus{
				ProvisioningPhase: provisioningv1alpha1.StateFulfilled,
			},
		},
	}

	assert.True(t, scalePRFulfilledAfter(request, 4, time.Time{}),
		"restoring an unchanged PR must accept its already fulfilled generation")

	request.Status.ObservedGeneration = 3
	assert.False(t, scalePRFulfilledAfter(request, 4, time.Time{}),
		"an unchanged PR still requires the current generation to be observed")
}

func useScaleRoleConfig(t *testing.T) {
	t.Helper()

	originalConfig := inittools.GeneralConfig

	t.Cleanup(func() {
		inittools.GeneralConfig = originalConfig
	})

	inittools.GeneralConfig = &config.GeneralConfig{
		KubernetesRolePrefix: "roles.example.com",
		WorkerLabel:          "roles.example.com/compute",
		ControlPlaneLabel:    "roles.example.com/control",
	}
}

func scaleTestSpokeNodes(roleLabels ...string) map[string]*nodes.Builder {
	labels := make(map[string]string, len(roleLabels))
	for _, label := range roleLabels {
		if label != "" {
			labels[label] = ""
		}
	}

	return map[string]*nodes.Builder{
		"node-a": {
			Object: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: labels}},
		},
	}
}
