package sriovocpenv

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
)

// IsMellanoxDevice reports whether the given interface uses the mlx5_core driver.
func IsMellanoxDevice(intName, nodeName string) (bool, error) {
	sriovNetworkState := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName,
		SriovOcpConfig.OcpSriovOperatorNamespace)

	driverName, err := sriovNetworkState.GetDriverName(intName)
	if err != nil {
		return false, fmt.Errorf("failed to get driver name for interface %s on node %s: %w", intName, nodeName, err)
	}

	return driverName == "mlx5_core", nil
}

// ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP configures Mellanox SR-IOV firmware and waits for MCP stability.
func ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
	mcpTimeout time.Duration,
	stableDuration time.Duration,
	workerNodes []*nodes.Builder,
	sriovInterfaceName string,
	enableSriov bool,
	numVfs int,
) error {
	for _, workerNode := range workerNodes {
		sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
			APIClient, workerNode.Object.Name, SriovOcpConfig.OcpSriovOperatorNamespace)

		pciAddress, err := sriovNetworkState.GetPciAddress(sriovInterfaceName)
		if err != nil {
			return fmt.Errorf("failed to get PCI address: %w", err)
		}

		mstconfigCmd := fmt.Sprintf("mstconfig -y -d %s set SRIOV_EN=%t NUM_OF_VFS=%d",
			pciAddress, enableSriov, numVfs)

		err = execOnSriovConfigDaemon(workerNode.Object.Name, mstconfigCmd)
		if err != nil {
			return fmt.Errorf("failed to configure Mellanox firmware on node %s: %w",
				workerNode.Object.Name, err)
		}

		_ = execOnSriovConfigDaemon(workerNode.Object.Name, "chroot /host reboot")
	}

	time.Sleep(10 * time.Second)

	return cluster.WaitForMcpStable(APIClient, mcpTimeout, stableDuration, SriovOcpConfig.MCPLabel)
}
