package neuronhelpers

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	"k8s.io/klog/v2"
)

// operatorsDeployedByTest tracks whether operators were deployed by the test.
// Uses atomic operations for thread-safe access during concurrent test execution.
var operatorsDeployedByTest int32

// GetOperatorsDeployedByTest returns whether operators were deployed by the test.
// Thread-safe for concurrent access.
func GetOperatorsDeployedByTest() bool {
	return atomic.LoadInt32(&operatorsDeployedByTest) == 1
}

// SetOperatorsDeployedByTest sets whether operators were deployed by the test.
// Thread-safe for concurrent access.
func SetOperatorsDeployedByTest(deployed bool) {
	if deployed {
		atomic.StoreInt32(&operatorsDeployedByTest, 1)
	} else {
		atomic.StoreInt32(&operatorsDeployedByTest, 0)
	}
}

const (
	// OperatorReadinessCheckTimeout is the timeout for checking if an operator is ready.
	OperatorReadinessCheckTimeout = 30 * time.Second
)

const (
	// DefaultNeuronPackageName is the default package name for the Neuron operator.
	DefaultNeuronPackageName = "aws-neuron-operator"
	// DefaultNeuronChannel is the default channel for the Neuron operator.
	DefaultNeuronChannel = "Fast"
	// DefaultNeuronCatalogSource is the default catalog source.
	DefaultNeuronCatalogSource = "community-operators"
	// DefaultNeuronCatalogSourceNamespace is the default catalog source namespace.
	DefaultNeuronCatalogSourceNamespace = "openshift-marketplace"
)

// NeuronInstallConfigOptions holds optional configuration for Neuron operator installation.
type NeuronInstallConfigOptions struct {
	CatalogSource          *string
	CatalogSourceNamespace *string
	Channel                *string
}

// StringPtr returns a pointer to a string.
func StringPtr(s string) *string {
	return &s
}

// GetDefaultNeuronInstallConfig returns the default installation configuration for Neuron operator.
func GetDefaultNeuronInstallConfig(apiClient *clients.Settings,
	options *NeuronInstallConfigOptions) deploy.OperatorInstallConfig {
	catalogSource := DefaultNeuronCatalogSource
	catalogSourceNamespace := DefaultNeuronCatalogSourceNamespace
	channel := DefaultNeuronChannel

	if options != nil {
		if options.CatalogSource != nil {
			catalogSource = *options.CatalogSource
		}

		if options.CatalogSourceNamespace != nil {
			catalogSourceNamespace = *options.CatalogSourceNamespace
		}

		if options.Channel != nil {
			channel = *options.Channel
		}
	}

	return deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              params.NeuronNamespace,
		OperatorGroupName:      "neuron-operator-group",
		SubscriptionName:       "neuron-subscription",
		PackageName:            DefaultNeuronPackageName,
		CatalogSource:          catalogSource,
		CatalogSourceNamespace: catalogSourceNamespace,
		Channel:                channel,
		TargetNamespaces:       nil, // Empty = AllNamespaces mode (required by Neuron)
		LogLevel:               klog.Level(params.NeuronLogLevel),
		InstallPlanApproval:    "Automatic",
	}
}

// GetDefaultKMMInstallConfig returns the default installation configuration for KMM operator.
func GetDefaultKMMInstallConfig(apiClient *clients.Settings) deploy.OperatorInstallConfig {
	return deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              "openshift-kmm",
		OperatorGroupName:      "kmm-operator-group",
		SubscriptionName:       "kmm-subscription",
		PackageName:            "kernel-module-management",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "stable",
		TargetNamespaces:       nil,
		LogLevel:               klog.Level(params.NeuronLogLevel),
		InstallPlanApproval:    "Automatic",
	}
}

// AreAllOperatorsReady checks if NFD, KMM, and Neuron operators are already deployed and ready.
func AreAllOperatorsReady(apiClient *clients.Settings, neuronOptions *NeuronInstallConfigOptions) bool {
	klog.V(params.NeuronLogLevel).Info("Checking if all operators are already ready")

	// Check NFD
	nfdInstallConfig := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              params.NFDNamespace,
		OperatorGroupName:      "nfd-operator-group",
		SubscriptionName:       "nfd-subscription",
		PackageName:            "nfd",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "stable",
		TargetNamespaces:       []string{params.NFDNamespace},
		LogLevel:               klog.Level(params.NeuronLogLevel),
	}
	nfdInstaller := deploy.NewOperatorInstaller(nfdInstallConfig)
	nfdReady, err := nfdInstaller.IsReady(OperatorReadinessCheckTimeout)

	if err != nil || !nfdReady {
		klog.V(params.NeuronLogLevel).Infof("NFD operator not ready: %v", err)

		return false
	}

	klog.V(params.NeuronLogLevel).Info("NFD operator is already ready")

	// Check KMM
	kmmInstallConfig := GetDefaultKMMInstallConfig(apiClient)
	kmmInstaller := deploy.NewOperatorInstaller(kmmInstallConfig)
	kmmReady, err := kmmInstaller.IsReady(OperatorReadinessCheckTimeout)

	if err != nil || !kmmReady {
		klog.V(params.NeuronLogLevel).Infof("KMM operator not ready: %v", err)

		return false
	}

	klog.V(params.NeuronLogLevel).Info("KMM operator is already ready")

	// Check Neuron
	neuronInstallConfig := GetDefaultNeuronInstallConfig(apiClient, neuronOptions)
	neuronInstaller := deploy.NewOperatorInstaller(neuronInstallConfig)
	neuronReady, err := neuronInstaller.IsReady(OperatorReadinessCheckTimeout)

	if err != nil || !neuronReady {
		klog.V(params.NeuronLogLevel).Infof("Neuron operator not ready: %v", err)

		return false
	}

	klog.V(params.NeuronLogLevel).Info("Neuron operator is already ready")

	return true
}

// deployNFDOperator deploys the NFD operator and creates the NFD instance.
func deployNFDOperator(apiClient *clients.Settings) error {
	klog.V(params.NeuronLogLevel).Info("Deploying NFD operator")

	nfdInstallConfig := deploy.OperatorInstallConfig{
		APIClient:              apiClient,
		Namespace:              params.NFDNamespace,
		OperatorGroupName:      "nfd-operator-group",
		SubscriptionName:       "nfd-subscription",
		PackageName:            "nfd",
		CatalogSource:          "redhat-operators",
		CatalogSourceNamespace: "openshift-marketplace",
		Channel:                "stable",
		TargetNamespaces:       []string{params.NFDNamespace},
		LogLevel:               klog.Level(params.NeuronLogLevel),
		InstallPlanApproval:    "Automatic",
	}

	nfdInstaller := deploy.NewOperatorInstaller(nfdInstallConfig)
	if err := nfdInstaller.Install(); err != nil {
		return fmt.Errorf("failed to deploy NFD operator: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Waiting for NFD operator to be ready")

	nfdReady, err := nfdInstaller.IsReady(10 * time.Minute)
	if err != nil || !nfdReady {
		return fmt.Errorf("NFD operator not ready: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("NFD operator ready, creating NFD instance")

	return CreateNFDInstance(apiClient)
}

// deployNeuronOperator deploys the Neuron operator and creates the NFD rule.
func deployNeuronOperator(apiClient *clients.Settings, options *NeuronInstallConfigOptions) error {
	klog.V(params.NeuronLogLevel).Info("Deploying Neuron operator (AllNamespaces mode)")

	neuronInstallConfig := GetDefaultNeuronInstallConfig(apiClient, options)
	neuronInstaller := deploy.NewOperatorInstaller(neuronInstallConfig)

	if err := neuronInstaller.Install(); err != nil {
		return fmt.Errorf("failed to deploy Neuron operator: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Waiting for Neuron operator to be ready")

	neuronReady, err := neuronInstaller.IsReady(10 * time.Minute)
	if err != nil || !neuronReady {
		return fmt.Errorf("neuron operator not ready: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Creating Neuron NFD rule")

	if !NFDRuleExists(apiClient, params.NeuronNamespace) {
		return CreateNeuronNFDRule(apiClient, params.NeuronNamespace)
	}

	klog.V(params.NeuronLogLevel).Info("Neuron NFD rule already exists")

	return nil
}

// DeployAllOperators deploys NFD, KMM, and Neuron operators if not already ready.
func DeployAllOperators(apiClient *clients.Settings, neuronOptions *NeuronInstallConfigOptions) error {
	klog.V(params.NeuronLogLevel).Info("Checking if operators are already deployed")

	if AreAllOperatorsReady(apiClient, neuronOptions) {
		klog.V(params.NeuronLogLevel).Info("All operators already ready - skipping deployment")

		SetOperatorsDeployedByTest(false)

		return nil
	}

	klog.V(params.NeuronLogLevel).Info("Operators not ready, proceeding with deployment")

	if err := deployNFDOperator(apiClient); err != nil {
		return err
	}

	klog.V(params.NeuronLogLevel).Info("Deploying KMM operator (AllNamespaces mode)")

	kmmInstaller := deploy.NewOperatorInstaller(GetDefaultKMMInstallConfig(apiClient))
	if err := kmmInstaller.Install(); err != nil {
		return fmt.Errorf("failed to deploy KMM operator: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Waiting for KMM operator to be ready")

	kmmReady, err := kmmInstaller.IsReady(10 * time.Minute)
	if err != nil || !kmmReady {
		return fmt.Errorf("KMM operator not ready: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("KMM operator ready")

	if err := deployNeuronOperator(apiClient, neuronOptions); err != nil {
		return err
	}

	// Only set flag after all operators deployed successfully
	SetOperatorsDeployedByTest(true)

	return nil
}

// UninstallAllOperators uninstalls Neuron, KMM, and NFD operators in reverse order.
func UninstallAllOperators(apiClient *clients.Settings) error {
	if !GetOperatorsDeployedByTest() {
		klog.V(params.NeuronLogLevel).Info("Operators were pre-existing - skipping uninstall")

		return nil
	}

	var errors []error

	// Delete NFD Rule first (before Neuron operator is removed)
	klog.V(params.NeuronLogLevel).Info("Deleting Neuron NFD rule")

	if NFDRuleExists(apiClient, params.NeuronNamespace) {
		if err := DeleteNeuronNFDRule(apiClient, params.NeuronNamespace); err != nil {
			klog.V(params.NeuronLogLevel).Infof("NFD rule deletion error: %v", err)
			errors = append(errors, err)
		}
	}

	// Uninstall Neuron operator
	klog.V(params.NeuronLogLevel).Info("Uninstalling Neuron operator")

	neuronUninstallConfig := deploy.OperatorUninstallConfig{
		APIClient:         apiClient,
		Namespace:         params.NeuronNamespace,
		OperatorGroupName: "neuron-operator-group",
		SubscriptionName:  "neuron-subscription",
		LogLevel:          klog.Level(params.NeuronLogLevel),
	}
	neuronUninstaller := deploy.NewOperatorUninstaller(neuronUninstallConfig)

	if err := neuronUninstaller.Uninstall(); err != nil {
		klog.V(params.NeuronLogLevel).Infof("Neuron operator uninstall error: %v", err)
		errors = append(errors, err)
	}

	// Uninstall KMM operator
	klog.V(params.NeuronLogLevel).Info("Uninstalling KMM operator")

	kmmUninstallConfig := deploy.OperatorUninstallConfig{
		APIClient:         apiClient,
		Namespace:         "openshift-kmm",
		OperatorGroupName: "kmm-operator-group",
		SubscriptionName:  "kmm-subscription",
		LogLevel:          klog.Level(params.NeuronLogLevel),
	}
	kmmUninstaller := deploy.NewOperatorUninstaller(kmmUninstallConfig)

	if err := kmmUninstaller.Uninstall(); err != nil {
		klog.V(params.NeuronLogLevel).Infof("KMM operator uninstall error: %v", err)
		errors = append(errors, err)
	}

	// Delete NFD Instance before uninstalling NFD operator
	klog.V(params.NeuronLogLevel).Info("Deleting NFD instance")

	if NFDInstanceExists(apiClient) {
		if err := DeleteNFDInstance(apiClient); err != nil {
			klog.V(params.NeuronLogLevel).Infof("NFD instance deletion error: %v", err)
			errors = append(errors, err)
		}
	}

	// Uninstall NFD operator
	klog.V(params.NeuronLogLevel).Info("Uninstalling NFD operator")

	nfdUninstallConfig := deploy.OperatorUninstallConfig{
		APIClient:         apiClient,
		Namespace:         params.NFDNamespace,
		OperatorGroupName: "nfd-operator-group",
		SubscriptionName:  "nfd-subscription",
		LogLevel:          klog.Level(params.NeuronLogLevel),
	}
	nfdUninstaller := deploy.NewOperatorUninstaller(nfdUninstallConfig)

	if err := nfdUninstaller.Uninstall(); err != nil {
		klog.V(params.NeuronLogLevel).Infof("NFD operator uninstall error: %v", err)
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("uninstall completed with %d errors", len(errors))
	}

	return nil
}
