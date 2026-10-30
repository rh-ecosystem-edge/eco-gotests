//go:build unit_test

package hwolenv

import (
	"context"
	"testing"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateHwolOperatorConfigDoesNotEnableDisabledPlugins(t *testing.T) {
	spec := sriovv1.SriovOperatorConfigSpec{
		EnableOvsOffload: true,
		FeatureGates:     map[string]bool{tsparams.ManageSoftwareBridgesGate: true},
	}
	require.NoError(t, validateHwolOperatorConfig(spec, "15b3"))
	spec.DisablePlugins = append(spec.DisablePlugins, "mellanox")
	require.ErrorContains(t, validateHwolOperatorConfig(spec, "15b3"), "Mellanox plugin")
	require.NoError(t, validateHwolOperatorConfig(spec, "8086"))
	spec.DisablePlugins = nil
	spec.EnableOvsOffload = false
	require.ErrorContains(t, validateHwolOperatorConfig(spec, "15b3"), "must enable")
}

func TestSwitchdevPreflightRejectsForeignState(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{
		"node-role.kubernetes.io/canary": "",
	}}}
	device := hwolconfig.DeviceConfig{InterfaceName: "pf0", Vendor: "15b3", DeviceID: "a2d6"}
	state := &sriovv1.SriovNetworkNodeState{ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Status: sriovv1.SriovNetworkNodeStateStatus{SyncStatus: "Succeeded", Interfaces: sriovv1.InterfaceExts{{
			Name: "pf0", Vendor: "15b3", DeviceID: "a2d6", PciAddress: "0000:4b:00.0", TotalVfs: 3,
		}}}}
	require.NoError(t, validateNodeFoundationState(node, state, device, 3, nil, nil))
	state.Status.Interfaces[0].NumVfs = 1
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "unused legacy PF")
	state.Status.Interfaces[0].NumVfs = 0
	state.Status.Interfaces[0].EswitchMode = "switchdev"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "unused legacy PF")
	state.Status.Interfaces[0].EswitchMode = "legacy"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 4, nil, nil), "capacity")
	state.Spec.Interfaces = sriovv1.Interfaces{{PciAddress: "0000:4b:00.0"}}
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "pre-existing")
	state.Spec.Interfaces = nil
	policy := sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{Name: "foreign"}}
	policy.Spec.NodeSelector = map[string]string{"node-role.kubernetes.io/canary": ""}
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, []sriovv1.SriovNetworkNodePolicy{policy}, nil), "policy foreign")
	pool := sriovv1.SriovNetworkPoolConfig{ObjectMeta: metav1.ObjectMeta{Name: "foreign-pool"}}
	pool.Spec.OvsHardwareOffloadConfig.Name = "canary"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, []sriovv1.SriovNetworkPoolConfig{pool}), "pool config foreign-pool")
}

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
