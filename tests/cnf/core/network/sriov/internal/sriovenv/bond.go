package sriovenv

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"k8s.io/klog/v2"
)

// Bond CNI mode strings for CreateBondNAD. Must match eco-goinfra/pkg/nad validModes
// (balance-rr, active-backup, balance-xor, balance-tlb, balance-alb). NMState-only modes
// such as 802.3ad are defined in the LACP test suite.
const (
	BondModeActiveBackup = "active-backup"
	BondModeBalanceRR    = "balance-rr"
	BondModeBalanceXOR   = "balance-xor"

	// Bond xmitHashPolicy values for balance-xor (Bond CNI JSON field xmitHashPolicy).
	BondXmitHashPolicyLayer2  = "layer2"
	BondXmitHashPolicyLayer23 = "layer2+3"
	BondXmitHashPolicyLayer34 = "layer3+4"

	// BondInterfaceName is the default bond master interface created by the Bond CNI plugin.
	BondInterfaceName = "bond0"
	// BondSlave1IfName and BondSlave2IfName are the default first two slave links in a 2-slave bond NAD.
	BondSlave1IfName = "net1"
	BondSlave2IfName = "net2"

	// BondActiveSlavePollInterval is the poll interval when waiting for bond slave state changes.
	BondActiveSlavePollInterval = 100 * time.Millisecond
	// BondActiveSlaveChangeTimeout is the maximum wait for bond active_slave or MII state transitions.
	BondActiveSlaveChangeTimeout = 30 * time.Second

	// BondMTU1280 and BondMTU9000 are the MTUs used by shared bond-mode SR-IOV policies.
	BondMTU1280 = 1280
	BondMTU9000 = 9000
	// BondMTUDefault is the standard L2 MTU used by Bond CNI xmitHashPolicy cases.
	BondMTUDefault = 1500

	// Shared bond policy resource names (must stay stable across bond-mode and bond-cni).
	BondResourcePF1Small   = "sriovbondpf1mtu1280"
	BondResourcePF1Jumbo   = "sriovbondpf1mtu9000"
	BondResourcePF2Small   = "sriovbondpf2mtu1280"
	BondResourcePF2Jumbo   = "sriovbondpf2mtu9000"
	BondResourcePF1Default = "sriovbondpf1mtu1500"
	BondResourcePF2Default = "sriovbondpf2mtu1500"

	// BondMinVFsPerPF is the minimum total VFs per PF for bond tests on five-VF devices.
	BondMinVFsPerPF = 5
	// BondMinVFsStandardLayout is the minimum total VFs per PF for the standard small/jumbo split.
	BondMinVFsStandardLayout = 10
)

// CreateBondNAD builds a Bond CNI NAD builder in the SR-IOV test namespace.
// The caller is responsible for calling .Create() on the returned builder.
func CreateBondNAD(
	nadName,
	mode,
	ipamType string,
	mtu,
	slaveCount int,
) (*nad.Builder, error) {
	return createBondNAD(nadName, mode, mtu, slaveCount, &nad.IPAM{Type: ipamType}, "")
}

// CreateBondNADWithXmitHashPolicy builds a Bond CNI NAD like CreateBondNAD with
// MasterBondPlugin.WithXmitHashPolicy set (layer2, layer2+3, or layer3+4).
func CreateBondNADWithXmitHashPolicy(
	nadName,
	mode,
	ipamType string,
	mtu,
	slaveCount int,
	xmitHashPolicy string,
) (*nad.Builder, error) {
	return createBondNAD(nadName, mode, mtu, slaveCount, &nad.IPAM{Type: ipamType}, xmitHashPolicy)
}

// CreateBondNADWithWhereabouts builds a Bond CNI NAD with Whereabouts IPAM on the bond interface.
// The caller is responsible for calling .Create() on the returned builder.
func CreateBondNADWithWhereabouts(
	nadName,
	mode string,
	mtu,
	slaveCount int,
	ipRange,
	gateway string,
) (*nad.Builder, error) {
	ipam, err := bondWhereaboutsIPAM(ipRange, gateway)
	if err != nil {
		return nil, err
	}

	return createBondNAD(nadName, mode, mtu, slaveCount, ipam, "")
}

// bondWhereaboutsIPAM builds Whereabouts IPAM with an explicit alloc pool so the gateway is not
// assigned to pods (same range_start/range_end pattern as SR-IOV Whereabouts NADs).
func bondWhereaboutsIPAM(ipRange, gateway string) (*nad.IPAM, error) {
	if ipRange == "" || gateway == "" {
		return nil, fmt.Errorf("invalid whereabouts IPAM range %q gateway %q", ipRange, gateway)
	}

	rangeStart := tsparams.WhereaboutsIPv6AllocStart
	rangeEnd := tsparams.WhereaboutsIPv6AllocEnd

	if ipRange == tsparams.WhereaboutsIPv6Range2 {
		rangeStart = tsparams.WhereaboutsIPv6AllocStart2
		rangeEnd = tsparams.WhereaboutsIPv6AllocEnd2
	}

	return &nad.IPAM{
		Type:       "whereabouts",
		AddrRange:  ipRange,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
		Gateway:    gateway,
	}, nil
}

func createBondNAD(
	nadName,
	mode string,
	mtu,
	slaveCount int,
	ipam *nad.IPAM,
	xmitHashPolicy string,
) (*nad.Builder, error) {
	if slaveCount < 2 {
		return nil, fmt.Errorf("slaveCount must be >= 2, got %d", slaveCount)
	}

	var links []nad.Link

	for idx := 1; idx <= slaveCount; idx++ {
		links = append(links, nad.Link{Name: fmt.Sprintf("net%d", idx)})
	}

	plugin := nad.NewMasterBondPlugin(nadName, mode).
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks(links).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(ipam)

	if xmitHashPolicy != "" {
		plugin = plugin.WithXmitHashPolicy(xmitHashPolicy)
	}

	masterPlugin, err := plugin.GetMasterPluginConfig()
	if err != nil {
		return nil, err
	}

	if mtu > 0 {
		masterPlugin.Mtu = mtu
	}

	return nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPlugin), nil
}

// GetBondActiveSlave reads the current active_slave from sysfs for the given bond interface.
func GetBondActiveSlave(clientPod *pod.Builder, bondName string) (string, error) {
	out, err := clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/active_slave", bondName)})
	if err != nil {
		return "", fmt.Errorf("failed to read bond active_slave: %w (out=%s)", err, out.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// WaitForBondActiveSlaveChange polls active_slave until it differs from previousSlave or times out.
func WaitForBondActiveSlaveChange(clientPod *pod.Builder, bondName, previousSlave string) (string, error) {
	var last string

	for deadline := time.Now().Add(BondActiveSlaveChangeTimeout); time.Now().Before(deadline); time.Sleep(
		BondActiveSlavePollInterval) {
		slave, err := GetBondActiveSlave(clientPod, bondName)
		if err != nil {
			return "", err
		}

		last = slave

		if slave != "" && slave != previousSlave {
			return slave, nil
		}
	}

	return last, fmt.Errorf(
		"bond did not switch active slave from %q within %v (last active_slave=%q)",
		previousSlave, BondActiveSlaveChangeTimeout, last)
}

// WaitForBondSlaveMIIDown polls until the given slave is MII-down, the bond is still up,
// and at least one other slave remains MII-up (degraded but operational).
func WaitForBondSlaveMIIDown(clientPod *pod.Builder, bondName, downSlave string) error {
	var (
		lastBondUp bool
		lastSlaves map[string]string
	)

	for deadline := time.Now().Add(BondActiveSlaveChangeTimeout); time.Now().Before(deadline); time.Sleep(
		BondActiveSlavePollInterval) {
		bondUp, err := isBondInterfaceUp(clientPod, bondName)
		if err != nil {
			return err
		}

		slaves, err := getBondSlaveMIIStatuses(clientPod, bondName)
		if err != nil {
			return err
		}

		lastBondUp = bondUp
		lastSlaves = slaves

		if bondUp && slaves[downSlave] == "down" && countBondSlavesWithMII(slaves, "up") >= 1 {
			return nil
		}
	}

	return fmt.Errorf(
		"bond %q did not stabilize with slave %q MII down within %v (bondUp=%t, slaves=%v)",
		bondName, downSlave, BondActiveSlaveChangeTimeout, lastBondUp, lastSlaves)
}

// WaitForBondDegradedOneSlaveDown polls until the bond is up with at least one slave MII-down
// and one MII-up.
func WaitForBondDegradedOneSlaveDown(clientPod *pod.Builder, bondName string) error {
	var (
		lastStatus bondDegradedStatus
		lastErr    error
	)

	for deadline := time.Now().Add(BondActiveSlaveChangeTimeout); time.Now().Before(deadline); time.Sleep(
		BondActiveSlavePollInterval) {
		status, err := checkBondDegradedOneSlaveDown(clientPod, bondName)
		if err != nil {
			lastErr = err

			continue
		}

		lastErr = nil
		lastStatus = status

		if !status.degraded {
			continue
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf(
			"bond %q did not degrade to at least one slave MII down within %v "+
				"(bondUp=%t, slaves=%v, lastErr=%w)",
			bondName, BondActiveSlaveChangeTimeout, lastStatus.bondUp, lastStatus.slaves, lastErr)
	}

	return fmt.Errorf(
		"bond %q did not degrade to at least one slave MII down within %v (bondUp=%t, slaves=%v)",
		bondName, BondActiveSlaveChangeTimeout, lastStatus.bondUp, lastStatus.slaves)
}

type bondDegradedStatus struct {
	degraded bool
	bondUp   bool
	slaves   map[string]string
}

func checkBondDegradedOneSlaveDown(
	clientPod *pod.Builder, bondName string,
) (bondDegradedStatus, error) {
	bondUp, err := isBondInterfaceUp(clientPod, bondName)
	if err != nil {
		return bondDegradedStatus{}, err
	}

	slaves, err := getBondSlaveMIIStatuses(clientPod, bondName)
	if err != nil {
		return bondDegradedStatus{bondUp: bondUp}, err
	}

	return bondDegradedStatus{
		degraded: bondUp &&
			countBondSlavesWithMII(slaves, "down") >= 1 &&
			countBondSlavesWithMII(slaves, "up") >= 1,
		bondUp: bondUp,
		slaves: slaves,
	}, nil
}

// WaitForBondSlavesMIIUp polls until all bond slaves report MII up and the bond is up.
func WaitForBondSlavesMIIUp(clientPod *pod.Builder, bondName string) error {
	var (
		lastBondUp bool
		lastSlaves map[string]string
	)

	for deadline := time.Now().Add(BondActiveSlaveChangeTimeout); time.Now().Before(deadline); time.Sleep(
		BondActiveSlavePollInterval) {
		bondUp, err := isBondInterfaceUp(clientPod, bondName)
		if err != nil {
			return err
		}

		slaves, err := getBondSlaveMIIStatuses(clientPod, bondName)
		if err != nil {
			return err
		}

		lastBondUp = bondUp
		lastSlaves = slaves

		if bondUp &&
			len(slaves) >= 2 &&
			countBondSlavesWithMII(slaves, "up") >= 2 &&
			countBondSlavesWithMII(slaves, "down") == 0 {
			return nil
		}
	}

	return fmt.Errorf(
		"bond %q did not recover all slaves to MII up within %v (bondUp=%t, slaves=%v)",
		bondName, BondActiveSlaveChangeTimeout, lastBondUp, lastSlaves)
}

// SetLinkStatus sets a network interface operstate to up or down inside a pod.
func SetLinkStatus(podBuilder *pod.Builder, nic, status string) error {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c", fmt.Sprintf("ip link set dev %s %s", nic, status)})
	if err != nil {
		return fmt.Errorf("failed to set interface %s %s: %w (out=%s)", nic, status, err, out.String())
	}

	return nil
}

// VerifyBondXmitHashPolicy checks sysfs xmit_hash_policy for the expected token (field match).
func VerifyBondXmitHashPolicy(podBuilder *pod.Builder, bondName, expectedPolicy string) error {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/xmit_hash_policy", bondName)})
	if err != nil {
		return fmt.Errorf("failed to read bond xmit_hash_policy: %w (out=%s)", err, out.String())
	}

	fields := strings.Fields(strings.TrimSpace(out.String()))
	for _, field := range fields {
		if field == expectedPolicy {
			return nil
		}
	}

	return fmt.Errorf("bond xmit_hash_policy mismatch: expected %q, got %q",
		expectedPolicy, strings.TrimSpace(out.String()))
}

// VerifyBondInterfaceState checks that the bond interface is up, has the expected mode, and slave count.
func VerifyBondInterfaceState(podBuilder *pod.Builder, bondName, expectedMode string, expectedSlaveCount int) error {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/operstate", bondName)})
	if err != nil {
		return fmt.Errorf("failed to read bond operstate: %w (out=%s)", err, out.String())
	}

	if strings.TrimSpace(out.String()) != "up" {
		return fmt.Errorf("bond interface %s is not up (operstate=%q)", bondName, strings.TrimSpace(out.String()))
	}

	out, err = podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/mode", bondName)})
	if err != nil {
		return fmt.Errorf("failed to read bond mode: %w (out=%s)", err, out.String())
	}

	if !strings.Contains(out.String(), expectedMode) {
		return fmt.Errorf("bond mode mismatch: expected %q, got %q", expectedMode, strings.TrimSpace(out.String()))
	}

	out, err = podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/slaves | wc -w", bondName)})
	if err != nil {
		return fmt.Errorf("failed to read bond slaves: %w (out=%s)", err, out.String())
	}

	got := strings.TrimSpace(out.String())
	if got != fmt.Sprintf("%d", expectedSlaveCount) {
		return fmt.Errorf("bond slave count mismatch: expected %d, got %s", expectedSlaveCount, got)
	}

	return nil
}

func isBondInterfaceUp(clientPod *pod.Builder, bondName string) (bool, error) {
	out, err := clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/operstate", bondName)})
	if err != nil {
		return false, fmt.Errorf("failed to read bond operstate: %w (out=%s)", err, out.String())
	}

	return strings.TrimSpace(out.String()) == "up", nil
}

func getBondSlaveMIIStatuses(clientPod *pod.Builder, bondName string) (map[string]string, error) {
	out, err := clientPod.ExecCommand([]string{"cat", fmt.Sprintf("/proc/net/bonding/%s", bondName)})
	if err != nil {
		return nil, fmt.Errorf("failed to read bond status: %w (out=%s)", err, out.String())
	}

	return parseBondSlaveMIIStatuses(out.String()), nil
}

func parseBondSlaveMIIStatuses(bondingOutput string) map[string]string {
	slaves := make(map[string]string)

	var currentSlave string

	for _, line := range strings.Split(bondingOutput, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Slave Interface:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				currentSlave = strings.TrimSpace(parts[1])
			}

			continue
		}

		if strings.HasPrefix(line, "MII Status:") && currentSlave != "" {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				slaves[currentSlave] = strings.TrimSpace(parts[1])
			}
		}
	}

	return slaves
}

func countBondSlavesWithMII(slaves map[string]string, status string) int {
	count := 0

	for _, slaveStatus := range slaves {
		if slaveStatus == status {
			count++
		}
	}

	return count
}

// BondFunctionalPolicyNames is the policy name list used by SetupBondStack.
var BondFunctionalPolicyNames = []string{
	"bond-policy-pf1-mtu1280",
	"bond-policy-pf1-mtu9000",
	"bond-policy-pf2-mtu1280",
	"bond-policy-pf2-mtu9000",
}

// BondStalePolicyNames is the cleanup list of current and prior bond policy names.
var BondStalePolicyNames = []string{
	"ipv4-policy-pf1-mtu500",
	"ipv4-policy-pf1-mtu9000",
	"ipv4-policy-pf2-mtu500",
	"ipv4-policy-pf2-mtu9000",
	"ipv6-policy-pf1-mtu1280",
	"ipv6-policy-pf1-mtu9000",
	"ipv6-policy-pf2-mtu1280",
	"ipv6-policy-pf2-mtu9000",
	"bond-policy-pf1-mtu1280",
	"bond-policy-pf1-mtu9000",
	"bond-policy-pf2-mtu1280",
	"bond-policy-pf2-mtu9000",
	"bond-policy-pf1-mtu1500",
	"bond-policy-pf2-mtu1500",
	"bond-scale-policy-pf1",
	"bond-scale-policy-pf2",
	"bond-scale-policy-pf1-ipv6",
	"bond-scale-policy-pf2-ipv6",
}

// BondNetworkConfig is an SR-IOV network name and resource for a bond slave.
type BondNetworkConfig struct {
	Name     string
	Resource string
}

// BondStackParams is the input for SetupBondStack.
type BondStackParams struct {
	PF1,
	PF2 string
	PF1NumVFs,
	PF2NumVFs int
	VFSmallStart,
	VFSmallEnd,
	VFLargeStart,
	VFLargeEnd int
	Networks       []BondNetworkConfig
	DefaultMTUOnly bool // when true, create only MTU 1500 policies (Bond CNI xmitHashPolicy suite)
}

// SelectBondVFLayout returns VF ranges for shared 1280/9000 bond policies on each PF.
func SelectBondVFLayout(minTotal int) (vfSmallEnd, vfLargeStart, vfLargeEnd int, ok bool) {
	if minTotal < BondMinVFsPerPF {
		return 0, 0, 0, false
	}

	switch {
	case minTotal >= BondMinVFsStandardLayout:
		// Standard layout: small MTU VFs 0-4, jumbo MTU VFs 5-9.
		return 4, 5, 9, true
	default:
		// Five-VF layout: small MTU VFs 0-1, jumbo MTU VFs 2-4.
		return 1, 2, 4, true
	}
}

// SetupBondStack creates shared bond SR-IOV policies and slave networks, then waits for
// SR-IOV/MCP stability after policy creation.
func SetupBondStack(params BondStackParams) error {
	type policySpec struct {
		pfSuffix string
		resource string
		pf       string
		numVFs   int
		mtu      int
		vfStart  int
		vfEnd    int
	}

	policies := []policySpec{
		{"pf1", BondResourcePF1Small, params.PF1, params.PF1NumVFs, BondMTU1280, params.VFSmallStart, params.VFSmallEnd},
		{"pf1", BondResourcePF1Jumbo, params.PF1, params.PF1NumVFs, BondMTU9000, params.VFLargeStart, params.VFLargeEnd},
		{"pf2", BondResourcePF2Small, params.PF2, params.PF2NumVFs, BondMTU1280, params.VFSmallStart, params.VFSmallEnd},
		{"pf2", BondResourcePF2Jumbo, params.PF2, params.PF2NumVFs, BondMTU9000, params.VFLargeStart, params.VFLargeEnd},
	}

	if params.DefaultMTUOnly {
		klog.Infof("Creating default-MTU (%d) SR-IOV bond policies only", BondMTUDefault)

		policies = []policySpec{
			{
				"pf1", BondResourcePF1Default, params.PF1, params.PF1NumVFs,
				BondMTUDefault, params.VFLargeStart, params.VFLargeEnd,
			},
			{
				"pf2", BondResourcePF2Default, params.PF2, params.PF2NumVFs,
				BondMTUDefault, params.VFLargeStart, params.VFLargeEnd,
			},
		}
	}

	for _, policy := range policies {
		policyName := fmt.Sprintf("bond-policy-%s-mtu%d", policy.pfSuffix, policy.mtu)

		if err := CreateSriovPolicy(
			policyName, policy.resource, policy.pf, policy.mtu, policy.vfStart, policy.vfEnd, policy.numVFs,
		); err != nil {
			return fmt.Errorf("failed to create bond %s MTU%d policy: %w", policy.pfSuffix, policy.mtu, err)
		}
	}

	if err := sriovoperator.WaitForSriovAndMCPStable(
		APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
		NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace); err != nil {
		return fmt.Errorf("failed to wait for SR-IOV and MCP stability after bond policies: %w", err)
	}

	if err := CreateBondSlaveNetworks(params.Networks); err != nil {
		return err
	}

	return nil
}

// CreateBondSlaveNetworks creates bond slave SriovNetworks without IPAM.
func CreateBondSlaveNetworks(configs []BondNetworkConfig) error {
	for _, cfg := range configs {
		if err := CreateSriovBondNetwork(cfg.Name, cfg.Resource); err != nil {
			return fmt.Errorf("failed to create network %s: %w", cfg.Name, err)
		}
	}

	return nil
}

// DeleteBondSriovPolicies deletes the named SriovNetworkNodePolicies and waits for MCP to stabilize.
func DeleteBondSriovPolicies(policyNames []string) error {
	anyDeleted := false

	for _, name := range policyNames {
		policy, pullErr := sriov.PullPolicy(APIClient, name, NetConfig.SriovOperatorNamespace)
		if pullErr != nil {
			// eco-goinfra PullPolicy returns a custom "does not exist" error, not k8s NotFound.
			if strings.Contains(pullErr.Error(), "does not exist") {
				continue
			}

			return fmt.Errorf("failed to pull SR-IOV policy %s: %w", name, pullErr)
		}

		if err := policy.Delete(); err != nil {
			return fmt.Errorf("failed to delete SR-IOV policy %s: %w", name, err)
		}

		anyDeleted = true
	}

	if !anyDeleted {
		return nil
	}

	return sriovoperator.WaitForSriovAndMCPStable(
		APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
		NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
}
