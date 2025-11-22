package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// CustomResourceCleaner defines an interface for custom resource cleanup.
type CustomResourceCleaner interface {
	CleanupCustomResources() error
}

// OperatorUninstallConfig holds configuration for operator uninstallation.
type OperatorUninstallConfig struct {
	APIClient             *clients.Settings
	Namespace             string
	OperatorGroupName     string
	SubscriptionName      string
	SkipNamespaceDeletion bool
	SkipOperatorGroup     bool
	CustomResourceCleaner CustomResourceCleaner
	LogLevel              klog.Level
}

// OperatorUninstaller provides generic operator uninstallation functionality.
type OperatorUninstaller struct {
	config OperatorUninstallConfig
}

// NewOperatorUninstaller creates a new operator uninstaller with the given configuration.
func NewOperatorUninstaller(config OperatorUninstallConfig) *OperatorUninstaller {
	if config.LogLevel == 0 {
		config.LogLevel = klog.Level(DefaultLogLevel)
	}

	return &OperatorUninstaller{config: config}
}

// Uninstall removes the operator following the reverse OLM pattern.
func (u *OperatorUninstaller) Uninstall() error {
	klog.V(u.config.LogLevel).Infof("Starting operator uninstallation from namespace %s", u.config.Namespace)

	if err := u.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if u.config.CustomResourceCleaner != nil {
		klog.V(u.config.LogLevel).Infof("Running custom resource cleanup")

		if err := u.config.CustomResourceCleaner.CleanupCustomResources(); err != nil {
			klog.V(u.config.LogLevel).Infof("Warning: custom resource cleanup failed: %v", err)
		}
	}

	if err := u.deleteCSV(); err != nil {
		klog.V(u.config.LogLevel).Infof("Warning: failed to delete CSV: %v", err)
	}

	if err := u.deleteSubscription(); err != nil {
		klog.V(u.config.LogLevel).Infof("Warning: failed to delete subscription: %v", err)
	}

	if !u.config.SkipOperatorGroup {
		if err := u.deleteOperatorGroup(); err != nil {
			klog.V(u.config.LogLevel).Infof("Warning: failed to delete operator group: %v", err)
		}
	} else {
		klog.V(u.config.LogLevel).
			Infof("Skipping operator group deletion for shared operator group: %s",
				u.config.OperatorGroupName)
	}

	if !u.config.SkipNamespaceDeletion {
		if err := u.deleteNamespace(); err != nil {
			klog.V(u.config.LogLevel).Infof("Warning: failed to delete namespace: %v", err)
		}
	} else {
		klog.V(u.config.LogLevel).Infof("Skipping namespace deletion for global namespace: %s", u.config.Namespace)
	}

	klog.V(u.config.LogLevel).Infof("Operator uninstallation completed for namespace %s", u.config.Namespace)

	return nil
}

// validateConfig validates the operator uninstallation configuration.
func (u *OperatorUninstaller) validateConfig() error {
	if u.config.APIClient == nil {
		return fmt.Errorf("API client cannot be nil")
	}

	if u.config.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	return nil
}

// deleteCSV finds and deletes the ClusterServiceVersion.
func (u *OperatorUninstaller) deleteCSV() error {
	klog.V(u.config.LogLevel).Infof("Looking for CSV to delete in namespace %s", u.config.Namespace)

	clusterServices, err := olm.ListClusterServiceVersion(u.config.APIClient, u.config.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list CSVs: %w", err)
	}

	if len(clusterServices) == 0 {
		klog.V(u.config.LogLevel).Infof("No CSVs found in namespace %s", u.config.Namespace)

		return nil
	}

	for _, csv := range clusterServices {
		klog.V(u.config.LogLevel).Infof("Deleting CSV: %s", csv.Definition.Name)

		if err := u.deleteResourceWithTimeout(csv, 60*time.Second); err != nil {
			return fmt.Errorf("failed to delete CSV %s: %w", csv.Definition.Name, err)
		}

		klog.V(u.config.LogLevel).Infof("SUCCESS: CSV %s deleted", csv.Definition.Name)
	}

	return nil
}

// deleteSubscription deletes the subscription.
func (u *OperatorUninstaller) deleteSubscription() error {
	if u.config.SubscriptionName == "" {
		klog.V(u.config.LogLevel).Infof("No subscription name specified, skipping subscription deletion")

		return nil
	}

	klog.V(u.config.LogLevel).Infof("Deleting subscription: %s", u.config.SubscriptionName)

	sub, err := olm.PullSubscription(u.config.APIClient, u.config.SubscriptionName, u.config.Namespace)
	if err != nil {
		klog.V(u.config.LogLevel).Infof("Subscription %s not found: %v", u.config.SubscriptionName, err)

		return nil
	}

	if sub == nil {
		klog.V(u.config.LogLevel).Infof("Subscription %s not found", u.config.SubscriptionName)

		return nil
	}

	if err := u.deleteResourceWithTimeout(sub, 60*time.Second); err != nil {
		return fmt.Errorf("failed to delete subscription %s: %w", u.config.SubscriptionName, err)
	}

	klog.V(u.config.LogLevel).Infof("SUCCESS: Subscription %s deleted", u.config.SubscriptionName)

	return nil
}

// deleteOperatorGroup deletes the operator group.
func (u *OperatorUninstaller) deleteOperatorGroup() error {
	if u.config.OperatorGroupName == "" {
		klog.V(u.config.LogLevel).Infof("No operator group name specified, skipping operator group deletion")

		return nil
	}

	klog.V(u.config.LogLevel).Infof("Deleting operator group: %s", u.config.OperatorGroupName)

	operatorGroup, err := olm.PullOperatorGroup(u.config.APIClient, u.config.OperatorGroupName, u.config.Namespace)
	if err != nil {
		klog.V(u.config.LogLevel).Infof("OperatorGroup %s not found: %v", u.config.OperatorGroupName, err)

		return nil
	}

	if operatorGroup == nil {
		klog.V(u.config.LogLevel).Infof("OperatorGroup %s not found", u.config.OperatorGroupName)

		return nil
	}

	if err := u.deleteResourceWithTimeout(operatorGroup, 60*time.Second); err != nil {
		return fmt.Errorf("failed to delete operator group %s: %w", u.config.OperatorGroupName, err)
	}

	klog.V(u.config.LogLevel).Infof("SUCCESS: OperatorGroup %s deleted", u.config.OperatorGroupName)

	return nil
}

// deleteNamespace deletes the namespace if it exists.
func (u *OperatorUninstaller) deleteNamespace() error {
	klog.V(u.config.LogLevel).Infof("Deleting namespace: %s", u.config.Namespace)

	nsBuilder := namespace.NewBuilder(u.config.APIClient, u.config.Namespace)

	if !nsBuilder.Exists() {
		klog.V(u.config.LogLevel).Infof("Namespace %s does not exist", u.config.Namespace)

		return nil
	}

	err := nsBuilder.DeleteAndWait(120 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", u.config.Namespace, err)
	}

	klog.V(u.config.LogLevel).Infof("SUCCESS: Namespace %s deleted", u.config.Namespace)

	return nil
}

// deleteResourceWithTimeout deletes a resource and waits for it to be removed.
func (u *OperatorUninstaller) deleteResourceWithTimeout(resource interface{}, timeout time.Duration) error {
	type deletable interface {
		Delete() error
		Exists() bool
	}

	delRes, ok := resource.(deletable)
	if !ok {
		return fmt.Errorf("resource does not implement required delete interface")
	}

	if err := delRes.Delete(); err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			exists := delRes.Exists()

			return !exists, nil
		})
}
