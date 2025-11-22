package nfdhelpers

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfddelete"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"k8s.io/klog/v2"
)

// NFDInstallConfigOptions holds optional overrides for NFD installation configuration.
type NFDInstallConfigOptions struct {
	OperatorGroupName      *string
	SubscriptionName       *string
	CatalogSource          *string
	CatalogSourceNamespace *string
	Channel                *string
	LogLevel               *klog.Level
}

// GetDefaultNFDInstallConfig returns the standard NFD installation configuration with optional overrides.
func GetDefaultNFDInstallConfig(
	apiClient *clients.Settings,
	options *NFDInstallConfigOptions) deploy.OperatorInstallConfig {
	config := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              nfdparams.NFDNamespace,
		OperatorGroupName:      "nfd-operator-group",
		SubscriptionName:       "nfd-subscription",
		PackageName:            "nfd",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "stable",
		TargetNamespaces:       []string{nfdparams.NFDNamespace},
		LogLevel:               klog.Level(nfdparams.LogLevel),
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

		if options.LogLevel != nil {
			config.LogLevel = *options.LogLevel
		}
	}

	return config
}

// GetDefaultNFDUninstallConfig returns the standard NFD uninstallation configuration.
func GetDefaultNFDUninstallConfig(apiClient *clients.Settings,
	operatorGroupName,
	subscriptionName string) deploy.OperatorUninstallConfig {
	nfdCleaner := nfddelete.NewNFDCustomResourceCleaner(apiClient, nfdparams.NFDNamespace, klog.Level(nfdparams.LogLevel))

	return deploy.OperatorUninstallConfig{
		APIClient:             apiClient,
		Namespace:             nfdparams.NFDNamespace,
		OperatorGroupName:     operatorGroupName,
		SubscriptionName:      subscriptionName,
		CustomResourceCleaner: nfdCleaner,
		LogLevel:              klog.Level(nfdparams.LogLevel),
	}
}

// GetGenericOperatorUninstallConfig returns uninstall configuration for non-NFD operators.
func GetGenericOperatorUninstallConfig(apiClient *clients.Settings,
	namespace,
	operatorGroupName,
	subscriptionName string) deploy.OperatorUninstallConfig {
	return deploy.OperatorUninstallConfig{
		APIClient:             apiClient,
		Namespace:             namespace,
		OperatorGroupName:     operatorGroupName,
		SubscriptionName:      subscriptionName,
		CustomResourceCleaner: nil,
		LogLevel:              klog.Level(nfdparams.LogLevel),
	}
}

// GetOperatorUninstallConfigWithCleaner returns uninstall configuration with custom resource cleaner.
// Example usage for other operators (KMM, AMD GPU, etc.):
//
//	type KMMCustomResourceCleaner struct { ... }
//	func (k *KMMCustomResourceCleaner) CleanupCustomResources() error { ... }
//
//	kmmCleaner := &KMMCustomResourceCleaner{...}
//	config := GetOperatorUninstallConfigWithCleaner(apiClient, "kmm-namespace", "kmm-group", "kmm-sub", kmmCleaner)
func GetOperatorUninstallConfigWithCleaner(
	apiClient *clients.Settings,
	namespace, operatorGroupName, subscriptionName string,
	cleaner deploy.CustomResourceCleaner) deploy.OperatorUninstallConfig {
	return deploy.OperatorUninstallConfig{
		APIClient:             apiClient,
		Namespace:             namespace,
		OperatorGroupName:     operatorGroupName,
		SubscriptionName:      subscriptionName,
		CustomResourceCleaner: cleaner,
		LogLevel:              klog.Level(nfdparams.LogLevel),
	}
}

// StringPtr returns a pointer to the given string (helper for options).
func StringPtr(s string) *string {
	return &s
}

// LogLevelPtr returns a pointer to the given klog.Level (helper for options).
func LogLevelPtr(level klog.Level) *klog.Level {
	return &level
}

// GetStandardNFDConfig returns the most common NFD configuration.
func GetStandardNFDConfig(apiClient *clients.Settings) deploy.OperatorInstallConfig {
	return GetDefaultNFDInstallConfig(apiClient, nil)
}

// GetUpgradeTestNFDConfig returns NFD configuration optimized for upgrade tests.
func GetUpgradeTestNFDConfig(apiClient *clients.Settings, catalogSource string) deploy.OperatorInstallConfig {
	options := &NFDInstallConfigOptions{
		OperatorGroupName: StringPtr("op-nfd"),
		SubscriptionName:  StringPtr("nfd"),
		CatalogSource:     StringPtr(catalogSource),
	}

	return GetDefaultNFDInstallConfig(apiClient, options)
}

// GetNFDCSVUtils returns a CSV utility instance for NFD operations.
func GetNFDCSVUtils(apiClient *clients.Settings) *deploy.CSVUtils {
	return deploy.NewCSVUtils(apiClient, nfdparams.NFDNamespace, klog.Level(nfdparams.LogLevel))
}

// WaitForNFDOperatorReady waits for NFD operator CSV to be ready - convenience function.
func WaitForNFDOperatorReady(apiClient *clients.Settings, timeout time.Duration) (bool, error) {
	csvUtils := GetNFDCSVUtils(apiClient)

	return csvUtils.WaitForCSVReady("nfd", timeout)
}

// CheckNFDOperatorStatus checks the current status of the NFD operator CSV.
func CheckNFDOperatorStatus(apiClient *clients.Settings) (string, error) {
	csvUtils := GetNFDCSVUtils(apiClient)

	csv, err := csvUtils.GetCSVByPackageName("nfd")
	if err != nil {
		return "", err
	}

	return string(csv.Object.Status.Phase), nil
}
