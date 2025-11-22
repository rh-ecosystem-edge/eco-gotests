package deploy

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"k8s.io/klog/v2"
)

// CSVUtils provides utilities for ClusterServiceVersion operations.
type CSVUtils struct {
	APIClient *clients.Settings
	Namespace string
	LogLevel  klog.Level
}

// NewCSVUtils creates a new CSV utilities instance.
func NewCSVUtils(apiClient *clients.Settings, namespace string, logLevel klog.Level) *CSVUtils {
	return &CSVUtils{
		APIClient: apiClient,
		Namespace: namespace,
		LogLevel:  logLevel,
	}
}

// GetCSVByPackageName finds a CSV in the namespace by package name.
func (c *CSVUtils) GetCSVByPackageName(packageName string) (*olm.ClusterServiceVersionBuilder, error) {
	klog.V(c.LogLevel).Infof("Looking for CSV with package name '%s' in namespace '%s'", packageName, c.Namespace)

	csvList, err := olm.ListClusterServiceVersion(c.APIClient, c.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list CSVs in namespace %s: %w", c.Namespace, err)
	}

	if len(csvList) == 0 {
		return nil, fmt.Errorf("no CSVs found in namespace %s", c.Namespace)
	}

	for _, csv := range csvList {
		if (strings.Contains(strings.ToLower(csv.Object.Name), strings.ToLower(packageName)) ||
			strings.Contains(strings.ToLower(csv.Object.Spec.DisplayName), strings.ToLower(packageName))) &&
			csv.Object.Namespace == c.Namespace {
			klog.V(c.LogLevel).Infof("Found CSV: %s for package %s", csv.Object.Name, packageName)

			return csv, nil
		}
	}

	return nil, fmt.Errorf("no CSV found for package '%s' in namespace %s", packageName, c.Namespace)
}

// GetCSVByName gets a specific CSV by name in the namespace.
func (c *CSVUtils) GetCSVByName(csvName string) (*olm.ClusterServiceVersionBuilder, error) {
	klog.V(c.LogLevel).Infof("Getting CSV '%s' in namespace '%s'", csvName, c.Namespace)

	csv, err := olm.PullClusterServiceVersion(c.APIClient, csvName, c.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get CSV %s in namespace %s: %w", csvName, c.Namespace, err)
	}

	if csv == nil {
		return nil, fmt.Errorf("CSV %s not found in namespace %s", csvName, c.Namespace)
	}

	return csv, nil
}

// IsCSVReady checks if a CSV is in the Succeeded phase.
func (c *CSVUtils) IsCSVReady(csv *olm.ClusterServiceVersionBuilder) (bool, error) {
	if csv == nil {
		return false, fmt.Errorf("CSV is nil")
	}

	if csv.Object.Namespace != c.Namespace {
		return false, fmt.Errorf("CSV %s is in namespace %s, expected namespace %s",
			csv.Object.Name, csv.Object.Namespace, c.Namespace)
	}

	phase := csv.Object.Status.Phase

	klog.V(c.LogLevel).Infof("CSV %s status: %s", csv.Object.Name, phase)

	switch phase {
	case v1alpha1.CSVPhaseSucceeded:
		klog.V(c.LogLevel).Infof("CSV %s is ready (Succeeded)", csv.Object.Name)

		return true, nil

	case v1alpha1.CSVPhaseFailed:
		return false, fmt.Errorf("CSV %s failed: %s", csv.Object.Name, csv.Object.Status.Message)

	case v1alpha1.CSVPhasePending, v1alpha1.CSVPhaseInstalling, v1alpha1.CSVPhaseInstallReady:
		klog.V(c.LogLevel).Infof("CSV %s is still installing (phase: %s)", csv.Object.Name, phase)

		return false, nil

	case v1alpha1.CSVPhaseReplacing, v1alpha1.CSVPhaseDeleting:
		klog.V(c.LogLevel).Infof("CSV %s is being replaced or deleted (phase: %s)", csv.Object.Name, phase)

		return false, nil

	case v1alpha1.CSVPhaseUnknown, v1alpha1.CSVPhaseAny:
		klog.V(c.LogLevel).Infof("CSV %s in unknown/any phase: %s", csv.Object.Name, phase)

		return false, nil

	default:
		klog.V(c.LogLevel).Infof("CSV %s in unexpected phase: %s", csv.Object.Name, phase)

		return false, nil
	}
}

// WaitForCSVReady waits for a CSV to become ready within the specified timeout.
func (c *CSVUtils) WaitForCSVReady(packageName string, timeout time.Duration) (bool, error) {
	klog.V(c.LogLevel).Infof("Waiting for CSV readiness for package '%s' in namespace '%s' (timeout: %v)",
		packageName, c.Namespace, timeout)

	start := time.Now()
	ticker := time.NewTicker(10 * time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			csv, err := c.GetCSVByPackageName(packageName)
			if err != nil {
				klog.V(c.LogLevel).Infof("CSV not found yet for package %s: %v", packageName, err)

				if time.Since(start) < timeout {
					continue
				}

				return false, fmt.Errorf("timeout waiting for CSV for package %s: %w", packageName, err)
			}

			ready, err := c.IsCSVReady(csv)
			if err != nil {
				return false, fmt.Errorf("CSV readiness check failed: %w", err)
			}

			if ready {
				klog.V(c.LogLevel).Infof("CSV for package %s is ready after %v", packageName, time.Since(start))

				return true, nil
			}

			if time.Since(start) >= timeout {
				return false, fmt.Errorf("timeout waiting for CSV readiness for package %s after %v", packageName, timeout)
			}

		case <-time.After(timeout):
			return false, fmt.Errorf("timeout waiting for CSV readiness for package %s", packageName)
		}
	}
}

// ListCSVsInNamespace returns all CSVs in the namespace.
func (c *CSVUtils) ListCSVsInNamespace() ([]*olm.ClusterServiceVersionBuilder, error) {
	klog.V(c.LogLevel).Infof("Listing all CSVs in namespace '%s'", c.Namespace)

	csvList, err := olm.ListClusterServiceVersion(c.APIClient, c.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list CSVs in namespace %s: %w", c.Namespace, err)
	}

	klog.V(c.LogLevel).Infof("Found %d CSVs in namespace %s", len(csvList), c.Namespace)

	for _, csv := range csvList {
		klog.V(c.LogLevel).Infof("CSV: %s, Phase: %s, Version: %s",
			csv.Object.Name, csv.Object.Status.Phase, csv.Object.Spec.Version)
	}

	return csvList, nil
}

// GetCSVForOperator gets the CSV for a specific operator installation.
func (c *CSVUtils) GetCSVForOperator(packageName, subscriptionName string) (*olm.ClusterServiceVersionBuilder, error) {
	klog.V(c.LogLevel).Infof("Getting CSV for operator package '%s' via subscription '%s'", packageName, subscriptionName)

	if subscriptionName != "" {
		subscription, err := olm.PullSubscription(c.APIClient, subscriptionName, c.Namespace)
		if err == nil && subscription != nil && subscription.Object.Status.CurrentCSV != "" {
			csvName := subscription.Object.Status.CurrentCSV
			klog.V(c.LogLevel).Infof("Found CSV name '%s' from subscription '%s'", csvName, subscriptionName)

			csv, err := c.GetCSVByName(csvName)
			if err == nil {
				return csv, nil
			}

			klog.V(c.LogLevel).
				Infof("Failed to get CSV %s from subscription, falling back to package search: %v",
					csvName,
					err)
		}
	}

	return c.GetCSVByPackageName(packageName)
}
