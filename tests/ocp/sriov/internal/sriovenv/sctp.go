package sriovenv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	sctpMachineConfigRoleLabel = "machineconfiguration.openshift.io/role"
	sctpIgnitionConfig         = `{"ignition":{"version":"3.2.0"},"storage":{"files":[` +
		`{"path":"/etc/modprobe.d/sctp-blacklist.conf","mode":420,"overwrite":true,` +
		`"contents":{"source":"data:,"}},` +
		`{"path":"/etc/modules-load.d/sctp-load.conf","mode":420,"overwrite":true,` +
		`"contents":{"source":"data:,sctp"}}]}}`
	sctpMCPUpdatingTimeout = 3 * time.Minute
	sctpMCPStableDuration  = 1 * time.Minute
)

// EnableSCTPOnWorkerNodes enables the SCTP kernel module on worker nodes when it is not already enabled.
// OpenShift/RHCOS blacklists SCTP by default; a MachineConfig is required so the module survives node reboots.
// The MachineConfig is left in place after the test.
func EnableSCTPOnWorkerNodes() error {
	klog.V(90).Infof("Enabling SCTP on worker nodes if it is not already enabled")

	loaded, err := isSCTPLoadedOnWorkers()
	if err != nil {
		return fmt.Errorf("failed to check SCTP module on worker nodes: %w", err)
	}

	mcExists := mco.NewMCBuilder(APIClient, tsparams.SCTPMachineConfigName).Exists()
	if loaded && mcExists {
		klog.V(90).Infof("SCTP is already enabled on worker nodes")

		return nil
	}

	if !mcExists {
		if err := createSCTPMachineConfig(); err != nil {
			return err
		}
	}

	if err := waitForWorkerMCPAfterSCTP(); err != nil {
		return fmt.Errorf("failed to wait for MCP after enabling SCTP: %w", err)
	}

	loaded, err = isSCTPLoadedOnWorkers()
	if err != nil {
		return fmt.Errorf("failed to verify SCTP module on worker nodes: %w", err)
	}

	if !loaded {
		return fmt.Errorf("SCTP kernel module is not loaded on all worker nodes after applying %s",
			tsparams.SCTPMachineConfigName)
	}

	return nil
}

func isSCTPLoadedOnWorkers() (bool, error) {
	outputs, err := cluster.ExecCmdWithStdout(APIClient, "lsmod | grep sctp || true",
		metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
	if err != nil {
		return false, err
	}

	if len(outputs) == 0 {
		return false, nil
	}

	for nodeName, output := range outputs {
		if !strings.Contains(output, "sctp") {
			klog.V(90).Infof("SCTP module is not loaded on node %s", nodeName)

			return false, nil
		}
	}

	return true, nil
}

func createSCTPMachineConfig() error {
	klog.V(90).Infof("Creating MachineConfig %s to load the SCTP kernel module", tsparams.SCTPMachineConfigName)

	_, err := mco.NewMCBuilder(APIClient, tsparams.SCTPMachineConfigName).
		WithLabel(sctpMachineConfigRoleLabel, workerMCPName()).
		WithRawConfig([]byte(sctpIgnitionConfig)).
		Create()
	if err != nil {
		return fmt.Errorf("failed to create MachineConfig %s: %w", tsparams.SCTPMachineConfigName, err)
	}

	return nil
}

func workerMCPName() string {
	if SriovOcpConfig.MCPLabel != "" {
		return SriovOcpConfig.MCPLabel
	}

	return SriovOcpConfig.WorkerLabelEnvVar
}

func waitForWorkerMCPAfterSCTP() error {
	mcpName := workerMCPName()

	mcp, err := mco.Pull(APIClient, mcpName)
	if err != nil {
		return fmt.Errorf("failed to pull MCP %s: %w", mcpName, err)
	}

	_ = wait.PollUntilContextTimeout(context.TODO(), tsparams.MCPStableInterval, sctpMCPUpdatingTimeout, true,
		func(ctx context.Context) (bool, error) {
			if !mcp.Exists() {
				return false, nil
			}

			status := mcp.Object.Status

			return status.UpdatedMachineCount != status.MachineCount ||
				status.ReadyMachineCount != status.MachineCount, nil
		})

	return cluster.WaitForMcpStable(APIClient, tsparams.MCOWaitTimeout, sctpMCPStableDuration, mcpName)
}
