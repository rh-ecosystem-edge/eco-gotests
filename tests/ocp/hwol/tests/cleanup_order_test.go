//go:build unit_test

package tests

import (
	"fmt"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
