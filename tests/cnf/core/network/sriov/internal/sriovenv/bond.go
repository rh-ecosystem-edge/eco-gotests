package sriovenv

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
)

// Bond CNI mode strings for CreateBondNAD. Must match eco-goinfra/pkg/nad validModes
// (balance-rr, active-backup, balance-xor, balance-tlb, balance-alb). NMState-only modes
// such as 802.3ad are defined in the LACP test suite.
const (
	BondModeActiveBackup = "active-backup"
	BondModeBalanceRR    = "balance-rr"
	BondModeBalanceXOR   = "balance-xor"

	// BondInterfaceName is the default bond master interface created by the Bond CNI plugin.
	BondInterfaceName = "bond0"
	// BondSlave1IfName and BondSlave2IfName are the default first two slave links in a 2-slave bond NAD.
	BondSlave1IfName = "net1"
	BondSlave2IfName = "net2"

	// BondActiveSlavePollInterval is the poll interval when waiting for bond slave state changes.
	BondActiveSlavePollInterval = 100 * time.Millisecond
	// BondActiveSlaveChangeTimeout is the maximum wait for bond active_slave or MII state transitions.
	BondActiveSlaveChangeTimeout = 30 * time.Second
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
		WithIPAM(&nad.IPAM{Type: ipamType})

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
	if slaveCount < 2 {
		return nil, fmt.Errorf("slaveCount must be >= 2, got %d", slaveCount)
	}

	var links []nad.Link

	for idx := 1; idx <= slaveCount; idx++ {
		links = append(links, nad.Link{Name: fmt.Sprintf("net%d", idx)})
	}

	ipam := nad.IPAMWhereAbouts(ipRange, gateway)
	if ipam == nil {
		return nil, fmt.Errorf("invalid whereabouts IPAM range %q gateway %q", ipRange, gateway)
	}

	plugin := nad.NewMasterBondPlugin(nadName, mode).
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks(links).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(ipam)

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
	deadline := time.Now().Add(BondActiveSlaveChangeTimeout)

	var last string

	for {
		slave, err := GetBondActiveSlave(clientPod, bondName)
		if err != nil {
			return "", err
		}

		last = slave

		if slave != "" && slave != previousSlave {
			return slave, nil
		}

		if time.Now().After(deadline) {
			return last, fmt.Errorf(
				"bond did not switch active slave from %q within %v (last active_slave=%q)",
				previousSlave, BondActiveSlaveChangeTimeout, last)
		}

		time.Sleep(BondActiveSlavePollInterval)
	}
}

// WaitForBondSlaveMIIDown polls until the given slave is MII-down, the bond is still up,
// and at least one other slave remains MII-up (degraded but operational).
func WaitForBondSlaveMIIDown(clientPod *pod.Builder, bondName, downSlave string) error {
	deadline := time.Now().Add(BondActiveSlaveChangeTimeout)

	var lastSlaves map[string]string

	for {
		bondUp, err := isBondInterfaceUp(clientPod, bondName)
		if err != nil {
			return err
		}

		slaves, err := getBondSlaveMIIStatuses(clientPod, bondName)
		if err != nil {
			return err
		}

		lastSlaves = slaves

		if bondUp && slaves[downSlave] == "down" && countBondSlavesWithMII(slaves, "up") >= 1 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"bond %q did not stabilize with slave %q MII down within %v (bondUp=%t, slaves=%v)",
				bondName, downSlave, BondActiveSlaveChangeTimeout, bondUp, lastSlaves)
		}

		time.Sleep(BondActiveSlavePollInterval)
	}
}

// WaitForBondSlavesMIIUp polls until all bond slaves report MII up and the bond is up.
func WaitForBondSlavesMIIUp(clientPod *pod.Builder, bondName string) error {
	deadline := time.Now().Add(BondActiveSlaveChangeTimeout)

	var lastSlaves map[string]string

	for {
		bondUp, err := isBondInterfaceUp(clientPod, bondName)
		if err != nil {
			return err
		}

		slaves, err := getBondSlaveMIIStatuses(clientPod, bondName)
		if err != nil {
			return err
		}

		lastSlaves = slaves

		if bondUp &&
			len(slaves) >= 2 &&
			countBondSlavesWithMII(slaves, "up") >= 2 &&
			countBondSlavesWithMII(slaves, "down") == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"bond %q did not recover all slaves to MII up within %v (bondUp=%t, slaves=%v)",
				bondName, BondActiveSlaveChangeTimeout, bondUp, lastSlaves)
		}

		time.Sleep(BondActiveSlavePollInterval)
	}
}

// SetLinkStatus sets a network interface operstate to up or down inside a pod.
func SetLinkStatus(podBuilder *pod.Builder, nic, status string) error {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c", fmt.Sprintf("ip link set dev %s %s", nic, status)})
	if err != nil {
		return fmt.Errorf("failed to set interface %s %s: %w (out=%s)", nic, status, err, out.String())
	}

	return nil
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
