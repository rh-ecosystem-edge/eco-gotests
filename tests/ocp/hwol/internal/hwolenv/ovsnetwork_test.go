package hwolenv

import (
	"strings"
	"testing"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOvsNetworkDeleteAndWaitRemovesObject(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := sriovv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add SR-IOV scheme: %v", err)
	}

	network := &sriovv1.OVSNetwork{ObjectMeta: metav1.ObjectMeta{
		Name:      "hwol-ovsnet",
		Namespace: "openshift-sriov-network-operator",
		Labels: map[string]string{
			tsparams.TestResourceLabelKey: tsparams.TestResourceLabelValue,
		},
	}}
	apiClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(network).Build()
	builder := &OvsNetworkBuilder{Definition: network.DeepCopy(), apiClient: apiClient}

	if err := builder.DeleteAndWait(time.Second); err != nil {
		t.Fatalf("delete and wait for OVSNetwork: %v", err)
	}

	if _, err := builder.Get(); !k8serrors.IsNotFound(err) {
		t.Fatalf("expected OVSNetwork to be deleted, got: %v", err)
	}
}

func TestOvsNetworkWaitUntilDeletedReturnsWhileObjectIsAbsent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := sriovv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add SR-IOV scheme: %v", err)
	}

	builder := &OvsNetworkBuilder{
		Definition: &sriovv1.OVSNetwork{ObjectMeta: metav1.ObjectMeta{
			Name:      "hwol-ovsnet",
			Namespace: "openshift-sriov-network-operator",
		}},
		apiClient: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	if err := builder.WaitUntilDeleted(time.Second); err != nil {
		t.Fatalf("wait for absent OVSNetwork: %v", err)
	}
}

func TestOvsNetworkDeleteRejectsUnownedObject(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := sriovv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add SR-IOV scheme: %v", err)
	}

	network := &sriovv1.OVSNetwork{ObjectMeta: metav1.ObjectMeta{
		Name:      "hwol-ovsnet",
		Namespace: "openshift-sriov-network-operator",
	}}
	apiClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(network).Build()
	builder := &OvsNetworkBuilder{Definition: network.DeepCopy(), apiClient: apiClient}

	err := builder.Delete()
	if err == nil || !strings.Contains(err.Error(), "not owned by the HWOL suite") {
		t.Fatalf("expected ownership error, got: %v", err)
	}

	if _, err := builder.Get(); err != nil {
		t.Fatalf("expected unowned OVSNetwork to remain, got: %v", err)
	}
}
