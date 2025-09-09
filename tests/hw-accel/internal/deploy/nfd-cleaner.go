package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"k8s.io/apimachinery/pkg/api/errors"
)

// NFDCustomResourceCleaner implements CustomResourceCleaner for NFD operators.
type NFDCustomResourceCleaner struct {
	APIClient *clients.Settings
	Namespace string
	LogLevel  glog.Level
}

// NewNFDCustomResourceCleaner creates a new NFD custom resource cleaner.
func NewNFDCustomResourceCleaner(
	apiClient *clients.Settings,
	namespace string,
	logLevel glog.Level) *NFDCustomResourceCleaner {
	return &NFDCustomResourceCleaner{
		APIClient: apiClient,
		Namespace: namespace,
		LogLevel:  logLevel,
	}
}

// CleanupCustomResources implements the CustomResourceCleaner interface for NFD.
func (n *NFDCustomResourceCleaner) CleanupCustomResources() error {
	glog.V(n.LogLevel).Infof("Deleting NodeFeatureDiscovery custom resources in namespace %s", n.Namespace)

	nfdCRName := "amd-gpu-nfd-instance"
	deletedCount := 0

	if err := n.deleteNFDCRByName(nfdCRName); err != nil {
		glog.V(n.LogLevel).Infof("NFD CR %s: %v", nfdCRName, err)
	} else {
		deletedCount++
	}

	if err := n.cleanupAMDGPUFeatureRule(); err != nil {
		glog.V(n.LogLevel).Infof("AMD GPU FeatureRule cleanup: %v", err)
	}

	if deletedCount == 0 {
		glog.V(n.LogLevel).Infof("No NodeFeatureDiscovery custom resources found to delete")
	} else {
		glog.V(n.LogLevel).Infof("Successfully deleted %d NodeFeatureDiscovery custom resources", deletedCount)
	}

	return nil
}

// deleteNFDCRByName attempts to delete a specific NFD CR by name with finalizer handling.
func (n *NFDCustomResourceCleaner) deleteNFDCRByName(crName string) error {
	glog.V(n.LogLevel).Infof("Attempting to delete NFD CR: %s", crName)

	nfdCR, err := nfd.Pull(n.APIClient, crName, n.Namespace)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			glog.V(n.LogLevel).Infof("NFD CR %s does not exist", crName)

			return fmt.Errorf("NFD CR %s not found", crName)
		}

		glog.V(n.LogLevel).Infof("Failed to pull NFD CR %s: %v", crName, err)

		return fmt.Errorf("failed to pull NFD CR %s: %w", crName, err)
	}

	if len(nfdCR.Object.GetFinalizers()) > 0 {
		glog.V(n.LogLevel).Infof("Removing finalizers from NFD CR %s", crName)
		nfdCR.Object.SetFinalizers([]string{})
		_, err = nfdCR.Update(true) // force=true to update finalizers

		if err != nil {
			glog.V(n.LogLevel).Infof("Warning: failed to remove finalizers from %s: %v", crName, err)
		}
	}

	// Delete the CR
	_, err = nfdCR.Delete()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			glog.V(n.LogLevel).Infof("NFD CR %s already deleted", crName)

			return nil
		}

		return fmt.Errorf("failed to delete NFD CR %s: %w", crName, err)
	}

	glog.V(n.LogLevel).Infof("Successfully deleted NFD CR: %s", crName)

	return nil
}

// cleanupAMDGPUFeatureRule cleans up the AMD GPU FeatureRule.
func (n *NFDCustomResourceCleaner) cleanupAMDGPUFeatureRule() error {
	nodeFeaturRuleBuilder, err := nfd.PullFeatureRule(n.APIClient, "amd-gpu-feature-rule", "openshift-amd-gpu")

	ctx := context.Background()

	if err == nil {
		glog.V(n.LogLevel).Info("Deleting AMD GPU FeatureRule")
		err = n.APIClient.Client.Delete(ctx, nodeFeaturRuleBuilder.Object)

		if err != nil {
			glog.V(n.LogLevel).Infof("Error deleting AMD GPU FeatureRule: %v", err)

			return err
		}

		glog.V(n.LogLevel).Info("Successfully deleted AMD GPU FeatureRule")
	} else if !errors.IsNotFound(err) {
		glog.V(n.LogLevel).Infof("Error checking AMD GPU FeatureRule: %v", err)

		return err
	}

	return nil
}

// Interface verification.
var _ CustomResourceCleaner = (*NFDCustomResourceCleaner)(nil)
