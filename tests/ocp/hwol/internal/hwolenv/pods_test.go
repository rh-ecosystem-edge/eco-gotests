//go:build unit_test

package hwolenv

import (
	"context"
	"testing"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
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

func TestCreateOwnedPodDoesNotAdoptExisting(t *testing.T) {
	foreign := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "workload", Namespace: "test", UID: types.UID("foreign"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "foreign-run"},
	}}
	kube := fake.NewSimpleClientset(foreign)
	settings := &clients.Settings{K8sClient: kube, CoreV1Interface: kube.CoreV1()}
	previous := APIClient
	APIClient = settings

	t.Cleanup(func() { APIClient = previous })

	_, err := CreateOwnedPod(pod.NewBuilder(settings, "workload", "test", "image"), "run", time.Second)
	require.Error(t, err)

	for _, action := range kube.Actions() {
		require.NotEqual(t, "delete", action.GetVerb())
	}

	current, err := kube.CoreV1().Pods("test").Get(context.Background(), "workload", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, foreign.UID, current.UID)
}

func TestCreateOwnedPodRecordsUID(t *testing.T) {
	kube := fake.NewSimpleClientset()
	kube.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		createAction, isCreateAction := action.(ktesting.CreateAction)
		require.True(t, isCreateAction)

		rawPod := createAction.GetObject()

		obj, isPod := rawPod.(*corev1.Pod)
		require.True(t, isPod)

		obj.UID = types.UID("created")
		obj.ResourceVersion = "1"
		obj.Status.Phase = corev1.PodRunning

		return false, nil, nil
	})
	settings := &clients.Settings{K8sClient: kube, CoreV1Interface: kube.CoreV1()}
	previous := APIClient
	APIClient = settings

	t.Cleanup(func() { APIClient = previous })

	created, err := CreateOwnedPod(pod.NewBuilder(settings, "workload", "test", "image"), "run", time.Second)
	require.NoError(t, err)
	require.Equal(t, types.UID("created"), created.Definition.UID)
	require.NoError(t, DeleteOwnedPod(created, time.Second))
}

func TestOwnedPodCleanup(t *testing.T) {
	obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "workload", Namespace: "test", UID: types.UID("original"), ResourceVersion: "7",
		Labels: map[string]string{tsparams.TestRunLabelKey: "run"},
	}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	client := fake.NewSimpleClientset(obj)
	pods := client.CoreV1().Pods("test")

	require.NoError(t, waitForOwnedPod(pods, obj.Name, obj.UID, "run", time.Second))
	require.ErrorContains(t, waitForOwnedPod(pods, obj.Name, types.UID("other"), "run", time.Second), "replaced")
	require.ErrorContains(t, deleteOwnedPod(pods, obj.Name, obj.UID, "other", time.Second), "ownership changed")
	require.ErrorContains(t, deleteOwnedPod(pods, obj.Name, types.UID("other"), "run", time.Second), "UID or ownership")
	_, err := pods.Get(context.Background(), obj.Name, metav1.GetOptions{})
	require.NoError(t, err)

	require.NoError(t, deleteOwnedPod(pods, obj.Name, obj.UID, "run", time.Second))

	for _, action := range client.Actions() {
		if action.GetVerb() == "delete" {
			deleteAction, isDeleteAction := action.(ktesting.DeleteAction)
			require.True(t, isDeleteAction)

			options := deleteAction.GetDeleteOptions()
			require.NotNil(t, options.Preconditions)
			require.Equal(t, obj.UID, *options.Preconditions.UID)
			require.Equal(t, obj.ResourceVersion, *options.Preconditions.ResourceVersion)

			return
		}
	}

	t.Fatal("expected a UID- and resourceVersion-guarded delete")
}

func TestOwnedPodReplacementIsNotDeleted(t *testing.T) {
	obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "workload", Namespace: "test", UID: types.UID("replacement"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "run"},
	}}
	client := fake.NewSimpleClientset(obj)
	err := deleteOwnedPod(client.CoreV1().Pods("test"), obj.Name, types.UID("original"), "run", time.Second)
	require.ErrorContains(t, err, "UID or ownership changed")

	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb())
	}
}
