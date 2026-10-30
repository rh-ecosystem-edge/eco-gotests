package hwolenv

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// ValidateSwitchdevPreconditions refuses to alter nodes already managed by another SR-IOV policy.
func ValidateSwitchdevPreconditions(operatorNS, mcpLabel string, device hwolconfig.DeviceConfig, vfNum int) error {
	if APIClient == nil || APIClient.Client == nil {
		return fmt.Errorf("API client is required")
	}
	if err := APIClient.AttachScheme(sriovv1.AddToScheme); err != nil {
		return fmt.Errorf("cannot register SR-IOV types: %w", err)
	}
	nodeList, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil || len(nodeList) == 0 {
		return fmt.Errorf("cannot list HWOL MCP nodes %s: %v", mcpLabel, err)
	}
	var policies sriovv1.SriovNetworkNodePolicyList
	if err := APIClient.Client.List(context.TODO(), &policies, runtimeClient.InNamespace(operatorNS)); err != nil {
		return fmt.Errorf("cannot inspect SR-IOV policies: %w", err)
	}
	var pools sriovv1.SriovNetworkPoolConfigList
	if err := APIClient.Client.List(context.TODO(), &pools, runtimeClient.InNamespace(operatorNS)); err != nil {
		return fmt.Errorf("cannot inspect SR-IOV pool configs: %w", err)
	}
	for _, node := range nodeList {
		if node == nil || node.Object == nil {
			return fmt.Errorf("MCP node object is unavailable")
		}
		state := &sriovv1.SriovNetworkNodeState{}
		if err := APIClient.Client.Get(context.TODO(), runtimeClient.ObjectKey{
			Name: node.Object.Name, Namespace: operatorNS,
		}, state); err != nil {
			return fmt.Errorf("cannot inspect SR-IOV state on %s: %w", node.Object.Name, err)
		}
		if err := validateNodeFoundationState(node.Object, state, device, vfNum, policies.Items, pools.Items); err != nil {
			return err
		}
	}
	return nil
}

func validateNodeFoundationState(node *corev1.Node, state *sriovv1.SriovNetworkNodeState,
	device hwolconfig.DeviceConfig, vfNum int, policies []sriovv1.SriovNetworkNodePolicy,
	pools []sriovv1.SriovNetworkPoolConfig) error {
	if node == nil || state == nil || node.Name == "" || state.Name != node.Name || vfNum < 1 {
		return fmt.Errorf("invalid node, SR-IOV state or VF count")
	}
	if state.Status.SyncStatus != "Succeeded" || len(state.Spec.Interfaces) != 0 {
		return fmt.Errorf("node %s has non-idle or pre-existing desired SR-IOV interfaces", node.Name)
	}
	if err := validateUnclaimedNode(node, policies, pools); err != nil {
		return fmt.Errorf("refusing HWOL setup: %w", err)
	}
	iface, err := findStatusInterface(state.Status.Interfaces, device.InterfaceName)
	if err != nil {
		return fmt.Errorf("node %s: %w", node.Name, err)
	}
	if !strings.EqualFold(iface.Vendor, device.Vendor) || !strings.EqualFold(iface.DeviceID, device.DeviceID) || iface.PciAddress == "" {
		return fmt.Errorf("node %s PF %s does not match configured PCI identity", node.Name, device.InterfaceName)
	}
	if iface.NumVfs != 0 || len(iface.VFs) != 0 || iface.ExternallyManaged ||
		(iface.EswitchMode != "" && iface.EswitchMode != "legacy") || iface.TotalVfs < vfNum {
		return fmt.Errorf("node %s PF %s is not an unused legacy PF with capacity for %d VFs", node.Name, device.InterfaceName, vfNum)
	}
	for _, bridge := range state.Status.Bridges.OVS {
		for _, uplink := range bridge.Uplinks {
			if uplink.Name == iface.Name || uplink.PciAddress == iface.PciAddress {
				return fmt.Errorf("node %s PF %s is already attached to managed bridge %s", node.Name, iface.Name, bridge.Name)
			}
		}
	}
	return nil
}

// validateUnclaimedNode rejects selecting policies, pool selectors and ambiguous pool targets.
func validateUnclaimedNode(node *corev1.Node, policies []sriovv1.SriovNetworkNodePolicy,
	pools []sriovv1.SriovNetworkPoolConfig) error {
	for _, policy := range policies {
		if len(policy.Spec.NodeSelector) == 0 || labels.SelectorFromSet(policy.Spec.NodeSelector).Matches(labels.Set(node.Labels)) {
			return fmt.Errorf("SR-IOV policy %s selects node %s", policy.Name, node.Name)
		}
	}
	for _, pool := range pools {
		poolName := pool.Spec.OvsHardwareOffloadConfig.Name
		if poolName != "" {
			if _, selected := node.Labels["node-role.kubernetes.io/"+poolName]; selected {
				return fmt.Errorf("SR-IOV pool config %s selects node %s", pool.Name, node.Name)
			}
		}
		if pool.Spec.NodeSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
			if err != nil {
				return fmt.Errorf("invalid selector in SR-IOV pool config %s: %w", pool.Name, err)
			}
			if selector.Matches(labels.Set(node.Labels)) {
				return fmt.Errorf("SR-IOV pool config %s selects node %s", pool.Name, node.Name)
			}
		} else if poolName == "" {
			return fmt.Errorf("SR-IOV pool config %s has no identifiable target", pool.Name)
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
