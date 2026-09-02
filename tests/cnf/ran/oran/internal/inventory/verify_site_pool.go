package inventory

import (
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyOCloudSiteMatchesCR checks that an API O-Cloud Site matches the corresponding OCloudSite CR and related pools.
func VerifyOCloudSiteMatchesCR(
	apiSite oranapi.OCloudSiteInfo,
	siteCR *oran.OCloudSiteBuilder,
	readyPools []*oran.ResourcePoolBuilder,
) error {
	type view struct {
		Name             string
		GlobalLocationID string
		Description      string
	}

	var comparisonErr error
	if diff := cmp.Diff(
		view{siteCR.Definition.Name, siteCR.Definition.Spec.GlobalLocationName, siteCR.Definition.Spec.Description},
		view{apiSite.Name, apiSite.GlobalLocationId, apiSite.Description},
	); diff != "" {
		comparisonErr = fmt.Errorf("O-Cloud site mismatch (-want +got):\n%s", diff)
	}

	return errors.Join(
		comparisonErr,
		verifyStringSetEqual(
			collectExpectedPoolIDsForSite(siteCR, readyPools), extractAPIPoolIDs(apiSite),
			fmt.Sprintf("resourcePools for OCloudSite %s", siteCR.Definition.Name)),
	)
}

// collectExpectedPoolIDsForSite returns UIDs of ready ResourcePools bound to the given OCloudSite.
func collectExpectedPoolIDsForSite(siteCR *oran.OCloudSiteBuilder, readyPools []*oran.ResourcePoolBuilder) []string {
	var poolIDs []string

	for _, pool := range readyPools {
		if pool.Definition.Spec.OCloudSiteName == siteCR.Definition.Name {
			poolIDs = append(poolIDs, string(pool.Definition.UID))
		}
	}

	return poolIDs
}

// extractAPIPoolIDs returns string UIDs from an API OCloudSite's ResourcePools.
func extractAPIPoolIDs(apiSite oranapi.OCloudSiteInfo) []string {
	poolIDs := make([]string, 0, len(apiSite.ResourcePools))
	for _, poolID := range apiSite.ResourcePools {
		poolIDs = append(poolIDs, poolID.String())
	}

	return poolIDs
}

// VerifyResourcePoolMatchesCR checks that an API Resource Pool matches the corresponding ResourcePool CR.
func VerifyResourcePoolMatchesCR(apiPool oranapi.ResourcePool, poolCR *oran.ResourcePoolBuilder) error {
	type view struct {
		Name         string
		Description  string
		OCloudSiteID string
	}

	if diff := cmp.Diff(
		view{poolCR.Definition.Name, poolCR.Definition.Spec.Description,
			poolCR.Definition.Status.ResolvedOCloudSiteUID},
		view{apiPool.Name, apiPool.Description, apiPool.OCloudSiteId.String()},
	); diff != "" {
		return fmt.Errorf("resource pool mismatch (-want +got):\n%s", diff)
	}

	return nil
}
