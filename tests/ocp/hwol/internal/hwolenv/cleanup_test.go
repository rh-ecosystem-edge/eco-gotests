//go:build unit_test

package hwolenv

import (
	"context"
	"testing"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeleteOwnedTestResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	obj := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("original-uid"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "other-run"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()

	err := deleteOwnedTestResource(client, "operator", "policy", "this-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "not owned")
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(obj), &sriovv1.SriovNetworkNodePolicy{}))

	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing")
	key := resourceKey(obj, "other-run")
	createdResourceUIDs.Store(key, types.UID("different-uid"))
	t.Cleanup(func() { createdResourceUIDs.Delete(key) })
	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing or changed")
	createdResourceUIDs.Store(key, obj.UID)
	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.NoError(t, err)
	require.Error(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(obj), &sriovv1.SriovNetworkNodePolicy{}))
}

func TestCreateOwnedTestResourceNeverAdoptsExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	foreign := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("foreign"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "other-run"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreign).Build()
	wanted := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", Labels: map[string]string{tsparams.TestRunLabelKey: "this-run"},
	}}
	err := createOwnedTestResource(client, wanted, "this-run")
	require.True(t, k8serrors.IsAlreadyExists(err), "got %v", err)
	current := &sriovv1.SriovNetworkNodePolicy{}
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(foreign), current))
	require.Equal(t, foreign.UID, current.UID)
	require.Equal(t, "other-run", current.Labels[tsparams.TestRunLabelKey])
	require.Error(t, createOwnedTestResource(client, wanted, "different-run"))
}

func TestOwnedResourceRejectsReplacementWithCopiedLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	created := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("original"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "run-replacement-test"},
	}}
	key := resourceKey(created, "run-replacement-test")
	t.Cleanup(func() { createdResourceUIDs.Delete(key) })
	require.NoError(t, createOwnedTestResource(client, created, "run-replacement-test"))
	require.NoError(t, client.Delete(context.Background(), created))
	replacement := created.DeepCopy()
	replacement.UID = types.UID("replacement")
	replacement.ResourceVersion = ""
	require.NoError(t, client.Create(context.Background(), replacement))
	err := deleteOwnedTestResource(client, "operator", "policy", "run-replacement-test", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing or changed")
	current := &sriovv1.SriovNetworkNodePolicy{}
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(replacement), current))
	require.Equal(t, types.UID("replacement"), current.UID)
}
