package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	operatorsV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// DefaultLogLevel for operator installation logging.
	DefaultLogLevel = 90
)

// OperatorInstallConfig holds configuration for operator installation.
type OperatorInstallConfig struct {
	APIClient              *clients.Settings
	Namespace              string
	OperatorGroupName      string
	SubscriptionName       string
	PackageName            string
	CatalogSource          string
	CatalogSourceNamespace string
	Channel                string
	SkipNamespaceCreation  bool
	SkipOperatorGroup      bool
	TargetNamespaces       []string
	LogLevel               glog.Level
	InstallPlanApproval    string
}

// OperatorInstaller provides generic operator installation functionality.
type OperatorInstaller struct {
	config   OperatorInstallConfig
	csvUtils *CSVUtils
}

// NewOperatorInstaller creates a new operator installer with the given configuration.
func NewOperatorInstaller(config OperatorInstallConfig) *OperatorInstaller {
	if config.LogLevel == 0 {
		config.LogLevel = glog.Level(DefaultLogLevel)
	}

	csvUtils := NewCSVUtils(config.APIClient, config.Namespace, config.LogLevel)

	return &OperatorInstaller{
		config:   config,
		csvUtils: csvUtils,
	}
}

// Install deploys the operator following the standard OLM pattern.
func (o *OperatorInstaller) Install() error {
	glog.V(o.config.LogLevel).Infof("Starting operator installation: %s in namespace %s",
		o.config.PackageName, o.config.Namespace)

	if err := o.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if !o.config.SkipNamespaceCreation {
		if err := o.createNamespaceIfNotExist(); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}

		glog.V(o.config.LogLevel).
			Infof("SUCCESS: Namespace %s created/verified",
				o.config.Namespace)
	} else {
		glog.V(o.config.LogLevel).
			Infof("Skipping namespace creation for global namespace: %s",
				o.config.Namespace)
	}

	if !o.config.SkipOperatorGroup {
		if err := o.createOperatorGroup(); err != nil {
			return fmt.Errorf("failed to create operator group: %w", err)
		}

		glog.V(o.config.LogLevel).Infof("SUCCESS: OperatorGroup %s created", o.config.OperatorGroupName)
	} else {
		glog.V(o.config.LogLevel).Infof("Skipping operator group creation, using existing: %s", o.config.OperatorGroupName)
	}

	if err := o.createSubscription(); err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	glog.V(o.config.LogLevel).Infof("SUCCESS: Subscription %s created", o.config.SubscriptionName)

	return nil
}

// IsReady checks if the operator CSV is ready.
func (o *OperatorInstaller) IsReady(timeout time.Duration) (bool, error) {
	glog.V(o.config.LogLevel).Infof("Checking operator readiness for package=%s, namespace=%s",
		o.config.PackageName, o.config.Namespace)

	ready, err := o.csvUtils.WaitForCSVReady(o.config.PackageName, timeout)
	if err != nil {
		return false, fmt.Errorf("operator CSV readiness check failed: %w", err)
	}

	if ready {
		glog.V(o.config.LogLevel).Infof("Operator %s is ready (CSV in Succeeded state)", o.config.PackageName)
		glog.V(o.config.LogLevel).Infof("Operator %s installation completed", o.config.PackageName)
	}

	return ready, nil
}

// GetNamespace returns the operator namespace.
func (o *OperatorInstaller) GetNamespace() string {
	return o.config.Namespace
}

// GetPackageName returns the operator package name.
func (o *OperatorInstaller) GetPackageName() string {
	return o.config.PackageName
}

// GetCSVUtils returns the CSV utilities for advanced operations.
func (o *OperatorInstaller) GetCSVUtils() *CSVUtils {
	return o.csvUtils
}

// GetCSVStatus returns the current CSV status for debugging purposes.
func (o *OperatorInstaller) GetCSVStatus() (string, error) {
	csv, err := o.csvUtils.GetCSVByPackageName(o.config.PackageName)
	if err != nil {
		return "", fmt.Errorf("failed to get CSV for package %s: %w", o.config.PackageName, err)
	}

	return string(csv.Object.Status.Phase), nil
}

// validateConfig validates the operator installation configuration.
func (o *OperatorInstaller) validateConfig() error {
	if o.config.APIClient == nil {
		return fmt.Errorf("API client cannot be nil")
	}

	if o.config.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	if o.config.PackageName == "" {
		return fmt.Errorf("package name cannot be empty")
	}

	if o.config.SubscriptionName == "" {
		return fmt.Errorf("subscription name cannot be empty")
	}

	if o.config.CatalogSource == "" {
		return fmt.Errorf("catalog source cannot be empty")
	}

	return nil
}

// createNamespaceIfNotExist creates the namespace if it doesn't exist.
func (o *OperatorInstaller) createNamespaceIfNotExist() error {
	glog.V(o.config.LogLevel).Infof("Creating namespace %s if it doesn't exist", o.config.Namespace)

	nsBuilder := namespace.NewBuilder(o.config.APIClient, o.config.Namespace)

	if nsBuilder.Exists() {
		nsObj, err := o.config.APIClient.CoreV1Interface.Namespaces().Get(
			context.TODO(), o.config.Namespace, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get namespace %s status: %w", o.config.Namespace, err)
		}

		if nsObj.Status.Phase == corev1.NamespaceTerminating {
			glog.V(o.config.LogLevel).
				Infof("Namespace %s is terminating, waiting for deletion to complete...",
					o.config.Namespace)

			if err := o.waitForNamespaceDeletion(); err != nil {
				return fmt.Errorf("failed waiting for namespace %s deletion: %w", o.config.Namespace, err)
			}

			glog.V(o.config.LogLevel).
				Infof("Namespace deletion complete, creating fresh namespace %s",
					o.config.Namespace)
		} else {
			glog.V(o.config.LogLevel).
				Infof("Namespace %s already exists and is active",
					o.config.Namespace)

			return nil
		}
	}

	_, err := nsBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", o.config.Namespace, err)
	}

	glog.V(o.config.LogLevel).
		Infof("Successfully created namespace %s",
			o.config.Namespace)

	return nil
}

// waitForNamespaceDeletion waits for the namespace to be deleted.
func (o *OperatorInstaller) waitForNamespaceDeletion() error {
	timeout := 5 * time.Minute
	glog.V(o.config.LogLevel).Infof("Waiting up to %v for namespace %s to be deleted", timeout, o.config.Namespace)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := o.config.APIClient.CoreV1Interface.Namespaces().Get(
				ctx, o.config.Namespace, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					glog.V(o.config.LogLevel).Infof("Namespace %s has been successfully deleted", o.config.Namespace)

					return true, nil
				}

				glog.V(o.config.LogLevel).Infof("Error checking namespace %s: %v", o.config.Namespace, err)

				return false, err
			}

			glog.V(o.config.LogLevel).Infof("Namespace %s still exists, continuing to wait...", o.config.Namespace)

			return false, nil
		})

	if err != nil {
		return fmt.Errorf("timeout waiting for namespace %s deletion: %w", o.config.Namespace, err)
	}

	return nil
}

// createOperatorGroup creates the operator group if it doesn't exist.
func (o *OperatorInstaller) createOperatorGroup() error {
	glog.V(o.config.LogLevel).Infof("Creating operator group %s in namespace %s",
		o.config.OperatorGroupName, o.config.Namespace)

	operatorGroupBuilder := olm.NewOperatorGroupBuilder(
		o.config.APIClient, o.config.OperatorGroupName, o.config.Namespace)

	operatorGroupBuilder.Definition.Spec.TargetNamespaces = o.config.TargetNamespaces

	if operatorGroupBuilder.Exists() {
		glog.V(o.config.LogLevel).Infof("OperatorGroup %s already exists", o.config.OperatorGroupName)

		return nil
	}

	_, err := operatorGroupBuilder.Create()
	if err != nil {
		if strings.Contains(err.Error(), "is being terminated") ||
			strings.Contains(err.Error(), "NamespaceTerminating") {
			return fmt.Errorf("cannot create operator group in terminating namespace %s. "+
				"Please wait for namespace deletion to complete or use a different namespace: %w",
				o.config.Namespace, err)
		}

		return fmt.Errorf("failed to create operator group %s: %w", o.config.OperatorGroupName, err)
	}

	return nil
}

// createSubscription creates the subscription if it doesn't exist.
func (o *OperatorInstaller) createSubscription() error {
	glog.V(o.config.LogLevel).Infof("Creating subscription %s in namespace %s",
		o.config.SubscriptionName, o.config.Namespace)
	glog.V(o.config.LogLevel).Infof("Subscription details: Package=%s, CatalogSource=%s, Channel=%s",
		o.config.PackageName, o.config.CatalogSource, o.config.Channel)

	sub := olm.NewSubscriptionBuilder(
		o.config.APIClient,
		o.config.SubscriptionName,
		o.config.Namespace,
		o.config.CatalogSource,
		o.config.CatalogSourceNamespace,
		o.config.PackageName)

	sub.WithChannel(o.config.Channel)
	sub.WithInstallPlanApproval(convertToInstallPlanApproval(o.config.InstallPlanApproval))

	if sub.Exists() {
		glog.V(o.config.LogLevel).Infof("Subscription %s already exists", o.config.SubscriptionName)

		return nil
	}

	_, err := sub.Create()

	if err != nil {
		if strings.Contains(err.Error(), "is being terminated") ||
			strings.Contains(err.Error(), "NamespaceTerminating") {
			return fmt.Errorf("cannot create subscription in terminating namespace %s. "+
				"Please wait for namespace deletion to complete or use a different namespace: %w",
				o.config.Namespace, err)
		}

		return fmt.Errorf("failed to create subscription %s: %w", o.config.SubscriptionName, err)
	}

	return nil
}

func convertToInstallPlanApproval(approval string) operatorsV1alpha1.Approval {
	switch strings.ToLower(approval) {
	case "manual":
		return operatorsV1alpha1.ApprovalManual
	default:
		return operatorsV1alpha1.ApprovalAutomatic
	}
}
