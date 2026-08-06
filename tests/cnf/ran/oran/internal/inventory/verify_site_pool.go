package inventory

import (
	"errors"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyOCloudSiteMatchesCR checks that an API O-Cloud Site matches the corresponding OCloudSite CR and related pools.
func VerifyOCloudSiteMatchesCR(
	apiSite oranapi.OCloudSiteInfo,
	siteCR *oran.OCloudSiteBuilder,
	readyPools []*oran.ResourcePoolBuilder,
) error {
	var errs []error

	errs = appendMismatch(errs, "name", siteCR.Definition.Name, apiSite.Name)
	errs = appendMismatch(errs, "globalLocationId", siteCR.Definition.Spec.GlobalLocationName, apiSite.GlobalLocationId)
	errs = appendMismatch(errs, "description", siteCR.Definition.Spec.Description, apiSite.Description)
	errs = appendError(errs, verifyStringSetEqual(
		expectedPoolIDsForSite(siteCR, readyPools), apiPoolIDs(apiSite),
		fmt.Sprintf("resourcePools for OCloudSite %s", siteCR.Definition.Name)))

	return errors.Join(errs...)
}

// expectedPoolIDsForSite returns UIDs of ready ResourcePools bound to the given OCloudSite.
func expectedPoolIDsForSite(siteCR *oran.OCloudSiteBuilder, readyPools []*oran.ResourcePoolBuilder) []string {
	var poolIDs []string

	for _, pool := range readyPools {
		if pool.Definition.Spec.OCloudSiteName == siteCR.Definition.Name {
			poolIDs = append(poolIDs, string(pool.Definition.UID))
		}
	}

	return poolIDs
}

// apiPoolIDs returns string UIDs from an API OCloudSite's ResourcePools.
func apiPoolIDs(apiSite oranapi.OCloudSiteInfo) []string {
	poolIDs := make([]string, 0, len(apiSite.ResourcePools))
	for _, poolID := range apiSite.ResourcePools {
		poolIDs = append(poolIDs, poolID.String())
	}

	return poolIDs
}

// VerifyResourcePoolMatchesCR checks that an API Resource Pool matches the corresponding ResourcePool CR.
func VerifyResourcePoolMatchesCR(apiPool oranapi.ResourcePool, poolCR *oran.ResourcePoolBuilder) error {
	var errs []error

	errs = appendMismatch(errs, "name", poolCR.Definition.Name, apiPool.Name)
	errs = appendMismatch(errs, "description", poolCR.Definition.Spec.Description, apiPool.Description)
	errs = appendMismatch(errs, "oCloudSiteId",
		poolCR.Definition.Status.ResolvedOCloudSiteUID, apiPool.OCloudSiteId.String())

	return errors.Join(errs...)
}
