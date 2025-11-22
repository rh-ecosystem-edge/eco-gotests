package nfddelete

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	nodefeature "github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"k8s.io/klog/v2"
)

// NFDCustomResourceCleaner implements CustomResourceCleaner for NFD operators.
type NFDCustomResourceCleaner struct {
	APIClient *clients.Settings
	Namespace string
	LogLevel  klog.Level
}

// NewNFDCustomResourceCleaner creates a new NFD custom resource cleaner.
func NewNFDCustomResourceCleaner(
	apiClient *clients.Settings,
	namespace string,
	logLevel klog.Level) *NFDCustomResourceCleaner {
	return &NFDCustomResourceCleaner{
		APIClient: apiClient,
		Namespace: namespace,
		LogLevel:  logLevel,
	}
}

// CleanupCustomResources implements the CustomResourceCleaner interface for NFD.
func (n *NFDCustomResourceCleaner) CleanupCustomResources() error {
	klog.V(n.LogLevel).Infof("Deleting NodeFeatureDiscovery custom resources in namespace %s", n.Namespace)

	potentialCRNames := []string{
		"nfd-instance",
		"nfd-instance-custom",
		"nfd-instance-test",
		"nfd",
		"nodefeaturelist",
	}

	deletedCount := 0

	for _, crName := range potentialCRNames {
		if err := n.deleteNFDCRByName(crName); err != nil {
			klog.V(n.LogLevel).Infof("NFD CR %s: %v", crName, err)
		} else {
			deletedCount++
		}
	}

	if deletedCount == 0 {
		klog.V(n.LogLevel).Infof("No NodeFeatureDiscovery custom resources found to delete")
	} else {
		klog.V(n.LogLevel).Infof("Successfully deleted %d NodeFeatureDiscovery custom resources", deletedCount)
	}

	return nil
}

// deleteNFDCRByName attempts to delete a specific NFD CR by name with finalizer handling.
func (n *NFDCustomResourceCleaner) deleteNFDCRByName(crName string) error {
	klog.V(n.LogLevel).Infof("Attempting to delete NFD CR: %s", crName)

	nfdCR, err := nodefeature.Pull(n.APIClient, crName, n.Namespace)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			klog.V(n.LogLevel).Infof("NFD CR %s not found (already deleted or doesn't exist)", crName)

			return nil
		}

		return fmt.Errorf("failed to pull NFD CR %s: %w", crName, err)
	}

	if nfdCR == nil {
		klog.V(n.LogLevel).Infof("NFD CR %s not found", crName)

		return nil
	}

	klog.V(n.LogLevel).Infof("Found NFD CR %s, proceeding with deletion", crName)

	if len(nfdCR.Object.Finalizers) > 0 {
		klog.V(n.LogLevel).Infof("Clearing finalizers for NFD CR %s: %v", crName, nfdCR.Object.Finalizers)

		nfdCR.Definition.Finalizers = []string{}

		_, err := nfdCR.Update(true)
		if err != nil {
			klog.V(n.LogLevel).Infof("Warning: failed to clear finalizers for NFD CR %s: %v", crName, err)

			klog.V(n.LogLevel).Infof("Successfully cleared finalizers for NFD CR %s", crName)
		}
	}

	_, err = nfdCR.Delete()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			klog.V(n.LogLevel).Infof("NFD CR %s already deleted during finalizer cleanup", crName)

			return nil
		}

		return fmt.Errorf("failed to delete NFD CR %s: %w", crName, err)
	}

	klog.V(n.LogLevel).Infof("Successfully deleted NFD CR %s", crName)

	return nil
}

// AllNFDCustomResources deletes all NFD custom resources by names.
// This is a convenience function for direct use in tests.
func AllNFDCustomResources(apiClient *clients.Settings, namespace string, crNames ...string) error {
	klog.V(nfdparams.LogLevel).Infof("Deleting specified NFD custom resources in namespace %s", namespace)

	if len(crNames) == 0 {
		crNames = []string{
			"nfd-instance",
			"nfd-instance-custom",
			"nfd-instance-test",
			"nfd",
		}
	}

	cleaner := NewNFDCustomResourceCleaner(apiClient, namespace, klog.Level(nfdparams.LogLevel))

	deletedCount := 0

	for _, crName := range crNames {
		if err := cleaner.deleteNFDCRByName(crName); err != nil {
			klog.V(nfdparams.LogLevel).Infof("NFD CR %s: %v", crName, err)
		} else {
			deletedCount++
		}
	}

	if deletedCount == 0 {
		klog.V(nfdparams.LogLevel).Infof("No NFD custom resources found to delete")
	} else {
		klog.V(nfdparams.LogLevel).Infof("Successfully deleted %d NFD custom resources", deletedCount)
	}

	return nil
}

// Interface verification.
var _ deploy.CustomResourceCleaner = (*NFDCustomResourceCleaner)(nil)
