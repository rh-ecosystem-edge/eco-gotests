package inventory

import (
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyDeploymentManagerMatchesCluster checks that an API Deployment Manager matches a ManagedCluster.
func VerifyDeploymentManagerMatchesCluster(
	apiManager oranapi.DeploymentManager,
	cluster *ocm.ManagedClusterBuilder,
) error {
	type view struct {
		Name       string
		ServiceURI string
	}

	var comparisonErr error
	if diff := cmp.Diff(
		view{Name: cluster.Definition.Name, ServiceURI: deriveServiceURI(cluster)},
		view{Name: apiManager.Name, ServiceURI: apiManager.ServiceUri},
	); diff != "" {
		comparisonErr = fmt.Errorf("deployment manager mismatch (-want +got):\n%s", diff)
	}

	return errors.Join(comparisonErr, verifyDeploymentManagerRequiredFields(apiManager))
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

// deriveServiceURI returns the first non-empty ManagedCluster client config URL.
func deriveServiceURI(cluster *ocm.ManagedClusterBuilder) string {
	for _, clientConfig := range cluster.Definition.Spec.ManagedClusterClientConfigs {
		if clientConfig.URL != "" {
			return clientConfig.URL
		}
	}

	return ""
}
