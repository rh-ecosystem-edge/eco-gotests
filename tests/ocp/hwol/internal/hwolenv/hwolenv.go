// Package hwolenv provides helpers for OCP HWOL switchdev / OVS bridge setup.
package hwolenv

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrSwitchdevCleanupTimeout identifies the post-policy-deletion stability
// timeout that may be accepted after opt-in reboot recovery verifies cleanup.
// Other cleanup errors must not be converted into passing test results.
var ErrSwitchdevCleanupTimeout = errors.New(
	"HWOL cleanup timed out waiting for SR-IOV/MCP stable after switchdev teardown")

// MCPNodeSelector returns the node-role label selector for the HWOL MCP pool.
func MCPNodeSelector(mcpLabel string) map[string]string {
	return map[string]string{fmt.Sprintf("node-role.kubernetes.io/%s", mcpLabel): ""}
}

// hwolResourceLabels returns the label identifying CRs created by this suite.
func hwolResourceLabels() map[string]string {
	return map[string]string{tsparams.TestResourceLabelKey: tsparams.TestResourceLabelValue}
}

// ensureHwolResourceOwnership rejects a fixed-name resource not created by HWOL.
func ensureHwolResourceOwnership(kind, name string, resourceLabels map[string]string) error {
	if resourceLabels[tsparams.TestResourceLabelKey] == tsparams.TestResourceLabelValue {
		return nil
	}

	return fmt.Errorf(
		"%s %s exists but is not owned by the HWOL suite; expected label %s=%s",
		kind, name, tsparams.TestResourceLabelKey, tsparams.TestResourceLabelValue)
}

// EnsureHwolOperatorConfig verifies the cluster-wide SriovOperatorConfig is
// already suitable for the manageSoftwareBridges + mlx5 HWOL path. It must not
// modify the object: HWOL cleanup owns only its fixed-name resources and cannot
// safely restore administrator-owned operator settings after an interruption.
func EnsureHwolOperatorConfig(operatorNS string) error {
	opCfg, err := sriov.PullOperatorConfig(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	if !opCfg.Definition.Spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] {
		return fmt.Errorf(
			"SriovOperatorConfig must enable featureGates.%s before running HWOL; "+
				"configure a dedicated HWOL MCP instead of letting the suite mutate shared operator settings",
			tsparams.ManageSoftwareBridgesGate)
	}

	if !opCfg.Definition.Spec.EnableOvsOffload {
		return fmt.Errorf("SriovOperatorConfig must enable enableOvsOffload before running HWOL")
	}

	for _, plugin := range opCfg.Definition.Spec.DisablePlugins {
		if plugin == "mellanox" {
			return fmt.Errorf(
				"SriovOperatorConfig disables the mellanox plugin; enable it before running HWOL on a dedicated MCP")
		}
	}

	return nil
}

// EnsureSwitchdevFoundation configures operator, pool, and switchdev policy and waits for
// SR-IOV/MCP stability. It does not create SriovNetwork or OVSNetwork CRs.
func EnsureSwitchdevFoundation(
	operatorNS, mcpLabel string,
	device hwolconfig.DeviceConfig,
	vfNum int,
) error {
	if err := ValidateHwolPreconditions(operatorNS, mcpLabel, device); err != nil {
		return err
	}

	mcpNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(mcpNodes) == 0 {
		return fmt.Errorf("no nodes with label node-role.kubernetes.io/%s", mcpLabel)
	}

	if _, err := CreateOvsOffloadPoolConfig(tsparams.PoolConfigName, operatorNS, mcpLabel); err != nil {
		return err
	}

	if _, err := CreateSwitchdevPolicy(
		tsparams.PolicyName,
		operatorNS,
		tsparams.ResourceName,
		device,
		vfNum,
		MCPNodeSelector(mcpLabel),
	); err != nil {
		return err
	}

	return WaitForSriovAndMCPStable(
		operatorNS,
		mcpLabel,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration,
	)
}

// ValidateHwolPreconditions performs only read-only checks required before an
// HWOL run. It is called from the synchronized suite setup and again before
// resource creation so a shared PF is rejected without creating test resources.
func ValidateHwolPreconditions(operatorNS, mcpLabel string, device hwolconfig.DeviceConfig) error {
	if err := ensureHwolTargetIsDedicated(operatorNS, mcpLabel, device.InterfaceName); err != nil {
		return err
	}

	if err := ensureHwolMCPIsDedicated(operatorNS, mcpLabel); err != nil {
		return err
	}

	return EnsureHwolOperatorConfig(operatorNS)
}

// ensureHwolTargetIsDedicated rejects a PF that is already claimed by an
// administrator-owned policy. Replacing or coexisting with that policy can
// force an in-place switchdev reset and leave a user-owned OVS bridge attached.
func ensureHwolTargetIsDedicated(operatorNS, mcpLabel, pfName string) error {
	policies, err := sriov.ListPolicy(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to list SriovNetworkNodePolicies for HWOL preflight: %w", err)
	}

	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list HWOL MCP nodes for policy preflight: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes with label node-role.kubernetes.io/%s", mcpLabel)
	}

	for _, policy := range policies {
		if policy.Object.Name == tsparams.PolicyName {
			if err := ensureHwolResourceOwnership(
				"SriovNetworkNodePolicy", policy.Object.Name, policy.Object.Labels,
			); err != nil {
				return err
			}

			continue
		}

		selectsPF, err := policySelectsHwolPF(policy.Object, workerNodes, operatorNS, pfName)
		if err != nil {
			return err
		}

		if selectsPF {
			return fmt.Errorf(
				"HWOL requires a dedicated PF: %s is already selected by SriovNetworkNodePolicy %s; "+
					"do not run against a shared or externally OVS-managed PF",
				pfName, policy.Object.Name)
		}
	}

	return nil
}

// policySelectsHwolPF reports whether a non-HWOL policy selects the target PF on an HWOL MCP node.
func policySelectsHwolPF(
	policy *sriovv1.SriovNetworkNodePolicy,
	workerNodes []*nodes.Builder,
	operatorNS, pfName string,
) (bool, error) {
	for _, node := range workerNodes {
		if !nodeMatchesSelector(node.Object.Labels, policy.Spec.NodeSelector) {
			continue
		}

		state := sriov.NewNetworkNodeStateBuilder(APIClient, node.Object.Name, operatorNS)
		if err := state.Discover(); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}

			return false, fmt.Errorf(
				"failed to discover SriovNetworkNodeState for %s while checking policy %s: %w",
				node.Object.Name, policy.Name, err)
		}

		iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
		if err == nil && nicSelectorMatchesInterface(policy.Spec.NicSelector, iface) {
			return true, nil
		}
	}

	return false, nil
}

// nodeMatchesSelector reports whether nodeLabels satisfy the policy node selector.
func nodeMatchesSelector(nodeLabels, selector map[string]string) bool {
	for key, value := range selector {
		if nodeLabels[key] != value {
			return false
		}
	}

	return true
}

// nicSelectorMatchesInterface reports whether every specified NIC selector matches iface.
func nicSelectorMatchesInterface(selector sriovv1.SriovNetworkNicSelector, iface *sriovv1.InterfaceExt) bool {
	hasSelector := false

	if len(selector.PfNames) > 0 {
		hasSelector = true
		matchedPF := false

		for _, configuredPF := range selector.PfNames {
			if strings.SplitN(configuredPF, "#", 2)[0] == iface.Name {
				matchedPF = true

				break
			}
		}

		if !matchedPF {
			return false
		}
	}

	if len(selector.RootDevices) > 0 {
		hasSelector = true
		matchedRootDevice := false

		for _, rootDevice := range selector.RootDevices {
			if strings.EqualFold(rootDevice, iface.PciAddress) {
				matchedRootDevice = true

				break
			}
		}

		if !matchedRootDevice {
			return false
		}
	}

	if selector.Vendor != "" {
		hasSelector = true

		if !strings.EqualFold(selector.Vendor, iface.Vendor) {
			return false
		}
	}

	if selector.DeviceID != "" {
		hasSelector = true

		if !strings.EqualFold(selector.DeviceID, iface.DeviceID) {
			return false
		}
	}

	return hasSelector
}

// ensureHwolMCPIsDedicated rejects an MCP already selected by another
// OVS-hardware-offload PoolConfig. Such PoolConfigs share the operator-rendered
// 00-<mcp>-ovs-hw-offload MachineConfig. Removing the HWOL PoolConfig would
// otherwise also disable an administrator-owned HW offload configuration.
func ensureHwolMCPIsDedicated(operatorNS, mcpName string) error {
	poolConfigs, err := sriov.ListPoolConfigs(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to list SriovNetworkPoolConfigs for HWOL preflight: %w", err)
	}

	for _, poolConfig := range poolConfigs {
		if poolConfig.Object.Name == tsparams.PoolConfigName {
			if err := ensureHwolResourceOwnership(
				"SriovNetworkPoolConfig", poolConfig.Object.Name, poolConfig.Object.Labels,
			); err != nil {
				return err
			}

			continue
		}

		if poolConfig.Object.Spec.OvsHardwareOffloadConfig.Name == mcpName {
			return fmt.Errorf(
				"HWOL requires a dedicated MCP: SriovNetworkPoolConfig %s already enables OVS hardware offload on %s; "+
					"do not share its operator-owned MachineConfig with the suite",
				poolConfig.Object.Name, mcpName)
		}
	}

	return nil
}

// OvsOffloadMachineConfigName returns the name created by the SR-IOV Operator
// for a SriovNetworkPoolConfig that targets an MCP for OVS hardware offload.
func OvsOffloadMachineConfigName(mcpName string) string {
	return tsparams.OvsOffloadMachineConfigPrefix + mcpName + tsparams.OvsOffloadMachineConfigSuffix
}

// CreateOvsOffloadPoolConfig creates SriovNetworkPoolConfig with only
// ovsHardwareOffloadConfig.name set to the MCP name.
// Do not set nodeSelector/maxUnavailable together with OvsHardwareOffload — webhook rejects it.
func CreateOvsOffloadPoolConfig(name, operatorNS, mcpName string) (*sriov.PoolConfigBuilder, error) {
	builder := sriov.NewPoolConfigBuilder(APIClient, name, operatorNS)
	if builder == nil {
		return nil, fmt.Errorf("failed to init SriovNetworkPoolConfig builder")
	}

	builder.Definition.Spec.OvsHardwareOffloadConfig = sriovv1.OvsHardwareOffloadConfig{
		Name: mcpName,
	}
	builder.Definition.Labels = hwolResourceLabels()

	if builder.Exists() {
		if err := ensureHwolResourceOwnership("SriovNetworkPoolConfig", name, builder.Object.Labels); err != nil {
			return nil, err
		}

		builder.Definition.ResourceVersion = builder.Object.ResourceVersion

		updated, err := builder.Update()
		if err != nil {
			return nil, fmt.Errorf("failed to update SriovNetworkPoolConfig %s: %w", name, err)
		}

		return updated, nil
	}

	created, err := builder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkPoolConfig %s: %w", name, err)
	}

	return created, nil
}

// CreateSwitchdevPolicy creates a switchdev SriovNetworkNodePolicy with OVS bridge management.
// VF0 is reserved as the management port (pfNames range starts at 1).
// If a policy already exists with a different numVfs/pfNames selector, it is replaced.
func CreateSwitchdevPolicy(
	name, operatorNS, resourceName string,
	device hwolconfig.DeviceConfig,
	vfNum int,
	nodeSelector map[string]string,
) (*sriov.PolicyBuilder, error) {
	if vfNum < 2 {
		return nil, fmt.Errorf("vfNum must be >= 2 to reserve VF0 as management port, got %d", vfNum)
	}

	pfSelector := fmt.Sprintf("%s#1-%d", device.InterfaceName, vfNum-1)

	policy := sriov.NewPolicyBuilder(
		APIClient,
		name,
		operatorNS,
		resourceName,
		vfNum,
		[]string{pfSelector},
		nodeSelector,
	).WithDevType("netdevice").WithMTU(1500)

	policy.Definition.Spec.NicSelector.Vendor = device.Vendor
	policy.Definition.Spec.NicSelector.DeviceID = device.DeviceID
	policy.Definition.Spec.EswitchMode = tsparams.SwitchdevMode
	policy.Definition.Spec.Bridge = sriovv1.Bridge{
		OVS: &sriovv1.OVSConfig{},
	}
	policy.Definition.Labels = hwolResourceLabels()

	if policy.Exists() {
		if err := ensureHwolResourceOwnership("SriovNetworkNodePolicy", name, policy.Object.Labels); err != nil {
			return nil, err
		}

		existing := policy.Object.Spec
		same := existing.NumVfs == vfNum &&
			existing.EswitchMode == tsparams.SwitchdevMode &&
			existing.Bridge.OVS != nil &&
			len(existing.NicSelector.PfNames) == 1 &&
			existing.NicSelector.PfNames[0] == pfSelector &&
			existing.NicSelector.Vendor == device.Vendor &&
			existing.NicSelector.DeviceID == device.DeviceID

		if same {
			klog.V(90).Infof("SriovNetworkNodePolicy %s already matches desired HWOL config", name)

			return policy, nil
		}

		klog.V(90).Infof(
			"Replacing SriovNetworkNodePolicy %s (numVfs/pfNames mismatch: have %d %v, want %d [%s])",
			name, existing.NumVfs, existing.NicSelector.PfNames, vfNum, pfSelector)

		if err := policy.Delete(); err != nil {
			return nil, fmt.Errorf("failed to delete outdated SriovNetworkNodePolicy %s: %w", name, err)
		}

		// Rebuild definition after delete; Exists()/Delete mutate builder state.
		policy = sriov.NewPolicyBuilder(
			APIClient,
			name,
			operatorNS,
			resourceName,
			vfNum,
			[]string{pfSelector},
			nodeSelector,
		).WithDevType("netdevice").WithMTU(1500)
		policy.Definition.Spec.NicSelector.Vendor = device.Vendor
		policy.Definition.Spec.NicSelector.DeviceID = device.DeviceID
		policy.Definition.Spec.EswitchMode = tsparams.SwitchdevMode
		policy.Definition.Spec.Bridge = sriovv1.Bridge{
			OVS: &sriovv1.OVSConfig{},
		}
		policy.Definition.Labels = hwolResourceLabels()
	}

	created, err := policy.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkNodePolicy %s: %w", name, err)
	}

	return created, nil
}

// CreateSriovNetwork creates a SriovNetwork for the HWOL resource in the test namespace.
// If ipam is empty, HostLocalIPAM is used. Callers may pass other IPAM JSON later
// (for example NV-IPAM) without changing this helper.
//
// Sriov CNI same-node HWOL is deferred: VF representors are not ports on the managed
// OVS bridge, so L2 between same-node pods fails. Keep this helper for attach coverage;
// the offload Entry that uses it is Pending until representor plumbing exists.
func CreateSriovNetwork(
	name, operatorNS, resourceName, networkNS, ipam string,
) (*sriov.NetworkBuilder, error) {
	network := sriov.NewNetworkBuilder(APIClient, name, operatorNS, networkNS, resourceName)
	if network == nil {
		return nil, fmt.Errorf("failed to init SriovNetwork builder")
	}

	if ipam == "" {
		ipam = HostLocalIPAM
	}

	network.Definition.Spec.IPAM = ipam
	network.Definition.Labels = hwolResourceLabels()

	if network.Exists() {
		if network.Object == nil {
			return nil, fmt.Errorf("failed to determine whether SriovNetwork %s is owned by HWOL", name)
		}

		if err := ensureHwolResourceOwnership("SriovNetwork", name, network.Object.Labels); err != nil {
			return nil, err
		}

		return network, nil
	}

	created, err := network.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create SriovNetwork %s: %w", name, err)
	}

	return created, nil
}

// WaitForSriovAndMCPStable waits for SR-IOV node states and the HWOL MCP to stabilize.
func WaitForSriovAndMCPStable(operatorNS, mcpName string, timeout, stableDuration time.Duration) error {
	return sriovoperator.WaitForSriovAndMCPStable(
		APIClient, timeout, stableDuration, mcpName, operatorNS)
}

// ListMCPWorkerNodes returns nodes matching the HWOL MCP role label.
func ListMCPWorkerNodes(mcpLabel string) ([]*nodes.Builder, error) {
	selector := labels.Set(MCPNodeSelector(mcpLabel)).String()

	return nodes.List(APIClient, metav1.ListOptions{LabelSelector: selector})
}

// ListMCPWorkerNodesWithPF returns MCP workers that expose pfName in their
// SriovNetworkNodeState. An MCP can contain workers with different NIC layouts.
func ListMCPWorkerNodesWithPF(operatorNS, mcpLabel, pfName string) ([]*nodes.Builder, error) {
	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	matchingNodes := make([]*nodes.Builder, 0, len(workerNodes))
	for _, node := range workerNodes {
		state := sriov.NewNetworkNodeStateBuilder(APIClient, node.Object.Name, operatorNS)
		if err := state.Discover(); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", node.Object.Name, err)
		}

		if _, err := findStatusInterface(state.Objects.Status.Interfaces, pfName); err == nil {
			matchingNodes = append(matchingNodes, node)
		}
	}

	if len(matchingNodes) == 0 {
		return nil, fmt.Errorf("no MCP-labeled workers expose interface %s", pfName)
	}

	return matchingNodes, nil
}

// AssertSwitchdevAndOVSBridge checks SriovNetworkNodeState for switchdev mode and a managed OVS bridge
// on the given PF for each MCP-labeled node.
func AssertSwitchdevAndOVSBridge(operatorNS, mcpLabel, pfName string) error {
	workerNodes, err := ListMCPWorkerNodesWithPF(operatorNS, mcpLabel, pfName)
	if err != nil {
		return err
	}

	for _, node := range workerNodes {
		nodeName := node.Object.Name
		state := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, operatorNS)

		if err := state.Discover(); err != nil {
			return fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", nodeName, err)
		}

		if state.Objects.Status.SyncStatus != "Succeeded" {
			return fmt.Errorf("node %s syncStatus is %q, want Succeeded",
				nodeName, state.Objects.Status.SyncStatus)
		}

		iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
		if err != nil {
			return fmt.Errorf("node %s: %w", nodeName, err)
		}

		if iface.EswitchMode != tsparams.SwitchdevMode {
			return fmt.Errorf("node %s interface %s eSwitchMode is %q, want %s",
				nodeName, pfName, iface.EswitchMode, tsparams.SwitchdevMode)
		}

		if err := assertOVSBridgeForPF(state.Objects.Status.Bridges, pfName, iface.PciAddress); err != nil {
			return fmt.Errorf("node %s: %w", nodeName, err)
		}

		klog.V(90).Infof("HWOL switchdev+OVS bridge OK on node %s pf %s pci %s",
			nodeName, pfName, iface.PciAddress)
	}

	return nil
}

func findStatusInterface(ifaces sriovv1.InterfaceExts, pfName string) (*sriovv1.InterfaceExt, error) {
	for i := range ifaces {
		if ifaces[i].Name == pfName {
			return &ifaces[i], nil
		}
	}

	return nil, fmt.Errorf("interface %s not found in SriovNetworkNodeState status", pfName)
}

// LookupManagedOVSBridge returns the managed OVS bridge name for pfName on the first MCP worker.
func LookupManagedOVSBridge(operatorNS, mcpLabel, pfName string) (string, error) {
	workerNodes, err := ListMCPWorkerNodesWithPF(operatorNS, mcpLabel, pfName)
	if err != nil {
		return "", err
	}

	nodeName := workerNodes[0].Object.Name
	state := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, operatorNS)

	if err := state.Discover(); err != nil {
		return "", fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", nodeName, err)
	}

	iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
	if err != nil {
		return "", fmt.Errorf("node %s: %w", nodeName, err)
	}

	for _, bridge := range state.Objects.Status.Bridges.OVS {
		for _, uplink := range bridge.Uplinks {
			if uplink.Name == pfName || uplink.PciAddress == iface.PciAddress {
				if bridge.Name == "" {
					return "", fmt.Errorf("node %s: OVS bridge for PF %s has empty name", nodeName, pfName)
				}

				return bridge.Name, nil
			}
		}
	}

	return "", fmt.Errorf("node %s: no managed OVS bridge for PF %s", nodeName, pfName)
}

func assertOVSBridgeForPF(bridges sriovv1.Bridges, pfName, pciAddress string) error {
	if len(bridges.OVS) == 0 {
		return fmt.Errorf("no OVS bridges in SriovNetworkNodeState status for PF %s", pfName)
	}

	for _, bridge := range bridges.OVS {
		for _, uplink := range bridge.Uplinks {
			if uplink.Name == pfName || uplink.PciAddress == pciAddress {
				return nil
			}
		}
	}

	return fmt.Errorf("no OVS bridge uplink matching PF %s (%s)", pfName, pciAddress)
}

var offloadedPacketsRE = regexp.MustCompile(`packets:(\d+)`)

// vfRepresentorOnBridgeScript maps each VF PCI address to its host representor
// netdev and asserts that representor is an ovs-vsctl port on $1.
const vfRepresentorOnBridgeScript = `
set -eu
bridge="$1"
shift
normalize_pci() {
  pci="$1"
  case "$pci" in
    *:*:*.*) echo "$pci" ;;
    *:*.*) echo "0000:$pci" ;;
    *) echo "$pci" ;;
  esac
}
pci_basename() {
  basename "$(normalize_pci "$1")"
}
for pci in "$@"; do
  full="$(normalize_pci "$pci")"
  short="$(pci_basename "$pci")"
  dev="/sys/bus/pci/devices/$full"
  if [ ! -e "$dev" ]; then
    echo "PCI device $full not found on host" >&2
    exit 1
  fi
  if [ ! -e "$dev/physfn" ]; then
    echo "PCI $full has no physfn (not a VF?)" >&2
    exit 1
  fi
  pf_pci="$(basename "$(readlink -f "$dev/physfn")")"
  pf_net=""
  for n in /sys/bus/pci/devices/"$pf_pci"/net/*; do
    [ -e "$n" ] || continue
    pf_net="$(basename "$n")"
    break
  done
  if [ -z "$pf_net" ]; then
    echo "no netdev for PF $pf_pci" >&2
    exit 1
  fi
  vf_idx=""
  for virtfn in /sys/bus/pci/devices/"$pf_pci"/virtfn*; do
    [ -e "$virtfn" ] || continue
    target="$(basename "$(readlink -f "$virtfn")")"
    if [ "$target" = "$short" ] || [ "$target" = "$(basename "$full")" ]; then
      vf_idx="${virtfn##*virtfn}"
      break
    fi
  done
  if [ -z "$vf_idx" ]; then
    echo "could not map PCI $full to a virtfn under PF $pf_pci" >&2
    exit 1
  fi
  rep="${pf_net}_${vf_idx}"
  if [ ! -d "/sys/class/net/$rep" ]; then
    echo "representor netdev $rep for PCI $full not found" >&2
    exit 1
  fi
  if ! ovs-vsctl list-ports "$bridge" | grep -qx "$rep"; then
    echo "representor $rep (PCI $full) is not a port on bridge $bridge" >&2
    echo "bridge ports:" >&2
    ovs-vsctl list-ports "$bridge" >&2 || true
    exit 1
  fi
  echo "OK $full -> $rep on $bridge"
done
`

// AssertVFRepresentorsOnBridge maps each VF PCI address to its host representor netdev
// (PF virtfnN → typically <pf>_<N>) and asserts that representor is an ovs-vsctl port
// on bridge. Used for the ovs CNI offload path before iperf.
func AssertVFRepresentorsOnBridge(nodeName, bridge, image string, pciAddrs ...string) error {
	if nodeName == "" {
		return fmt.Errorf("nodeName cannot be empty")
	}

	if bridge == "" {
		return fmt.Errorf("bridge cannot be empty")
	}

	if image == "" {
		return fmt.Errorf("debug pod image cannot be empty")
	}

	if len(pciAddrs) == 0 {
		return fmt.Errorf("at least one PCI address is required")
	}

	for _, pci := range pciAddrs {
		if strings.TrimSpace(pci) == "" {
			return fmt.Errorf("PCI address cannot be empty")
		}
	}

	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return err
	}

	defer func() {
		_, _ = debugPod.DeleteAndWait(tsparams.DefaultTimeout)
	}()

	args := []string{"chroot", "/host", "sh", "-c", vfRepresentorOnBridgeScript, "sh", bridge}
	args = append(args, pciAddrs...)

	out, err := debugPod.ExecCommand(args)
	if err != nil {
		return fmt.Errorf("VF representor-on-bridge check failed on %s bridge %s: %w (out=%s)",
			nodeName, bridge, err, out.String())
	}

	klog.V(90).Infof("VF representors on bridge %s (%s):\n%s", bridge, nodeName, out.String())

	return nil
}

// AssertOvsOffloadedFlows runs ovs-appctl dpctl/dump-flows type=offloaded on the node
// via a privileged hostNetwork debug pod with the host root mounted at /host.
func AssertOvsOffloadedFlows(nodeName, image string) error {
	if nodeName == "" {
		return fmt.Errorf("nodeName cannot be empty")
	}

	if image == "" {
		return fmt.Errorf("debug pod image cannot be empty")
	}

	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return err
	}

	defer func() {
		_, _ = debugPod.DeleteAndWait(tsparams.DefaultTimeout)
	}()

	out, err := debugPod.ExecCommand([]string{
		"chroot", "/host", "ovs-appctl", "dpctl/dump-flows", "--names", "type=offloaded",
	})
	if err != nil {
		return fmt.Errorf("ovs-appctl dpctl/dump-flows type=offloaded failed on %s: %w (out=%s)",
			nodeName, err, out.String())
	}

	flows := strings.TrimSpace(out.String())
	if flows == "" {
		return fmt.Errorf("no offloaded flows on node %s", nodeName)
	}

	matches := offloadedPacketsRE.FindAllStringSubmatch(flows, -1)
	if len(matches) == 0 {
		// Success when dump has offloaded flows but omits packets: (some OVS versions);
		// all-zero packets: counters still fail below.
		return nil
	}

	for _, m := range matches {
		packets, convErr := strconv.ParseUint(m[1], 10, 64)
		if convErr != nil {
			continue
		}

		if packets > 0 {
			return nil
		}
	}

	return fmt.Errorf("offloaded flows on %s have zero packet counters:\n%s", nodeName, flows)
}

func newOvsDebugPod(nodeName, image string) (*pod.Builder, error) {
	hostPathType := corev1.HostPathDirectory
	builder := pod.NewBuilder(
		APIClient,
		fmt.Sprintf("hwol-ovs-debug-%s", strings.ReplaceAll(nodeName, ".", "-")),
		tsparams.TestNamespaceName,
		image,
	).DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithHostNetwork().
		WithHostPid(true).
		// BusyBox images reject GNU "sleep infinity".
		RedefineDefaultCMD([]string{"sleep", "86400000"}).
		WithVolume(corev1.Volume{
			Name: "host",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
					Type: &hostPathType,
				},
			},
		})

	if builder == nil || builder.Definition == nil || len(builder.Definition.Spec.Containers) == 0 {
		return nil, fmt.Errorf("failed to init OVS debug pod builder")
	}

	builder.Definition.Spec.Containers[0].VolumeMounts = append(
		builder.Definition.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "host", MountPath: "/host", ReadOnly: true},
	)

	created, err := builder.CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVS debug pod on %s: %w", nodeName, err)
	}

	return created, nil
}

// CleanupHwolResources removes only HWOL-owned resources and verifies that the
// target PFs have left switchdev before returning. A failure here must fail the
// suite: otherwise a green run can leave the next suite with an unusable NIC.
func CleanupHwolResources(operatorNS, mcpName, pfName string, timeout, stableDuration time.Duration) error {
	klog.V(90).Infof("Cleaning HWOL test resources in namespace %s", operatorNS)

	exists, err := hwolResourcesExist(operatorNS)
	if err != nil {
		return err
	}

	if !exists {
		machineConfigExists, err := ovsOffloadMachineConfigExists(OvsOffloadMachineConfigName(mcpName))
		if err != nil {
			return err
		}

		if machineConfigExists {
			return fmt.Errorf(
				"operator-owned OVS hardware-offload MachineConfig %s remains without HWOL resources",
				OvsOffloadMachineConfigName(mcpName))
		}

		// An interrupted run may have removed its CRs before the operator finished
		// returning the PF to legacy mode. Treat the absence of fixed-name
		// resources as a read-only cleanup-verification path, not as proof that
		// teardown completed.
		if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
			return fmt.Errorf("HWOL cleanup verification timed out waiting for SR-IOV/MCP stable: %w", err)
		}

		return verifyHwolCleanup(operatorNS, mcpName, pfName)
	}

	ovsNet := NewOvsNetworkBuilder(
		APIClient, tsparams.OvsNetworkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
	if ovsNet != nil {
		if err := ovsNet.DeleteAndWait(timeout); err != nil {
			return fmt.Errorf("failed to delete OVSNetwork %s: %w", tsparams.OvsNetworkName, err)
		}
	}

	if err := deleteHwolSriovNetwork(operatorNS, timeout); err != nil {
		return err
	}

	pool, err := sriov.PullPoolConfig(APIClient, tsparams.PoolConfigName, operatorNS)
	if err != nil {
		if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
			return fmt.Errorf("failed to pull SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, err)
		}
	} else {
		if err := ensureHwolResourceOwnership("SriovNetworkPoolConfig", pool.Object.Name, pool.Object.Labels); err != nil {
			return err
		}

		if delErr := pool.Delete(); delErr != nil {
			return fmt.Errorf("failed to delete SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, delErr)
		}
	}

	// The SR-IOV Operator owns the rendered MachineConfig. Do not delete it
	// directly: wait for the PoolConfig finalizer to remove it, then let the
	// MCO disable OVS HW offload before removing the switchdev policy. This
	// ordering is essential: deleting the policy first makes mlx5 change eSwitch
	// mode while OVN's hardware TC rules hold eSwitch references (E-BUSY).
	if err := waitForOvsOffloadMachineConfigDeletion(mcpName, timeout); err != nil {
		return err
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		return fmt.Errorf("HWOL cleanup timed out disabling OVS hardware offload before switchdev teardown: %w", err)
	}

	if err := deleteHwolPolicy(operatorNS); err != nil {
		return err
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf(
				"%w: SNNS may be stuck resetting switchdev (device or resource busy); "+
					"reboot the MCP-labeled HWOL node before re-run: %w",
				ErrSwitchdevCleanupTimeout, err)
		}

		return fmt.Errorf(
			"HWOL cleanup failed waiting for SR-IOV/MCP stable after switchdev teardown: %w",
			err)
	}

	return verifyHwolCleanup(operatorNS, mcpName, pfName)
}

func waitForOvsOffloadMachineConfigDeletion(mcpName string, timeout time.Duration) error {
	machineConfigName := OvsOffloadMachineConfigName(mcpName)
	deadline := time.Now().Add(timeout)

	for {
		exists, err := ovsOffloadMachineConfigExists(machineConfigName)
		if err != nil {
			return err
		}

		if !exists {
			return nil
		}

		if !time.Now().Before(deadline) {
			return fmt.Errorf(
				"timed out waiting for SR-IOV Operator to remove MachineConfig %s after deleting PoolConfig %s",
				machineConfigName, tsparams.PoolConfigName)
		}

		time.Sleep(tsparams.RetryInterval)
	}
}

func ovsOffloadMachineConfigExists(machineConfigName string) (bool, error) {
	if err := APIClient.AttachScheme(mcfgv1.Install); err != nil {
		return false, fmt.Errorf("failed to register MachineConfig API: %w", err)
	}

	machineConfig := &mcfgv1.MachineConfig{}

	err := APIClient.Client.Get(context.Background(), client.ObjectKey{Name: machineConfigName}, machineConfig)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to get MachineConfig %s: %w", machineConfigName, err)
	}

	return true, nil
}

// hwolResourcesExist reports whether any fixed-name resources owned by the HWOL
// suite remain. This makes CleanupHwolResources safe to call as a final suite
// safeguard after the per-Describe cleanup has already completed.
func hwolResourcesExist(operatorNS string) (bool, error) {
	ovsNet := NewOvsNetworkBuilder(
		APIClient, tsparams.OvsNetworkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
	if ovsNet == nil {
		return false, fmt.Errorf("failed to initialize OVSNetwork cleanup builder")
	}

	if ovsNet.Exists() {
		if err := ensureHwolResourceOwnership("OVSNetwork", ovsNet.Object.Name, ovsNet.Object.Labels); err != nil {
			return false, err
		}

		return true, nil
	}

	if network, err := sriov.PullNetwork(APIClient, tsparams.SriovNetworkName, operatorNS); err == nil {
		if err := ensureHwolResourceOwnership("SriovNetwork", network.Object.Name, network.Object.Labels); err != nil {
			return false, err
		}

		return true, nil
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return false, fmt.Errorf("failed to look up HWOL SriovNetwork %s: %w", tsparams.SriovNetworkName, err)
	}

	if policy, err := sriov.PullPolicy(APIClient, tsparams.PolicyName, operatorNS); err == nil {
		if err := ensureHwolResourceOwnership(
			"SriovNetworkNodePolicy", policy.Object.Name, policy.Object.Labels,
		); err != nil {
			return false, err
		}

		return true, nil
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return false, fmt.Errorf("failed to look up HWOL SriovNetworkNodePolicy %s: %w", tsparams.PolicyName, err)
	}

	if pool, err := sriov.PullPoolConfig(APIClient, tsparams.PoolConfigName, operatorNS); err == nil {
		if err := ensureHwolResourceOwnership("SriovNetworkPoolConfig", pool.Object.Name, pool.Object.Labels); err != nil {
			return false, err
		}

		return true, nil
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return false, fmt.Errorf("failed to look up HWOL SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, err)
	}

	return false, nil
}

// RecoverHwolCleanup reboots only HWOL MCP nodes after failed cleanup, then verifies
// the switchdev and OVS state is gone. Callers may clear only
// ErrSwitchdevCleanupTimeout after this function succeeds.
func RecoverHwolCleanup(operatorNS, mcpName, pfName string, timeout, stableDuration time.Duration) error {
	// A reboot is a last resort for a PF stuck leaving switchdev, not a
	// substitute for deleting the suite's CRs or disabling hardware offload.
	// In particular, never reboot while an operator-owned offload MC remains.
	resourcesExist, err := hwolResourcesExist(operatorNS)
	if err != nil {
		return err
	}

	if resourcesExist {
		return fmt.Errorf("refusing HWOL reboot recovery while test-owned resources remain")
	}

	machineConfigName := OvsOffloadMachineConfigName(mcpName)

	machineConfigExists, err := ovsOffloadMachineConfigExists(machineConfigName)
	if err != nil {
		return err
	}

	if machineConfigExists {
		return fmt.Errorf("refusing HWOL reboot recovery while MachineConfig %s remains", machineConfigName)
	}

	workerNodes, err := ListMCPWorkerNodesWithPF(operatorNS, mcpName, pfName)
	if err != nil {
		return err
	}

	for _, workerNode := range workerNodes {
		if err := rebootHwolNode(workerNode.Object.Name, operatorNS, timeout); err != nil {
			return err
		}
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		return fmt.Errorf("HWOL cleanup recovery did not stabilize SR-IOV/MCP: %w", err)
	}

	if err := verifyHwolCleanup(operatorNS, mcpName, pfName); err != nil {
		return fmt.Errorf("HWOL cleanup recovery verification failed: %w", err)
	}

	return nil
}

// rebootHwolNode verifies a worker is Ready, reboots it through its SR-IOV
// config-daemon pod, and confirms its NotReady-to-Ready transition.
func rebootHwolNode(nodeName, operatorNS string, timeout time.Duration) error {
	workerNode, err := nodes.Pull(APIClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get HWOL node %s for recovery: %w", nodeName, err)
	}

	if err := workerNode.WaitUntilReady(timeout); err != nil {
		return fmt.Errorf("HWOL node %s was not Ready before reboot: %w", nodeName, err)
	}

	klog.V(90).Infof("Rebooting HWOL node %s after cleanup failure", nodeName)

	daemonPods, err := pod.List(APIClient, operatorNS, metav1.ListOptions{
		LabelSelector: "app=sriov-network-config-daemon",
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return fmt.Errorf("failed to list SR-IOV config daemon on node %s: %w", nodeName, err)
	}

	if len(daemonPods) == 0 {
		return fmt.Errorf("no SR-IOV config daemon found on HWOL node %s", nodeName)
	}

	_, rebootErr := daemonPods[0].ExecCommand([]string{"chroot", "/host", "reboot"})
	if rebootErr != nil && !isExpectedRebootDisconnect(rebootErr) {
		return fmt.Errorf("failed to reboot HWOL node %s: %w", nodeName, rebootErr)
	}

	if err := workerNode.WaitUntilNotReady(tsparams.DefaultTimeout); err != nil {
		return fmt.Errorf("HWOL node %s did not become NotReady after reboot: %w", nodeName, err)
	}

	workerNode, err = nodes.Pull(APIClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get HWOL node %s after reboot: %w", nodeName, err)
	}

	if err := workerNode.WaitUntilReady(timeout); err != nil {
		return fmt.Errorf("HWOL node %s did not become Ready after reboot: %w", nodeName, err)
	}

	return nil
}

// isExpectedRebootDisconnect identifies transport errors expected when reboot
// terminates the exec session that issued the host reboot command.
func isExpectedRebootDisconnect(err error) bool {
	message := strings.ToLower(err.Error())

	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "eof") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "unable to upgrade connection") ||
		strings.Contains(message, "command terminated") ||
		strings.Contains(message, "context canceled") ||
		strings.Contains(message, "context deadline exceeded")
}

func deleteHwolSriovNetwork(operatorNS string, timeout time.Duration) error {
	network, err := sriov.PullNetwork(APIClient, tsparams.SriovNetworkName, operatorNS)
	if err != nil {
		if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
			return nil
		}

		return fmt.Errorf("failed to pull HWOL SriovNetwork %s: %w", tsparams.SriovNetworkName, err)
	}

	if err := ensureHwolResourceOwnership(
		"SriovNetwork", network.Object.Name, network.Object.Labels,
	); err != nil {
		return err
	}

	if err := network.DeleteAndWait(timeout); err != nil {
		return fmt.Errorf("failed to delete HWOL SriovNetwork %s: %w", tsparams.SriovNetworkName, err)
	}

	return nil
}

// DeleteOwnedSriovNetwork deletes a suite-owned SriovNetwork. It rejects a
// matching fixed-name object that was created by an administrator or another
// test suite.
func DeleteOwnedSriovNetwork(name, operatorNS string) error {
	network, err := sriov.PullNetwork(APIClient, name, operatorNS)
	if err != nil {
		if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
			return nil
		}

		return fmt.Errorf("failed to pull SriovNetwork %s: %w", name, err)
	}

	if err := ensureHwolResourceOwnership(
		"SriovNetwork", network.Object.Name, network.Object.Labels,
	); err != nil {
		return err
	}

	if err := network.Delete(); err != nil {
		return fmt.Errorf("failed to delete SriovNetwork %s: %w", name, err)
	}

	return nil
}

func deleteHwolPolicy(operatorNS string) error {
	policy, err := sriov.PullPolicy(APIClient, tsparams.PolicyName, operatorNS)
	if err != nil {
		if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
			return nil
		}

		return fmt.Errorf("failed to pull HWOL SriovNetworkNodePolicy %s: %w", tsparams.PolicyName, err)
	}

	if err := ensureHwolResourceOwnership(
		"SriovNetworkNodePolicy", policy.Object.Name, policy.Object.Labels,
	); err != nil {
		return err
	}

	if err := policy.Delete(); err != nil {
		return fmt.Errorf("failed to delete HWOL SriovNetworkNodePolicy %s: %w", tsparams.PolicyName, err)
	}

	return nil
}

func verifyHwolCleanup(operatorNS, mcpName, pfName string) error {
	if err := verifyHwolOvsNetworkDeleted(operatorNS); err != nil {
		return err
	}

	if err := verifyHwolSriovNetworkDeleted(operatorNS); err != nil {
		return err
	}

	if _, err := sriov.PullPolicy(APIClient, tsparams.PolicyName, operatorNS); err == nil {
		return fmt.Errorf("HWOL SriovNetworkNodePolicy %s still exists after cleanup", tsparams.PolicyName)
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return fmt.Errorf("failed to verify deletion of HWOL SriovNetworkNodePolicy %s: %w", tsparams.PolicyName, err)
	}

	if _, err := sriov.PullPoolConfig(APIClient, tsparams.PoolConfigName, operatorNS); err == nil {
		return fmt.Errorf("HWOL SriovNetworkPoolConfig %s still exists after cleanup", tsparams.PoolConfigName)
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return fmt.Errorf("failed to verify deletion of HWOL SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, err)
	}

	if exists, err := ovsOffloadMachineConfigExists(OvsOffloadMachineConfigName(mcpName)); err != nil {
		return err
	} else if exists {
		return fmt.Errorf(
			"operator-owned OVS hardware-offload MachineConfig %s still exists after HWOL cleanup",
			OvsOffloadMachineConfigName(mcpName))
	}

	return verifyHwolNodeCleanup(operatorNS, mcpName, pfName)
}

func verifyHwolNodeCleanup(operatorNS, mcpName, pfName string) error {
	workerNodes, err := ListMCPWorkerNodes(mcpName)
	if err != nil {
		return fmt.Errorf("failed to list HWOL MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpName)
	}

	matchedPF := false

	for _, node := range workerNodes {
		nodeName := node.Object.Name

		state := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, operatorNS)
		if err := state.Discover(); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}

			return fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", nodeName, err)
		}

		iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
		if err != nil {
			continue
		}

		matchedPF = true

		if state.Objects.Status.SyncStatus != "Succeeded" {
			return fmt.Errorf(
				"node %s syncStatus is %q after HWOL cleanup, want Succeeded",
				nodeName, state.Objects.Status.SyncStatus)
		}
		// Empty is the operator's representation of the default legacy mode.
		if iface.EswitchMode != "" && iface.EswitchMode != "legacy" {
			return fmt.Errorf(
				"node %s interface %s eSwitchMode is %q after HWOL cleanup, want legacy",
				nodeName, pfName, iface.EswitchMode)
		}

		for _, bridge := range state.Objects.Status.Bridges.OVS {
			for _, uplink := range bridge.Uplinks {
				if uplink.Name == pfName || uplink.PciAddress == iface.PciAddress {
					return fmt.Errorf(
						"node %s still has managed OVS bridge %s for PF %s after HWOL cleanup",
						nodeName, bridge.Name, pfName)
				}
			}
		}
	}

	if !matchedPF {
		return fmt.Errorf("no MCP-labeled workers expose interface %s", pfName)
	}

	return nil
}

func verifyHwolOvsNetworkDeleted(operatorNS string) error {
	ovsNet := NewOvsNetworkBuilder(
		APIClient, tsparams.OvsNetworkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
	if ovsNet == nil {
		return fmt.Errorf("failed to initialize OVSNetwork cleanup builder")
	}

	if _, err := ovsNet.Get(); err == nil {
		return fmt.Errorf("HWOL OVSNetwork %s still exists after cleanup", tsparams.OvsNetworkName)
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to verify deletion of HWOL OVSNetwork %s: %w", tsparams.OvsNetworkName, err)
	}

	return nil
}

func verifyHwolSriovNetworkDeleted(operatorNS string) error {
	if _, err := sriov.PullNetwork(APIClient, tsparams.SriovNetworkName, operatorNS); err == nil {
		return fmt.Errorf("HWOL SriovNetwork %s still exists after cleanup", tsparams.SriovNetworkName)
	} else if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
		return fmt.Errorf(
			"failed to verify deletion of HWOL SriovNetwork %s: %w",
			tsparams.SriovNetworkName, err)
	}

	return nil
}
