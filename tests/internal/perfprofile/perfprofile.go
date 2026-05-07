package perfprofile

import (
	"fmt"
	"time"

	v2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"k8s.io/klog/v2"
)

// DeployPerformanceProfile installs performanceProfile on cluster.
func DeployPerformanceProfile(
	apiClient *clients.Settings,
	workerLabelMap map[string]string,
	mcpLabel,
	profileName,
	isolatedCPU,
	reservedCPU string,
	hugePages1GCount int32,
	mcpWaitTimeout time.Duration) error {
	klog.V(90).Infof("Ensuring cluster has correct PerformanceProfile deployed")

	mcp, err := mco.Pull(apiClient, mcpLabel)
	if err != nil {
		return fmt.Errorf("fail to pull MCP due to : %w", err)
	}

	performanceProfiles, err := nto.ListProfiles(apiClient)
	if err != nil {
		return fmt.Errorf("fail to list PerformanceProfile objects on cluster due to: %w", err)
	}

	if len(performanceProfiles) > 0 {
		for _, perfProfile := range performanceProfiles {
			if perfProfile.Object.Name == profileName {
				klog.V(90).Infof("PerformanceProfile %s exists", profileName)

				return nil
			}
		}

		klog.V(90).Infof("PerformanceProfile doesn't exist on cluster. Removing all pre-existing profiles")

		err := nto.CleanAllPerformanceProfiles(apiClient)
		if err != nil {
			return fmt.Errorf("fail to clean pre-existing performance profiles due to %w", err)
		}

		err = mcp.WaitToBeStableFor(time.Minute, mcpWaitTimeout)
		if err != nil {
			return err
		}
	}

	klog.V(90).Infof("Required PerformanceProfile doesn't exist. Installing new profile PerformanceProfile")

	_, err = nto.NewBuilder(apiClient, profileName, isolatedCPU, reservedCPU, workerLabelMap).
		WithHugePages("1G", []v2.HugePage{{Size: "1G", Count: hugePages1GCount}}).Create()
	if err != nil {
		return fmt.Errorf("fail to deploy PerformanceProfile due to: %w", err)
	}

	return mcp.WaitToBeStableFor(time.Minute, mcpWaitTimeout)
}
