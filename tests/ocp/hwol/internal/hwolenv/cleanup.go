package hwolenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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

// validateUnclaimedNode is shared by setup and optional OVS recovery so both
// reject the same policies, pool selectors, and ambiguous pool targets.
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

// OvsOffloadBaseline records the original node identity and OVSDB setting.
// An empty Value means the key was absent, distinct from an explicit false.
type OvsOffloadBaseline struct {
	NodeUID types.UID
	Value   string
}

// CaptureOvsOffloadBaseline records the OVSDB setting on each selected node before setup.
func CaptureOvsOffloadBaseline(mcpLabel, image string) (map[string]OvsOffloadBaseline, error) {
	nodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to list HWOL nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes with HWOL MCP label %s", mcpLabel)
	}

	baseline := make(map[string]OvsOffloadBaseline, len(nodes))
	for _, node := range nodes {
		if node == nil || node.Object == nil || node.Object.UID == "" {
			return nil, fmt.Errorf("cannot verify OVS baseline node identity")
		}
		value, err := readOvsOffload(node.Object.Name, image)
		if err != nil {
			return nil, err
		}
		current, err := assertOvsNodeIdentity(node.Object.Name, node.Object.UID)
		if err != nil {
			return nil, err
		}
		baseline[node.Object.Name] = OvsOffloadBaseline{NodeUID: current.UID, Value: value}
	}

	return baseline, nil
}

// VerifyOvsOffloadBaseline fails closed if test teardown leaves a changed OVSDB setting.
// Restoring it can require an OVS restart, which must not be done automatically on shared nodes.
func VerifyOvsOffloadBaseline(mcpLabel, image string, baseline map[string]OvsOffloadBaseline) error {
	current, err := CaptureOvsOffloadBaseline(mcpLabel, image)
	if err != nil {
		return err
	}

	return compareOvsOffloadBaseline(current, baseline)
}

func compareOvsOffloadBaseline(current, baseline map[string]OvsOffloadBaseline) error {
	if len(current) != len(baseline) {
		return fmt.Errorf("HWOL MCP node membership changed during the test")
	}

	for node, want := range baseline {
		got, exists := current[node]
		if !exists {
			return fmt.Errorf("baseline HWOL node %s is no longer selected", node)
		}

		if want.NodeUID == "" || got.NodeUID != want.NodeUID {
			return fmt.Errorf("baseline OVS node %s was replaced or its identity is missing", node)
		}
		if got.Value != want.Value {
			return fmt.Errorf("node %s OVS hw-offload changed from %q to %q after cleanup; "+
				"preserve diagnostics and restore it with a controlled OVS restart", node, want.Value, got.Value)
		}
	}

	return nil
}

const ovsRestartReadyScript = `
set -eu
systemctl --no-pager is-active --quiet ovs-vswitchd.service
pid="$(systemctl --no-pager show ovs-vswitchd -p MainPID --value)"
test "$pid" -gt 0
test "$pid" != "$1"
ovs-vsctl --timeout=10 --if-exists get Open_vSwitch . other_config:hw-offload
`

// RestoreOvsOffloadBaseline is an opt-in recovery for dedicated test nodes.
// Restarting ovs-vswitchd can interrupt primary networking on the selected node.
func RestoreOvsOffloadBaseline(operatorNS, mcpLabel, image string, baseline map[string]OvsOffloadBaseline) error {
	current, err := CaptureOvsOffloadBaseline(mcpLabel, image)
	if err != nil {
		return err
	}

	if len(current) != len(baseline) {
		return fmt.Errorf("HWOL MCP node membership changed during the test")
	}

	for nodeName, want := range baseline {
		got, exists := current[nodeName]
		if !exists {
			return fmt.Errorf("baseline HWOL node %s is no longer selected", nodeName)
		}

		if want.NodeUID == "" || got.NodeUID != want.NodeUID {
			return fmt.Errorf("refusing OVS recovery: node %s was replaced or its identity is missing", nodeName)
		}
		if got.Value == want.Value {
			continue
		}

		if got.Value != "true" || (want.Value != "" && want.Value != "false") {
			return fmt.Errorf("refusing to restore OVS hw-offload on %s from %q to %q", nodeName, got.Value, want.Value)
		}
	}

	for nodeName, want := range baseline {
		if current[nodeName].Value == want.Value {
			continue
		}
		if err := restartNodeOVSWithOffloadBaseline(operatorNS, mcpLabel, nodeName, want); err != nil {
			return err
		}
	}

	return VerifyOvsOffloadBaseline(mcpLabel, image, baseline)
}

func assertOvsNodeIdentity(nodeName string, uid types.UID) (*corev1.Node, error) {
	if APIClient == nil || APIClient.Client == nil {
		return nil, fmt.Errorf("API client is required to verify node %s", nodeName)
	}
	if uid == "" {
		return nil, fmt.Errorf("node %s has no recorded UID", nodeName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	node := &corev1.Node{}
	if err := APIClient.Client.Get(ctx, runtimeClient.ObjectKey{Name: nodeName}, node); err != nil {
		return nil, fmt.Errorf("cannot verify node %s identity: %w", nodeName, err)
	}
	if node.UID != uid {
		return nil, fmt.Errorf("node %s was replaced since OVS baseline capture", nodeName)
	}
	return node, nil
}

func validateOvsRecoveryNode(node *corev1.Node, mcpLabel string, uid types.UID,
	policies []sriovv1.SriovNetworkNodePolicy, pools []sriovv1.SriovNetworkPoolConfig) error {
	if node == nil || uid == "" || node.UID != uid || !labels.SelectorFromSet(MCPNodeSelector(mcpLabel)).Matches(labels.Set(node.Labels)) {
		return fmt.Errorf("refusing OVS recovery: node identity or selection changed")
	}
	ready := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			ready = true
		}
	}
	if !ready {
		return fmt.Errorf("refusing OVS restart while node %s is not Ready", node.Name)
	}
	if err := validateUnclaimedNode(node, policies, pools); err != nil {
		return fmt.Errorf("refusing OVS recovery: %w", err)
	}
	return nil
}

func checkOvsRecoveryNode(operatorNS, mcpLabel, nodeName string, uid types.UID) error {
	node, err := assertOvsNodeIdentity(nodeName, uid)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var policies sriovv1.SriovNetworkNodePolicyList
	if err := APIClient.Client.List(ctx, &policies, runtimeClient.InNamespace(operatorNS)); err != nil {
		return fmt.Errorf("failed to inspect SR-IOV node policies: %w", err)
	}
	var pools sriovv1.SriovNetworkPoolConfigList
	if err := APIClient.Client.List(ctx, &pools, runtimeClient.InNamespace(operatorNS)); err != nil {
		return fmt.Errorf("failed to inspect SR-IOV pool configs: %w", err)
	}
	return validateOvsRecoveryNode(node, mcpLabel, uid, policies.Items, pools.Items)
}

func restartNodeOVSWithOffloadBaseline(operatorNS, mcpLabel, nodeName string, want OvsOffloadBaseline) error {
	daemon, err := nodeConfigDaemon(operatorNS, nodeName)
	if err != nil {
		return err
	}
	checkSafe := func() error { return checkOvsRecoveryNode(operatorNS, mcpLabel, nodeName, want.NodeUID) }
	if err := checkSafe(); err != nil {
		return err
	}
	pid, err := startOvsBaselineRestart(nodeName, want.Value, func(command []string, timeout time.Duration) (bytes.Buffer, error) {
		return daemon.ExecCommandWithTimeout(command, timeout)
	}, checkSafe)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, tsparams.DefaultTimeout, true,
		func(ctx context.Context) (bool, error) {
			node, err := nodes.Pull(APIClient, nodeName)
			if err != nil || node == nil || node.Object == nil {
				return false, nil
			}
			if node.Object.UID != want.NodeUID {
				return false, fmt.Errorf("node %s was replaced during OVS restart", nodeName)
			}

			ready := false
			for _, condition := range node.Object.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == "True" {
					ready = true
				}
			}
			if !ready {
				return false, nil
			}

			daemon, err := nodeConfigDaemon(operatorNS, nodeName)
			if err != nil {
				return false, nil
			}

			out, err := daemon.ExecCommandWithTimeout([]string{
				"chroot", "/host", "sh", "-c", ovsRestartReadyScript, "sh", pid,
			}, 20*time.Second)
			if err != nil {
				return false, nil
			}

			value, err := parseOvsOffloadValue(out.String())
			return err == nil && value == want.Value, nil
		})
	if err != nil {
		return fmt.Errorf("OVS restart or baseline verification failed on %s: %w", nodeName, err)
	}

	return nil
}

func startOvsBaselineRestart(nodeName, want string, exec func([]string, time.Duration) (bytes.Buffer, error),
	checkSafe func() error) (string, error) {
	if want != "" && want != "false" {
		return "", fmt.Errorf("refusing unsupported OVS baseline %q on %s", want, nodeName)
	}
	if exec == nil || checkSafe == nil {
		return "", fmt.Errorf("OVS recovery command and safety check are required")
	}
	unit, err := exec(
		[]string{"chroot", "/host", "env", "SYSTEMD_COLORS=0", "systemctl", "--no-pager", "cat", "ovs-vswitchd.service"}, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to inspect OVS startup configuration on %s: %w", nodeName, err)
	}
	setting, err := exec([]string{
		"chroot", "/host", "ovs-vsctl", "--timeout=10", "--if-exists", "get", "Open_vSwitch", ".", "other_config:hw-offload",
	}, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to recheck OVS offload on %s: %w", nodeName, err)
	}
	if err := validateOvsRestartConfig(unit.String(), setting.String()); err != nil {
		return "", fmt.Errorf("refusing OVS restart on %s: %w", nodeName, err)
	}

	oldPID, err := exec(
		[]string{"chroot", "/host", "systemctl", "--no-pager", "show", "ovs-vswitchd", "-p", "MainPID", "--value"}, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to read OVS PID on %s: %w", nodeName, err)
	}

	pid := strings.TrimSpace(oldPID.String())
	if n, err := strconv.Atoi(pid); err != nil || n <= 0 {
		return "", fmt.Errorf("invalid OVS PID %q on %s", pid, nodeName)
	}
	if err := checkSafe(); err != nil {
		return "", fmt.Errorf("refusing OVS baseline change on %s: %w", nodeName, err)
	}

	args := []string{"chroot", "/host", "ovs-vsctl", "--timeout=10", "--no-wait"}
	if want == "" {
		args = append(args, "--if-exists", "remove", "Open_vSwitch", ".", "other_config", "hw-offload")
	} else {
		args = append(args, "set", "Open_vSwitch", ".", "other_config:hw-offload=false")
	}

	if _, err := exec(args, 30*time.Second); err != nil {
		return "", fmt.Errorf("OVS offload write on %s failed or timed out; state may have changed: %w", nodeName, err)
	}
	if err := checkSafe(); err != nil {
		return "", fmt.Errorf("OVS baseline changed on %s but restart refused; inspect state: %w", nodeName, err)
	}

	_, restartErr := exec(
		[]string{"chroot", "/host", "systemctl", "--no-block", "restart", "ovs-vswitchd.service"}, 30*time.Second)
	if restartErr != nil {
		return "", fmt.Errorf("OVS restart request on %s failed or is uncertain; inspect OVS configuration and service: %w", nodeName, restartErr)
	}
	return pid, nil
}

func validateOvsRestartConfig(unit, output string) error {
	if strings.TrimSpace(unit) == "" || strings.Contains(unit, "\x1b") || strings.Contains(strings.ToLower(unit), "hw-offload") {
		return fmt.Errorf("OVS startup configuration is unavailable or mentions hw-offload")
	}
	value, err := parseOvsOffloadValue(output)
	if err != nil || value != "true" {
		return fmt.Errorf("OVS hw-offload is not the expected stale true value")
	}
	return nil
}

func nodeConfigDaemon(operatorNS, nodeName string) (*pod.Builder, error) {
	pods, err := pod.List(APIClient, operatorNS, metav1.ListOptions{
		LabelSelector: "app=sriov-network-config-daemon",
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find SR-IOV config daemon on %s: %v", nodeName, err)
	}

	for _, daemon := range pods {
		if daemon.Object != nil && daemon.Object.Status.Phase == corev1.PodRunning && daemon.Object.DeletionTimestamp == nil {
			return daemon, nil
		}
	}

	return nil, fmt.Errorf("no running SR-IOV config daemon on %s", nodeName)
}

func readOvsOffload(nodeName, image string) (string, error) {
	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return "", err
	}

	out, execErr := debugPod.ExecCommandWithTimeout([]string{
		"chroot", "/host", "ovs-vsctl", "--timeout=10", "--if-exists", "get", "Open_vSwitch", ".", "other_config:hw-offload",
	}, 30*time.Second)
	deleteErr := DeleteOwnedPod(debugPod, tsparams.DefaultTimeout)
	if execErr != nil {
		readErr := fmt.Errorf("failed to read OVS hw-offload on %s: %w (out=%s)", nodeName, execErr, out.String())
		if deleteErr != nil {
			return "", errors.Join(readErr, fmt.Errorf("failed to delete OVS debug pod on %s: %w", nodeName, deleteErr))
		}
		return "", readErr
	}

	if deleteErr != nil {
		return "", fmt.Errorf("failed to delete OVS debug pod on %s: %w", nodeName, deleteErr)
	}

	value, err := parseOvsOffloadValue(out.String())
	if err != nil {
		return "", fmt.Errorf("node %s: %w", nodeName, err)
	}

	return value, nil
}

func parseOvsOffloadValue(output string) (string, error) {
	value := strings.Trim(strings.TrimSpace(output), `"`)
	if value != "" && value != "true" && value != "false" {
		return "", fmt.Errorf("unexpected OVS hw-offload value %q", value)
	}

	return value, nil
}
