package amdgpuhelpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuconfig"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"k8s.io/klog/v2"
)

const (
	// timeout for operator installation and readiness in SNO environments
	// where MachineConfig changes can trigger node reboots taking 30-40 minutes.
	timeout = 60 * time.Minute
)

// formatAMDGPUStartingCSV formats the version string to the proper StartingCSV format.
// If version is "1.4.1", returns "amd-gpu-operator.v1.4.1".
// If version already starts with "amd-gpu-operator", returns as-is.
func formatAMDGPUStartingCSV(version string) string {
	// If already in full CSV format, return as-is
	if strings.HasPrefix(version, "amd-gpu-operator") {
		return version
	}

	// Add "v" prefix if not present
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return fmt.Sprintf("amd-gpu-operator.%s", version)
}

// DeployAllOperators deploys NFD, KMM, and AMD GPU operators using the generic installer.
func DeployAllOperators(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Deploying all operators")

	operators := []string{"nfd", "kmm", "amdgpu"}
	for _, operator := range operators {
		config := getConfigByName(operator, apiClient)
		if config.Namespace == "" {
			return fmt.Errorf("invalid operator name: %s", operator)
		}

		installer := deploy.NewOperatorInstaller(config)

		err := installer.Install()
		if err != nil {
			return fmt.Errorf("failed to install %s operator: %w", operator, err)
		}

		_, err = installer.IsReady(timeout)
		if err != nil {
			return fmt.Errorf("%s operator readiness check failed: %w", operator, err)
		}
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("All operators deployed successfully")

	return nil
}

func getConfigByName(operatorName string, apiClient *clients.Settings) deploy.OperatorInstallConfig {
	switch strings.ToLower(operatorName) {
	case "nfd":
		return deploy.OperatorInstallConfig{
			APIClient:              apiClient,
			Namespace:              nfdparams.NFDNamespace,
			OperatorGroupName:      "nfd-operator-group",
			SubscriptionName:       "nfd-subscription",
			PackageName:            "nfd",
			CatalogSource:          "redhat-operators",
			CatalogSourceNamespace: "openshift-marketplace",
			Channel:                "stable",
			TargetNamespaces:       []string{nfdparams.NFDNamespace}, // NFD only watches its own namespace
			LogLevel:               klog.Level(amdgpuparams.AMDGPULogLevel),
		}
	case "kmm":
		return GetDefaultKMMInstallConfig(apiClient, nil)
	case "amdgpu":
		var options *AMDGPUInstallConfigOptions

		amdConfig := amdgpuconfig.NewAMDConfig()
		if amdConfig != nil && amdConfig.AMDOperatorVersion != "" {
			// Format the StartingCSV as "amd-gpu-operator.vX.Y.Z"
			// User provides version like "1.4.1", we need "amd-gpu-operator.v1.4.1"
			startingCSV := formatAMDGPUStartingCSV(amdConfig.AMDOperatorVersion)
			options = &AMDGPUInstallConfigOptions{
				StartingCSV: &startingCSV,
			}
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Using AMD GPU operator StartingCSV: %s", startingCSV)
		}

		return GetDefaultAMDGPUInstallConfig(apiClient, options)
	default:
		return deploy.OperatorInstallConfig{}
	}
}
