// Package hwolenv provides helpers for OCP HWOL switchdev / OVS bridge setup.
package hwolenv

import (
	"fmt"
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
func EnsureHwolOperatorConfig(operatorNS string) error {
	opCfg, err := sriov.PullOperatorConfig(APIClient, operatorNS)
	if err != nil {
		return fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	changed := false

	if len(opCfg.Definition.Spec.DisablePlugins) > 0 {
		opCfg = opCfg.RemoveDisablePlugins()
		changed = true
	}

	if opCfg.Definition.Spec.FeatureGates == nil {
		opCfg.Definition.Spec.FeatureGates = map[string]bool{}
	}

	if !opCfg.Definition.Spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] {
		opCfg.Definition.Spec.FeatureGates[tsparams.ManageSoftwareBridgesGate] = true
		changed = true
	}

	if !opCfg.Definition.Spec.EnableOvsOffload {
		opCfg.Definition.Spec.EnableOvsOffload = true
		changed = true
	}

	if !changed {
		return nil
	}

	_, err = opCfg.Update()
	if err != nil {
		return fmt.Errorf("failed to update SriovOperatorConfig for HWOL: %w", err)
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
	if err := EnsureHwolOperatorConfig(operatorNS); err != nil {
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

	if builder.Exists() {
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

	if policy.Exists() {
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

	ovsNet := NewOvsNetworkBuilder(
		APIClient, tsparams.OvsNetworkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
	if ovsNet != nil {
		if err := ovsNet.Delete(); err != nil {
			return fmt.Errorf("failed to delete OVSNetwork %s: %w", tsparams.OvsNetworkName, err)
		}
	}

	if err := deleteHwolSriovNetwork(operatorNS); err != nil {
		return err
	}

	if err := deleteHwolPolicy(operatorNS); err != nil {
		return err
	}

	pool, err := sriov.PullPoolConfig(APIClient, tsparams.PoolConfigName, operatorNS)
	if err != nil {
		if !k8serrors.IsNotFound(err) && !strings.Contains(err.Error(), "does not exist") {
			return fmt.Errorf("failed to pull SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, err)
		}
	} else if delErr := pool.Delete(); delErr != nil {
		return fmt.Errorf("failed to delete SriovNetworkPoolConfig %s: %w", tsparams.PoolConfigName, delErr)
	}

	if err := WaitForSriovAndMCPStable(operatorNS, mcpName, timeout, stableDuration); err != nil {
		return fmt.Errorf(
			"HWOL cleanup timed out waiting for SR-IOV/MCP stable: SNNS may be stuck resetting "+
				"switchdev (device or resource busy); reboot the MCP-labeled HWOL node before re-run: %w",
			err)
	}

	return verifyHwolCleanup(operatorNS, mcpName, pfName)
}

func deleteHwolSriovNetwork(operatorNS string) error {
	network, err := sriov.PullNetwork(APIClient, tsparams.SriovNetworkName, operatorNS)
	if err != nil {
		if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
			return nil
		}

		return fmt.Errorf("failed to pull HWOL SriovNetwork %s: %w", tsparams.SriovNetworkName, err)
	}

	if err := network.Delete(); err != nil {
		return fmt.Errorf("failed to delete HWOL SriovNetwork %s: %w", tsparams.SriovNetworkName, err)
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

	if err := policy.Delete(); err != nil {
		return fmt.Errorf("failed to delete HWOL SriovNetworkNodePolicy %s: %w", tsparams.PolicyName, err)
	}

	return nil
}

func verifyHwolCleanup(operatorNS, mcpName, pfName string) error {
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
