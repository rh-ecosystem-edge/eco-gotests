package hwolenv

import (
	"context"
	"fmt"
	"sync"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ownedResourceKey struct {
	kind, namespace, name, runID string
}

var createdResourceUIDs sync.Map

func resourceKey(obj runtimeClient.Object, runID string) ownedResourceKey {
	return ownedResourceKey{kind: fmt.Sprintf("%T", obj), namespace: obj.GetNamespace(), name: obj.GetName(), runID: runID}
}

// EnsureTestResourcesAbsent prevents this suite from adopting and deleting existing resources.
func EnsureTestResourcesAbsent(operatorNS string) error {
	if APIClient == nil {
		return fmt.Errorf("API client cannot be nil")
	}

	if err := APIClient.AttachScheme(sriovv1.AddToScheme); err != nil {
		return fmt.Errorf("failed to register SR-IOV types: %w", err)
	}

	resources := []struct {
		name string
		obj  runtimeClient.Object
	}{
		{tsparams.OvsNetworkName, &sriovv1.OVSNetwork{}},
		{tsparams.SriovNetworkName, &sriovv1.SriovNetwork{}},
		{tsparams.PolicyName, &sriovv1.SriovNetworkNodePolicy{}},
		{tsparams.PoolConfigName, &sriovv1.SriovNetworkPoolConfig{}},
	}

	for _, resource := range resources {
		err := APIClient.Client.Get(context.TODO(), runtimeClient.ObjectKey{
			Name: resource.name, Namespace: operatorNS,
		}, resource.obj)
		if err == nil {
			return fmt.Errorf("refusing to overwrite existing HWOL resource %s/%s", operatorNS, resource.name)
		}

		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to check HWOL resource %s/%s: %w", operatorNS, resource.name, err)
		}
	}

	return nil
}

func createOwnedTestResource(client runtimeClient.Client, obj runtimeClient.Object, runID string) error {
	if client == nil || obj == nil || obj.GetName() == "" || obj.GetNamespace() == "" || runID == "" ||
		obj.GetLabels()[tsparams.TestRunLabelKey] != runID {
		return fmt.Errorf("client and labelled, named HWOL resource with run ID are required")
	}
	// The API server's atomic Create rejects an existing resource. Builder.Create
	// can silently adopt one that appeared after a separate Exists check.
	if err := client.Create(context.TODO(), obj); err != nil {
		return err
	}
	if obj.GetUID() == "" {
		return fmt.Errorf("created HWOL resource %s/%s has no UID; refusing unsafe cleanup", obj.GetNamespace(), obj.GetName())
	}
	createdResourceUIDs.Store(resourceKey(obj, runID), obj.GetUID())
	return nil
}

// DeleteOwnedTestResource deletes only an object created by this suite execution.
func DeleteOwnedTestResource(operatorNS, name, runID string, obj runtimeClient.Object) error {
	if APIClient == nil {
		return fmt.Errorf("API client cannot be nil")
	}

	return deleteOwnedTestResource(APIClient.Client, operatorNS, name, runID, obj)
}

func deleteOwnedTestResource(client runtimeClient.Client, operatorNS, name, runID string, obj runtimeClient.Object) error {
	if client == nil || operatorNS == "" || name == "" || runID == "" || obj == nil {
		return fmt.Errorf("client, namespace, resource name, run ID and object are required")
	}

	err := client.Get(context.TODO(), runtimeClient.ObjectKey{Name: name, Namespace: operatorNS}, obj)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to read %s/%s before cleanup: %w", operatorNS, name, err)
	}

	if obj.GetLabels()[tsparams.TestRunLabelKey] != runID {
		return fmt.Errorf("refusing to delete %s/%s: not owned by this HWOL run", operatorNS, name)
	}

	uid := obj.GetUID()
	if uid == "" {
		return fmt.Errorf("refusing to delete %s/%s without a UID", operatorNS, name)
	}
	expected, found := createdResourceUIDs.Load(resourceKey(obj, runID))
	if !found || expected.(types.UID) != uid {
		return fmt.Errorf("refusing to delete %s/%s: original created UID is missing or changed", operatorNS, name)
	}
	rv := obj.GetResourceVersion()

	err = client.Delete(context.TODO(), obj, &runtimeClient.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &rv},
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete owned HWOL resource %s/%s: %w", operatorNS, name, err)
	}

	return nil
}
