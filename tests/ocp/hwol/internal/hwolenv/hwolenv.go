// Package hwolenv provides helpers for OCP HWOL switchdev / OVS bridge setup.
package hwolenv

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
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
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
)

// MCPNodeSelector returns the node-role label selector for the HWOL MCP pool.
func MCPNodeSelector(mcpLabel string) map[string]string {
	return map[string]string{fmt.Sprintf("node-role.kubernetes.io/%s", mcpLabel): ""}
}

// EnsureHwolOperatorConfig verifies SriovOperatorConfig is set for the
// manageSoftwareBridges + mlx5 HWOL path (enable OVS offload).
//
// Do not disable the mellanox plugin: on some beds CX6 firmware SR-IOV
// (PCI SR-IOV / TotalVfs) is only enabled by that plugin during Apply.
// Forcing disablePlugins=[mellanox] left Lab245 stuck at TotalVfs=0.
func EnsureHwolOperatorConfig(operatorNS, vendor string) error {
	opCfg, err := sriov.PullOperatorConfig(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	return validateHwolOperatorConfig(opCfg.Definition.Spec, vendor)
}

func validateHwolOperatorConfig(spec sriovv1.SriovOperatorConfigSpec, vendor string) error {
	for _, plugin := range spec.DisablePlugins {
		if strings.EqualFold(vendor, tsparams.MlxVendorID) && strings.EqualFold(string(plugin), "mellanox") {
			return fmt.Errorf("SriovOperatorConfig disables the Mellanox plugin; configure the prerequisite before running HWOL")
		}
	}

	if !spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] || !spec.EnableOvsOffload {
		return fmt.Errorf("SriovOperatorConfig must enable manageSoftwareBridges and OVS offload before running HWOL")
	}

	return nil
}

// EnsureSwitchdevFoundation configures operator, pool, and switchdev policy and waits for
// SR-IOV/MCP stability. It does not create SriovNetwork or OVSNetwork CRs.
func EnsureSwitchdevFoundation(
	operatorNS, mcpLabel string,
	device hwolconfig.DeviceConfig,
	vfNum int,
	docaDatapath bool,
	runID string,
) error {
	if runID == "" {
		return fmt.Errorf("HWOL run ID cannot be empty")
	}

	if err := ValidateSwitchdevPreconditions(operatorNS, mcpLabel, device, vfNum); err != nil {
		return err
	}

	if err := EnsureHwolOperatorConfig(operatorNS, device.Vendor); err != nil {
		return err
	}

	mcpNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(mcpNodes) == 0 {
		return fmt.Errorf("no nodes with label node-role.kubernetes.io/%s", mcpLabel)
	}

	if _, err := CreateOvsOffloadPoolConfig(tsparams.PoolConfigName, operatorNS, mcpLabel, runID); err != nil {
		return err
	}

	if _, err := CreateSwitchdevPolicy(
		tsparams.PolicyName,
		operatorNS,
		tsparams.ResourceName,
		device,
		vfNum,
		MCPNodeSelector(mcpLabel),
		docaDatapath,
		runID,
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

// CreateOvsOffloadPoolConfig creates SriovNetworkPoolConfig with only
// ovsHardwareOffloadConfig.name set to the MCP name.
// Do not set nodeSelector/maxUnavailable together with OvsHardwareOffload — webhook rejects it.
func CreateOvsOffloadPoolConfig(name, operatorNS, mcpName, runID string) (*sriov.PoolConfigBuilder, error) {
	if runID == "" {
		return nil, fmt.Errorf("HWOL run ID cannot be empty")
	}

	builder := sriov.NewPoolConfigBuilder(APIClient, name, operatorNS)
	if builder == nil {
		return nil, fmt.Errorf("failed to init SriovNetworkPoolConfig builder")
	}

	builder.Definition.Spec.OvsHardwareOffloadConfig = sriovv1.OvsHardwareOffloadConfig{
		Name: mcpName,
	}
	builder.Definition.Labels = map[string]string{tsparams.TestRunLabelKey: runID}

	if err := createOwnedTestResource(APIClient.Client, builder.Definition, runID); err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkPoolConfig %s: %w", name, err)
	}

	builder.Object = builder.Definition

	return builder, nil
}

// CreateSwitchdevPolicy creates a switchdev SriovNetworkNodePolicy with OVS bridge management.
// VF0 is reserved as the management port (pfNames range starts at 1).
// Existing policies are not adopted or replaced.
func CreateSwitchdevPolicy(
	name, operatorNS, resourceName string,
	device hwolconfig.DeviceConfig,
	vfNum int,
	nodeSelector map[string]string,
	docaDatapath bool,
	runID string,
) (*sriov.PolicyBuilder, error) {
	if runID == "" {
		return nil, fmt.Errorf("HWOL run ID cannot be empty")
	}

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
	policy.Definition.Spec.Bridge = switchdevBridgeConfig(docaDatapath)
	policy.Definition.Labels = map[string]string{tsparams.TestRunLabelKey: runID}

	if err := createOwnedTestResource(APIClient.Client, policy.Definition, runID); err != nil {
		return nil, fmt.Errorf("failed to create SriovNetworkNodePolicy %s: %w", name, err)
	}

	policy.Object = policy.Definition

	return policy, nil
}

func switchdevBridgeConfig(docaDatapath bool) sriovv1.Bridge {
	config := &sriovv1.OVSConfig{}
	if docaDatapath {
		config.Bridge.DatapathType = "doca"
		config.Uplink.Interface.Type = "doca"
	}

	return sriovv1.Bridge{OVS: config}
}

// CreateSriovNetwork creates a SriovNetwork for the HWOL resource in the test namespace.
// If ipam is empty, HostLocalIPAM is used. Callers may pass other IPAM JSON later
// (for example NV-IPAM) without changing this helper.
//
// Sriov CNI same-node HWOL is deferred: VF representors are not ports on the managed
// OVS bridge, so L2 between same-node pods fails. Keep this helper for attach coverage;
// the offload Entry that uses it is Pending until representor plumbing exists.
func CreateSriovNetwork(
	name, operatorNS, resourceName, networkNS, ipam, runID string,
) (*sriov.NetworkBuilder, error) {
	if runID == "" {
		return nil, fmt.Errorf("HWOL run ID cannot be empty")
	}

	network := sriov.NewNetworkBuilder(APIClient, name, operatorNS, networkNS, resourceName)
	if network == nil {
		return nil, fmt.Errorf("failed to init SriovNetwork builder")
	}

	if ipam == "" {
		ipam = HostLocalIPAM
	}

	network.Definition.Spec.IPAM = ipam

	network.Definition.Labels = map[string]string{tsparams.TestRunLabelKey: runID}
	if err := createOwnedTestResource(APIClient.Client, network.Definition, runID); err != nil {
		return nil, fmt.Errorf("failed to create SriovNetwork %s: %w", name, err)
	}

	network.Object = network.Definition

	return network, nil
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

// AssertSwitchdevAndOVSBridge checks SriovNetworkNodeState for switchdev mode and a managed OVS bridge
// on the given PF for each MCP-labeled node.
func AssertSwitchdevAndOVSBridge(operatorNS, mcpLabel, pfName string) error {
	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpLabel)
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
	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return "", fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return "", fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpLabel)
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

var offloadedEthernetRE = regexp.MustCompile(`(?i)eth\(src=([0-9a-f:]{17}),dst=([0-9a-f:]{17})\)`)

const assertOvsDocaScript = `
set -eu
doca_init="$(ovs-vsctl get Open_vSwitch . other_config:doca-init | tr -d '\"')"
hw_offload="$(ovs-vsctl get Open_vSwitch . other_config:hw-offload | tr -d '\"')"
if [ "$doca_init" != "true" ]; then
  echo "Open_vSwitch other_config:doca-init is $doca_init, want true" >&2
  exit 1
fi
if [ "$hw_offload" != "true" ]; then
  echo "Open_vSwitch other_config:hw-offload is $hw_offload, want true" >&2
  exit 1
fi
if ! ovs-appctl list-commands | grep -q 'dpif-doca/dpif-impl-get'; then
  echo "running ovs-vswitchd does not expose DOCA datapath commands" >&2
  exit 1
fi
echo "OVS-DOCA capability and configuration verified: doca-init=$doca_init hw-offload=$hw_offload"
`

// AssertOvsDocaEnabled verifies that OVS-DOCA was provisioned on every HWOL MCP node.
// OVS-DOCA provisioning is owned by the CI lane; this only validates its prerequisite.
func AssertOvsDocaEnabled(mcpLabel, image string) error {
	if image == "" {
		return fmt.Errorf("debug pod image cannot be empty")
	}

	workerNodes, err := ListMCPWorkerNodes(mcpLabel)
	if err != nil {
		return fmt.Errorf("failed to list MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpLabel)
	}

	for _, node := range workerNodes {
		nodeName := node.Object.Name

		debugPod, err := newOvsDebugPod(nodeName, image)
		if err != nil {
			return err
		}

		out, execErr := debugPod.ExecCommand([]string{
			"chroot", "/host", "sh", "-c", assertOvsDocaScript,
		})
		delErr := DeleteOwnedPod(debugPod, tsparams.DefaultTimeout)

		if execErr != nil {
			checkErr := fmt.Errorf("OVS-DOCA check failed on %s: %w (out=%s)", nodeName, execErr, out.String())
			if delErr != nil {
				return errors.Join(checkErr, fmt.Errorf("failed to delete OVS debug pod on %s: %w", nodeName, delErr))
			}

			return checkErr
		}

		if delErr != nil {
			return fmt.Errorf("failed to delete OVS-DOCA debug pod on %s: %w", nodeName, delErr)
		}

		klog.V(90).Infof("OVS-DOCA configuration OK on node %s: %s", nodeName, out.String())
	}

	return nil
}

// AssertDocaBridge verifies the operator created a working DOCA datapath and uplink.
func AssertDocaBridge(nodeName, bridge, pfName, image string) (resultErr error) {
	if nodeName == "" || bridge == "" || pfName == "" || image == "" {
		return fmt.Errorf("node, bridge, PF and debug image are required")
	}

	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return err
	}

	defer closeOvsDebugPod(debugPod, &resultErr)

	out, err := debugPod.ExecCommand([]string{"chroot", "/host", "sh", "-c", `
set -eu
bridge="$1"
pf="$2"
test "$(ovs-vsctl get Bridge "$bridge" datapath_type | tr -d '"')" = doca
test "$(ovs-vsctl get Interface "$pf" type | tr -d '"')" = doca
test "$(ovs-vsctl get Interface "$pf" ofport)" -gt 0
error="$(ovs-vsctl get Interface "$pf" error)"
test "$error" = '[]' || test -z "$error"
ovs-appctl dpif/show
`, "sh", bridge, pfName})
	if err != nil {
		return fmt.Errorf("DOCA bridge check failed on %s: %w (out=%s)", nodeName, err, out.String())
	}

	dps, err := debugPod.ExecCommand([]string{"chroot", "/host", "ovs-appctl", "dpctl/dump-dps"})
	if err != nil {
		return fmt.Errorf("failed to list datapaths on %s: %w (out=%s)", nodeName, err, dps.String())
	}

	if _, err := selectOvsDatapath(dps.String(), "doca"); err != nil {
		return fmt.Errorf("DOCA datapath is not active on %s: %w", nodeName, err)
	}

	klog.V(90).Infof("DOCA bridge %s on %s: %s", bridge, nodeName, out.String())

	return nil
}

// vfRepresentorOnBridgeScript maps each VF PCI address to its host representor
// netdev and asserts that representor is an ovs-vsctl port on $1.
const vfRepresentorOnBridgeScript = `
set -eu
bridge="$1"
interface_type="$2"
shift 2
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
  if [ -n "$interface_type" ]; then
    actual_type="$(ovs-vsctl get Interface "$rep" type | tr -d '"')"
    [ "$actual_type" = "$interface_type" ] || {
      echo "representor $rep type is $actual_type, want $interface_type" >&2
      exit 1
    }
    [ "$(ovs-vsctl get Interface "$rep" ofport)" -gt 0 ] || {
      echo "representor $rep has no valid OVS port" >&2
      exit 1
    }
    error="$(ovs-vsctl get Interface "$rep" error)"
    [ "$error" = '[]' ] || [ -z "$error" ] || { echo "representor $rep error: $error" >&2; exit 1; }
  fi
  echo "OK $full -> $rep on $bridge"
done
`

// AssertVFRepresentorsOnBridge maps each VF PCI address to its host representor netdev
// (PF virtfnN → typically <pf>_<N>) and asserts that representor is an ovs-vsctl port
// on bridge. Used for the ovs CNI offload path before iperf.
func AssertVFRepresentorsOnBridge(nodeName, bridge, image string, pciAddrs ...string) error {
	return assertVFRepresentorsOnBridge(nodeName, bridge, image, "", nil, pciAddrs...)
}

// AssertDocaVFRepresentorsOnBridge also verifies fresh DOCA flow API assignments.
func AssertDocaVFRepresentorsOnBridge(
	nodeName, bridge, image string, cursor DocaProviderCursor, pciAddrs ...string,
) error {
	return assertVFRepresentorsOnBridge(nodeName, bridge, image, "doca", &cursor, pciAddrs...)
}

func assertVFRepresentorsOnBridge(nodeName, bridge, image, interfaceType string,
	cursor *DocaProviderCursor, pciAddrs ...string) (resultErr error) {
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

	defer closeOvsDebugPod(debugPod, &resultErr)

	args := []string{"chroot", "/host", "sh", "-c", vfRepresentorOnBridgeScript, "sh", bridge, interfaceType}
	args = append(args, pciAddrs...)

	out, err := debugPod.ExecCommandWithTimeout(args, 30*time.Second)
	if err != nil {
		return fmt.Errorf("VF representor-on-bridge check failed on %s bridge %s: %w (out=%s)",
			nodeName, bridge, err, out.String())
	}

	klog.V(90).Infof("VF representors on bridge %s (%s):\n%s", bridge, nodeName, out.String())

	if cursor != nil {
		representors, err := parseRepresentorEvidence(out.String(), bridge, pciAddrs)
		if err != nil {
			return fmt.Errorf("cannot identify DOCA workload representors on %s: %w", nodeName, err)
		}

		log, err := readDocaProviderLog(debugPod, *cursor)
		if err != nil {
			return fmt.Errorf("cannot inspect DOCA provider assignments on %s: %w", nodeName, err)
		}

		if err := verifyDocaProviderAssignments(log, representors); err != nil {
			return fmt.Errorf("DOCA provider assignment on %s: %w", nodeName, err)
		}

		klog.V(90).Infof(
			"Verified fresh doca_flow_api assignments for workload representors %v on %s", representors, nodeName)
	}

	return nil
}

// OvsOffloadedTrafficPackets counts offloaded IPv4 packets between two workload MACs.
// The caller should compare snapshots before and after sending test traffic.
func OvsOffloadedTrafficPackets(
	nodeName, image, datapathType, firstMAC, secondMAC string,
) (count uint64, resultErr error) {
	if nodeName == "" || image == "" || datapathType == "" {
		return 0, fmt.Errorf("node name, debug pod image and datapath type cannot be empty")
	}

	debugPod, err := newOvsDebugPod(nodeName, image)
	if err != nil {
		return 0, err
	}

	defer closeOvsDebugPod(debugPod, &resultErr)

	dps, err := debugPod.ExecCommand([]string{"chroot", "/host", "ovs-appctl", "dpctl/dump-dps"})
	if err != nil {
		return 0, fmt.Errorf("failed to list datapaths on %s: %w (out=%s)", nodeName, err, dps.String())
	}

	datapath, err := selectOvsDatapath(dps.String(), datapathType)
	if err != nil {
		return 0, err
	}

	out, err := debugPod.ExecCommand([]string{
		"chroot", "/host", "ovs-appctl", "dpctl/dump-flows", "--names", datapath, "type=offloaded",
	})
	if err != nil {
		return 0, fmt.Errorf("failed to dump offloaded flows on %s: %w (out=%s)", nodeName, err, out.String())
	}

	return countOffloadedTrafficPackets(out.String(), firstMAC, secondMAC)
}

func selectOvsDatapath(output, datapathType string) (string, error) {
	var selected string

	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if !strings.HasPrefix(name, datapathType+"@") {
			continue
		}

		if selected != "" {
			return "", fmt.Errorf("multiple %s datapaths found; cannot identify workload datapath", datapathType)
		}

		selected = name
	}

	if selected == "" {
		return "", fmt.Errorf("no %s datapath found in %q", datapathType, output)
	}

	return selected, nil
}

func countOffloadedTrafficPackets(flows, firstMAC, secondMAC string) (uint64, error) {
	first, err := net.ParseMAC(firstMAC)
	if err != nil || len(first) != 6 {
		return 0, fmt.Errorf("invalid first workload MAC %q", firstMAC)
	}

	second, err := net.ParseMAC(secondMAC)
	if err != nil || len(second) != 6 || first.String() == second.String() {
		return 0, fmt.Errorf("invalid or duplicate second workload MAC %q", secondMAC)
	}

	var total uint64

	for _, line := range strings.Split(flows, "\n") {
		if !strings.Contains(strings.ToLower(line), "eth_type(0x0800)") {
			continue
		}

		macs := offloadedEthernetRE.FindStringSubmatch(line)
		if len(macs) != 3 {
			continue
		}

		src, dst := strings.ToLower(macs[1]), strings.ToLower(macs[2])

		isWorkloadPair := (src == first.String() && dst == second.String()) ||
			(src == second.String() && dst == first.String())

		if !isWorkloadPair {
			continue
		}

		counter := offloadedPacketsRE.FindStringSubmatch(line)
		if len(counter) != 2 {
			return 0, fmt.Errorf("matching offloaded flow has no packet counter: %s", line)
		}

		packets, err := strconv.ParseUint(counter[1], 10, 64)
		if err != nil || ^uint64(0)-total < packets {
			return 0, fmt.Errorf("invalid packet counter in offloaded flow: %s", line)
		}

		total += packets

		klog.V(90).Infof("Matching workload offloaded flow: %s", line)
	}

	return total, nil
}

func closeOvsDebugPod(debugPod *pod.Builder, resultErr *error) {
	if err := DeleteOwnedPod(debugPod, tsparams.DefaultTimeout); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("failed to clean up OVS debug pod: %w", err))
	}
}

func newOvsDebugPod(nodeName, image string) (*pod.Builder, error) {
	hostPathType := corev1.HostPathDirectory
	runID := string(uuid.NewUUID())
	builder := pod.NewBuilder(
		APIClient,
		"hwol-ovs-debug-"+runID,
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

	created, err := CreateOwnedPod(builder, runID, tsparams.DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVS debug pod on %s: %w", nodeName, err)
	}

	return created, nil
}

// CleanupHwolResources removes only HWOL-owned resources and verifies that the
// target PFs have left switchdev before returning. A failure here must fail the
// suite: otherwise a green run can leave the next suite with an unusable NIC.
func CleanupHwolResources(operatorNS, mcpName, pfName, runID string, timeout, stableDuration time.Duration) error {
	klog.V(90).Infof("Cleaning HWOL test resources in namespace %s", operatorNS)

	if err := DeleteOwnedTestResource(operatorNS, tsparams.OvsNetworkName, runID, &sriovv1.OVSNetwork{}); err != nil {
		return err
	}

	if err := DeleteOwnedTestResource(operatorNS, tsparams.SriovNetworkName, runID, &sriovv1.SriovNetwork{}); err != nil {
		return err
	}

	if err := DeleteOwnedTestResource(
		operatorNS, tsparams.PolicyName, runID, &sriovv1.SriovNetworkNodePolicy{}); err != nil {
		return err
	}

	if err := DeleteOwnedTestResource(
		operatorNS, tsparams.PoolConfigName, runID, &sriovv1.SriovNetworkPoolConfig{}); err != nil {
		return err
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		return fmt.Errorf("HWOL cleanup did not converge; inspect operator and MCP diagnostics: %w", err)
	}

	return verifyHwolCleanup(operatorNS, mcpName, pfName)
}

// RecoverHwolCleanup reboots only HWOL MCP nodes after failed cleanup, then verifies
// the switchdev and OVS state is gone. Callers must preserve the original failure.
func RecoverHwolCleanup(operatorNS, mcpName, pfName string, timeout, stableDuration time.Duration) error {
	workerNodes, err := ListMCPWorkerNodes(mcpName)
	if err != nil {
		return fmt.Errorf("failed to list HWOL MCP nodes for recovery: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpName)
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

func verifyHwolCleanup(operatorNS, mcpName, pfName string) error {
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

	workerNodes, err := ListMCPWorkerNodes(mcpName)
	if err != nil {
		return fmt.Errorf("failed to list HWOL MCP nodes: %w", err)
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("no nodes found with label node-role.kubernetes.io/%s", mcpName)
	}

	for _, node := range workerNodes {
		nodeName := node.Object.Name

		state := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, operatorNS)
		if err := state.Discover(); err != nil {
			return fmt.Errorf("failed to discover SriovNetworkNodeState for %s: %w", nodeName, err)
		}

		if state.Objects.Status.SyncStatus != "Succeeded" {
			return fmt.Errorf(
				"node %s syncStatus is %q after HWOL cleanup, want Succeeded",
				nodeName, state.Objects.Status.SyncStatus)
		}

		iface, err := findStatusInterface(state.Objects.Status.Interfaces, pfName)
		if err != nil {
			return fmt.Errorf("node %s: %w", nodeName, err)
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
