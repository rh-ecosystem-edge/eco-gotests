package amdgpumachineconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	amdgpuBlacklistContent = "YmxhY2tsaXN0IGFtZGdwdQo="
	conditionStatusTrue    = "True"
)

// CreateAMDGPUBlacklist creates MachineConfig to blacklist amdgpu kernel module.
func CreateAMDGPUBlacklist(apiClient *clients.Settings, roleName string) error {
	mcBuilder, _ := mco.PullMachineConfig(apiClient, amdgpuparams.DefaultMachineConfigName)
	if mcBuilder != nil && mcBuilder.Exists() {
		glog.V(amdgpuparams.AMDGPULogLevel).Info("AMD GPU blacklist MachineConfig already exists")

		return nil
	}

	actualRole, err := determineNodeRole(apiClient, roleName)
	if err != nil {
		return fmt.Errorf("failed to determine node role: %w", err)
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Infof("Creating MachineConfig to blacklist amdgpu module for role: %s", actualRole)
	glog.V(amdgpuparams.AMDGPULogLevel).Info("WARNING: This will trigger automatic node reboots!")

	mcBuilder = mco.NewMCBuilder(apiClient, amdgpuparams.DefaultMachineConfigName)
	mcBuilder.WithLabel("machineconfiguration.openshift.io/role", actualRole)

	rawIgnitionJSON := `{
      "ignition": { "version": "3.2.0" },
      "storage": {
        "files": [
          {
            "path": "/etc/modprobe.d/amdgpu-blacklist.conf",
            "mode": 420,
            "overwrite": true,
            "contents": {
              "source": "data:text/plain;base64,` + amdgpuBlacklistContent + `"
            }
          }
        ]
      }
    }`
	mcBuilder.WithRawConfig([]byte(rawIgnitionJSON))

	glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfig YAML:\n%s", rawIgnitionJSON)

	// Create MachineConfig in cluster
	if _, err := mcBuilder.Create(); err != nil {
		return fmt.Errorf("failed to create MachineConfig: %w", err)
	}

	return nil
}

// DetermineMachineConfigPoolName returns the correct MachineConfigPool name for the cluster.
func DetermineMachineConfigPoolName(apiClient *clients.Settings) (string, error) {
	isSNO, err := amdgpucommon.IsSingleNodeOpenShift(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to determine cluster type: %w", err)
	}

	if isSNO {
		return "master", nil
	}

	return "worker", nil
}

// WaitForMachineConfigPoolStable waits for MachineConfigPool to be stable after MachineConfig creation.
func WaitForMachineConfigPoolStable(apiClient *clients.Settings, mcpName string, timeout time.Duration) error {
	glog.V(amdgpuparams.AMDGPULogLevel).
		Infof("Waiting for MachineConfigPool %s to be stable after MachineConfig update",
			mcpName)
	glog.V(amdgpuparams.AMDGPULogLevel).Infof("This includes waiting for node reboots to complete...")

	isSNO, err := amdgpucommon.IsSingleNodeOpenShift(apiClient)
	if err != nil {
		return fmt.Errorf("failed to determine cluster type: %w", err)
	}

	if isSNO {
		glog.V(amdgpuparams.AMDGPULogLevel).Info("SNO environment detected - using resilient polling for node reboot")

		return waitForMCPStableSNO(apiClient, mcpName, timeout)
	}

	return waitForMCPStableMultiNode(apiClient, mcpName, timeout)
}

// determineNodeRole automatically determines the correct MachineConfig role for the cluster.
func determineNodeRole(apiClient *clients.Settings, requestedRole string) (string, error) {
	isSNO, err := amdgpucommon.IsSingleNodeOpenShift(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to determine cluster type: %w", err)
	}

	if isSNO {
		glog.V(amdgpuparams.AMDGPULogLevel).Info("SNO environment detected - using 'master' role for MachineConfig")

		return "master", nil
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Infof("Multi-node environment detected - using requested role: %s", requestedRole)

	return requestedRole, nil
}

// waitForMCPStableMultiNode handles MCP stability waiting for multi-node clusters.
func waitForMCPStableMultiNode(apiClient *clients.Settings, mcpName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := waitForMCPUpdating(apiClient, mcpName, ctx)
	if err != nil {
		return fmt.Errorf("MachineConfigPool %s did not start updating: %w", mcpName, err)
	}

	err = waitForMCPStable(apiClient, mcpName, ctx)
	if err != nil {
		return fmt.Errorf("MachineConfigPool %s did not become stable: %w", mcpName, err)
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Infof(
		"MachineConfigPool %s is now stable - all nodes rebooted successfully", mcpName)

	return nil
}

// waitForMCPStableSNO handles MCP stability waiting for SNO clusters with API connection resilience.
func waitForMCPStableSNO(apiClient *clients.Settings, mcpName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for SNO node reboot and recovery...")

	return wait.PollUntilContextTimeout(
		ctx, 3*time.Minute, timeout, true, func(ctx context.Context) (bool, error) {
			return checkSNOMCPStatus(apiClient, mcpName)
		})
}

// checkSNOMCPStatus checks if SNO MCP is stable and updated.
func checkSNOMCPStatus(apiClient *clients.Settings, mcpName string) (bool, error) {
	mcpBuilder := mco.NewMCPBuilder(apiClient, mcpName)
	if mcpBuilder == nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to create MCP builder for %s (expected during SNO reboot)", mcpName)

		return false, nil
	}

	_, err := mcpBuilder.Get()
	if err != nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("API unavailable during SNO reboot (expected): %v", err)

		return false, nil
	}

	return evaluateMCPStability(mcpBuilder)
}

// evaluateMCPStability evaluates if MCP is stable and has Updated condition.
func evaluateMCPStability(mcpBuilder *mco.MCPBuilder) (bool, error) {
	mcpObj, err := mcpBuilder.Get()
	if err != nil {
		return false, nil
	}

	isStable := isMCPStable(mcpObj)
	if isStable && hasMCPUpdatedCondition(mcpObj) {
		glog.V(amdgpuparams.AMDGPULogLevel).Info("SNO node successfully rebooted and MCP is stable")

		return true, nil
	}

	logMCPStatus(mcpObj)

	return false, nil
}

// isMCPStable checks if MCP counts indicate stability.
func isMCPStable(mcp *mcv1.MachineConfigPool) bool {
	return mcp.Status.ReadyMachineCount == mcp.Status.MachineCount &&
		mcp.Status.MachineCount == mcp.Status.UpdatedMachineCount &&
		mcp.Status.DegradedMachineCount == 0
}

// hasMCPUpdatedCondition checks if MCP has Updated condition set to True.
func hasMCPUpdatedCondition(mcp *mcv1.MachineConfigPool) bool {
	for _, condition := range mcp.Status.Conditions {
		if condition.Type == "Updated" && condition.Status == conditionStatusTrue {
			return true
		}
	}

	return false
}

// logMCPStatus logs the current MCP status for debugging.
func logMCPStatus(mcp *mcv1.MachineConfigPool) {
	glog.V(amdgpuparams.AMDGPULogLevel).Infof("SNO MCP status: Ready=%d/%d, Updated=%d/%d, Degraded=%d",
		mcp.Status.ReadyMachineCount, mcp.Status.MachineCount,
		mcp.Status.UpdatedMachineCount, mcp.Status.MachineCount, mcp.Status.DegradedMachineCount)
}

// waitForMCPUpdating waits for the MachineConfigPool to start updating.
func waitForMCPUpdating(apiClient *clients.Settings, mcpName string, ctx context.Context) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Infof("Waiting for MachineConfigPool %s to start updating...", mcpName)

	err := wait.PollUntilContextTimeout(
		ctx, 30*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
			mcpBuilder := mco.NewMCPBuilder(apiClient, mcpName)
			if mcpBuilder == nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to create MCP builder for %s", mcpName)

				return false, nil
			}

			mcp, err := mcpBuilder.Get()
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Error getting MachineConfigPool %s: %v", mcpName, err)

				return false, nil
			}

			for _, condition := range mcp.Status.Conditions {
				if condition.Type == "Updating" && condition.Status == conditionStatusTrue {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfigPool %s started updating", mcpName)

					return true, nil
				}
			}

			glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfigPool %s not yet updating, waiting...", mcpName)

			return false, nil
		})

	return err
}

// waitForMCPStable waits for the MachineConfigPool to become stable.
func waitForMCPStable(apiClient *clients.Settings, mcpName string, ctx context.Context) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Infof("Waiting for MachineConfigPool %s to become stable...", mcpName)

	err := wait.PollUntilContextTimeout(
		ctx, 30*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
			mcpBuilder := mco.NewMCPBuilder(apiClient, mcpName)
			if mcpBuilder == nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to create MCP builder for %s", mcpName)

				return false, nil
			}

			mcp, err := mcpBuilder.Get()
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Error getting MachineConfigPool %s: %v", mcpName, err)

				return false, nil
			}

			isStable := mcp.Status.ReadyMachineCount == mcp.Status.MachineCount &&
				mcp.Status.MachineCount == mcp.Status.UpdatedMachineCount &&
				mcp.Status.DegradedMachineCount == 0

			if isStable {
				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Updated" && condition.Status == conditionStatusTrue {
						glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfigPool %s is stable and updated", mcpName)

						return true, nil
					}
				}
			}

			glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfigPool %s status: Ready=%d/%d, Updated=%d/%d, Degraded=%d",
				mcpName, mcp.Status.ReadyMachineCount, mcp.Status.MachineCount,
				mcp.Status.UpdatedMachineCount, mcp.Status.MachineCount, mcp.Status.DegradedMachineCount)

			return false, nil
		})

	return err
}
