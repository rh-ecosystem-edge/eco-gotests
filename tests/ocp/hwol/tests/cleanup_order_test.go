//go:build unit_test

package tests

import (
	"fmt"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestCleanupSpecResourcesOrderAndFailure(t *testing.T) {
	server := &pod.Builder{Definition: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "server"}}}
	client := &pod.Builder{Definition: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "client"}}}
	var calls []string
	deletePod := func(p *pod.Builder) error {
		calls = append(calls, p.Definition.Name)
		return nil
	}
	deleteNetwork := func() error { calls = append(calls, "network"); return nil }
	require.NoError(t, cleanupSpecResources([]*pod.Builder{server, client}, deletePod, deleteNetwork))
	require.Equal(t, []string{"client", "server", "network"}, calls)

	calls = nil
	err := cleanupSpecResources([]*pod.Builder{server, client}, func(p *pod.Builder) error {
		calls = append(calls, p.Definition.Name)
		if p == client {
			return fmt.Errorf("client delete failed")
		}
		return nil
	}, deleteNetwork)
	require.ErrorContains(t, err, "client delete failed")
	require.Equal(t, []string{"client", "server"}, calls, "network must remain if a pod cannot be removed")
}

func TestTrafficPodRequiresAndUsesSelectedNode(t *testing.T) {
	_, err := createTrafficPod("attach", "network", "", "run", nil)
	require.ErrorContains(t, err, "node is required")

	kube := fake.NewSimpleClientset()
	kube.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		created := action.(ktesting.CreateAction).GetObject().(*corev1.Pod)
		require.Equal(t, "wsfd-advnetlab246", created.Spec.NodeName)
		created.UID = types.UID("owned-pod")
		created.Status.Phase = corev1.PodRunning
		return false, nil, nil
	})
	previousClient, previousConfig, previousNamespace := APIClient, HwolOcpConfig, tsparams.TestNamespaceName
	APIClient = &clients.Settings{K8sClient: kube, CoreV1Interface: kube.CoreV1()}
	HwolOcpConfig = &hwolconfig.HwolOcpConfig{OcpHwolTestContainer: "example.invalid/iperf"}
	tsparams.TestNamespaceName = "hwol-tests-unit"
	t.Cleanup(func() {
		APIClient, HwolOcpConfig, tsparams.TestNamespaceName = previousClient, previousConfig, previousNamespace
	})
	created, err := createTrafficPod("attach", "network", "wsfd-advnetlab246", "run", nil)
	require.NoError(t, err)
	require.Equal(t, "wsfd-advnetlab246", created.Object.Spec.NodeName)
}
