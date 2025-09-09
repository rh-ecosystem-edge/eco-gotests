package amdgpuhelpers

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpudelete"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpumachineconfig"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
)

// AMDGPUInstallConfigOptions holds optional overrides for operator installation configuration.
type AMDGPUInstallConfigOptions struct {
	OperatorGroupName      *string
	SubscriptionName       *string
	CatalogSource          *string
	CatalogSourceNamespace *string
	Channel                *string
	SkipOperatorGroup      *bool
	TargetNamespaces       []string
	LogLevel               *glog.Level
}

// GetDefaultKMMInstallConfig returns the standard KMM installation configuration.
func GetDefaultKMMInstallConfig(
	apiClient *clients.Settings,
	options *AMDGPUInstallConfigOptions) deploy.OperatorInstallConfig {
	config := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              "openshift-kmm",
		OperatorGroupName:      "kernel-module-management",
		SubscriptionName:       "kernel-module-management",
		PackageName:            "kernel-module-management",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "stable",
		SkipOperatorGroup:      false,
		TargetNamespaces:       []string{},
		LogLevel:               glog.Level(amdgpuparams.AMDGPULogLevel),
	}

	if options != nil {
		if options.OperatorGroupName != nil {
			config.OperatorGroupName = *options.OperatorGroupName
		}

		if options.SubscriptionName != nil {
			config.SubscriptionName = *options.SubscriptionName
		}

		if options.CatalogSource != nil {
			config.CatalogSource = *options.CatalogSource
		}

		if options.CatalogSourceNamespace != nil {
			config.CatalogSourceNamespace = *options.CatalogSourceNamespace
		}

		if options.Channel != nil {
			config.Channel = *options.Channel
		}

		if options.SkipOperatorGroup != nil {
			config.SkipOperatorGroup = *options.SkipOperatorGroup
		}

		if len(options.TargetNamespaces) > 0 {
			config.TargetNamespaces = options.TargetNamespaces
		}

		if options.LogLevel != nil {
			config.LogLevel = *options.LogLevel
		}
	}

	return config
}

// GetAlternativeKMMInstallConfig returns alternative KMM installation configuration.
// Tries community operators catalog with fast channel.
func GetAlternativeKMMInstallConfig(
	apiClient *clients.Settings,
	options *AMDGPUInstallConfigOptions) deploy.OperatorInstallConfig {
	config := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              "openshift-kmm",
		OperatorGroupName:      "kernel-module-management",
		SubscriptionName:       "kernel-module-management",
		PackageName:            "kernel-module-management",
		CatalogSource:          "community-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "fast",
		SkipOperatorGroup:      false,
		TargetNamespaces:       []string{""},
		LogLevel:               glog.Level(amdgpuparams.AMDGPULogLevel),
	}

	if options != nil {
		if options.OperatorGroupName != nil {
			config.OperatorGroupName = *options.OperatorGroupName
		}

		if options.SubscriptionName != nil {
			config.SubscriptionName = *options.SubscriptionName
		}

		if options.CatalogSource != nil {
			config.CatalogSource = *options.CatalogSource
		}

		if options.CatalogSourceNamespace != nil {
			config.CatalogSourceNamespace = *options.CatalogSourceNamespace
		}

		if options.Channel != nil {
			config.Channel = *options.Channel
		}

		if options.SkipOperatorGroup != nil {
			config.SkipOperatorGroup = *options.SkipOperatorGroup
		}

		if len(options.TargetNamespaces) > 0 {
			config.TargetNamespaces = options.TargetNamespaces
		}

		if options.LogLevel != nil {
			config.LogLevel = *options.LogLevel
		}
	}

	return config
}

// GetLegacyKMMInstallConfig returns legacy KMM installation configuration.
// Tries operator with suffix and older channel.
func GetLegacyKMMInstallConfig(
	apiClient *clients.Settings,
	options *AMDGPUInstallConfigOptions) deploy.OperatorInstallConfig {
	config := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              "openshift-kmm",
		OperatorGroupName:      "kernel-module-management",
		SubscriptionName:       "kernel-module-management-operator",
		PackageName:            "kernel-module-management-operator",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "1.0",
		SkipOperatorGroup:      false,
		TargetNamespaces:       []string{"openshift-kmm"},
		LogLevel:               glog.Level(amdgpuparams.AMDGPULogLevel),
	}

	if options != nil {
		if options.OperatorGroupName != nil {
			config.OperatorGroupName = *options.OperatorGroupName
		}

		if options.SubscriptionName != nil {
			config.SubscriptionName = *options.SubscriptionName
		}

		if options.CatalogSource != nil {
			config.CatalogSource = *options.CatalogSource
		}

		if options.CatalogSourceNamespace != nil {
			config.CatalogSourceNamespace = *options.CatalogSourceNamespace
		}

		if options.Channel != nil {
			config.Channel = *options.Channel
		}

		if options.SkipOperatorGroup != nil {
			config.SkipOperatorGroup = *options.SkipOperatorGroup
		}

		if len(options.TargetNamespaces) > 0 {
			config.TargetNamespaces = options.TargetNamespaces
		}

		if options.LogLevel != nil {
			config.LogLevel = *options.LogLevel
		}
	}

	return config
}

// GetCustomKMMInstallConfig returns a custom KMM configuration for manual testing.
// Use this when you know the exact package name, channel, and catalog source that work in your environment.
func GetCustomKMMInstallConfig(
	apiClient *clients.Settings,
	packageName string,
	channel string,
	catalogSource string) deploy.OperatorInstallConfig {
	return deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              "openshift-operators",
		OperatorGroupName:      "global-operators",
		SubscriptionName:       packageName,
		PackageName:            packageName,
		CatalogSource:          catalogSource,
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                channel,
		SkipOperatorGroup:      true,
		LogLevel:               glog.Level(amdgpuparams.AMDGPULogLevel),
	}
}

// GetDefaultAMDGPUInstallConfig returns the standard AMD GPU installation configuration.
func GetDefaultAMDGPUInstallConfig(
	apiClient *clients.Settings,
	options *AMDGPUInstallConfigOptions) deploy.OperatorInstallConfig {
	config := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              amdgpuparams.AMDGPUNamespace,
		OperatorGroupName:      "amd-gpu-operator-group",
		SubscriptionName:       "amd-gpu-subscription",
		PackageName:            "amd-gpu-operator",
		CatalogSource:          "certified-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "alpha",

		TargetNamespaces: []string{},
		LogLevel:         glog.Level(amdgpuparams.AMDGPULogLevel),
	}

	if options != nil {
		if options.OperatorGroupName != nil {
			config.OperatorGroupName = *options.OperatorGroupName
		}

		if options.SubscriptionName != nil {
			config.SubscriptionName = *options.SubscriptionName
		}

		if options.CatalogSource != nil {
			config.CatalogSource = *options.CatalogSource
		}

		if options.CatalogSourceNamespace != nil {
			config.CatalogSourceNamespace = *options.CatalogSourceNamespace
		}

		if options.Channel != nil {
			config.Channel = *options.Channel
		}

		if options.SkipOperatorGroup != nil {
			config.SkipOperatorGroup = *options.SkipOperatorGroup
		}

		if options.LogLevel != nil {
			config.LogLevel = *options.LogLevel
		}
	}

	return config
}

// GetDefaultAMDGPUUninstallConfig returns the standard AMD GPU uninstallation configuration.
func GetDefaultAMDGPUUninstallConfig(
	apiClient *clients.Settings,
	operatorGroupName,
	subscriptionName string) deploy.OperatorUninstallConfig {
	amdgpuCleaner := amdgpudelete.NewAMDGPUCustomResourceCleaner(
		apiClient,
		amdgpuparams.AMDGPUNamespace,
		glog.Level(amdgpuparams.AMDGPULogLevel))

	return deploy.OperatorUninstallConfig{
		APIClient:             apiClient,
		Namespace:             amdgpuparams.AMDGPUNamespace,
		OperatorGroupName:     operatorGroupName,
		SubscriptionName:      subscriptionName,
		CustomResourceCleaner: amdgpuCleaner,
		LogLevel:              glog.Level(amdgpuparams.AMDGPULogLevel),
	}
}

// GetDefaultKMMUninstallConfig returns the standard KMM uninstallation configuration.
func GetDefaultKMMUninstallConfig(
	apiClient *clients.Settings,
	options *AMDGPUInstallConfigOptions) deploy.OperatorUninstallConfig {
	kmmCleaner := deploy.NewKMMCustomResourceCleaner(
		apiClient,
		amdgpuparams.AMDGPUNamespace,
		glog.Level(amdgpuparams.AMDGPULogLevel))

	return deploy.OperatorUninstallConfig{
		APIClient:             apiClient,
		Namespace:             "openshift-kmm",
		OperatorGroupName:     "kernel-module-management",
		SubscriptionName:      "kernel-module-management",
		CustomResourceCleaner: kmmCleaner,
		LogLevel:              glog.Level(amdgpuparams.AMDGPULogLevel),
	}
}

// CreateBlacklistMachineConfig creates a MachineConfig to blacklist the amdgpu kernel module.
func CreateBlacklistMachineConfig(apiClient *clients.Settings) error {
	glog.V(amdgpuparams.AMDGPULogLevel).Info(
		"Creating MachineConfig to blacklist amdgpu module (auto-detecting SNO vs multi-node)")

	err := amdgpumachineconfig.CreateAMDGPUBlacklist(apiClient, "worker")
	if err != nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfig creation result: %v", err)

		return fmt.Errorf("MachineConfig creation requires cluster admin privileges or may already exist")
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info(
		"Waiting for MachineConfigPool to become stable after node reboots (auto-detecting MCP name)")

	mcpName, err := amdgpumachineconfig.DetermineMachineConfigPoolName(apiClient)
	if err != nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to determine MachineConfigPool name: %v", err)

		return fmt.Errorf("failed to determine correct MachineConfigPool name")
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Infof("Using MachineConfigPool: %s", mcpName)

	err = amdgpumachineconfig.WaitForMachineConfigPoolStable(apiClient, mcpName, 60*time.Minute)
	if err != nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfigPool stability check failed: %v", err)

		return fmt.Errorf("MachineConfigPool stability check failed - may need more time or manual intervention")
	}

	glog.V(amdgpuparams.AMDGPULogLevel).Info("Verifying amdgpu kernel module is properly blacklisted")

	err = amdgpuconfig.VerifyAMDGPUKernelModule(apiClient)
	if err != nil {
		glog.V(amdgpuparams.AMDGPULogLevel).Infof("Kernel module verification failed (ignoring): %v", err)
	}

	return nil
}
