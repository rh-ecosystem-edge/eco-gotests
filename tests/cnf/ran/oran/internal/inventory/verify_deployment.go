package inventory

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyDeploymentManagerMatchesCluster checks that an API Deployment Manager matches a ManagedCluster.
func VerifyDeploymentManagerMatchesCluster(
	apiManager oranapi.DeploymentManager,
	cluster *ocm.ManagedClusterBuilder,
) error {
	var errs []error

	errs = appendMismatch(errs, "name", cluster.Definition.Name, apiManager.Name)
	errs = appendError(errs, verifyDeploymentManagerRequiredFields(apiManager))

	expectedURI, uriErr := expectedServiceURI(cluster)
	errs = appendError(errs, uriErr)
	errs = appendMismatch(errs, "serviceUri", expectedURI, apiManager.ServiceUri)

	return errors.Join(errs...)
}

// verifyDeploymentManagerRequiredFields checks fields that must be present on an API Deployment Manager.
func verifyDeploymentManagerRequiredFields(apiManager oranapi.DeploymentManager) error {
	var errs []error

	if apiManager.Description == "" {
		errs = append(errs, fmt.Errorf("description: want non-empty, got empty"))
	}

	if apiManager.OCloudId == uuid.Nil {
		errs = append(errs, fmt.Errorf("oCloudId: want non-nil UUID, got %v", apiManager.OCloudId))
	}

	return errors.Join(errs...)
}

// expectedServiceURI returns the first non-empty ManagedCluster client config URL.
func expectedServiceURI(cluster *ocm.ManagedClusterBuilder) (string, error) {
	for _, clientConfig := range cluster.Definition.Spec.ManagedClusterClientConfigs {
		if clientConfig.URL != "" {
			return clientConfig.URL, nil
		}
	}

	return "", fmt.Errorf("ManagedCluster %s has no client config URL", cluster.Definition.Name)
}
