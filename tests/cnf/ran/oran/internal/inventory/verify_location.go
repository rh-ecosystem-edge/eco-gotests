package inventory

import (
	"errors"
	"fmt"
	"strconv"

	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyLocationMatchesCR checks that an API Location matches the corresponding Location CR and related OCloudSites.
func VerifyLocationMatchesCR(
	apiLocation oranapi.LocationInfo,
	locationCR *oran.LocationBuilder,
	readySites []*oran.OCloudSiteBuilder,
) error {
	var errs []error

	errs = appendMismatch(errs, "name", locationCR.Definition.Name, apiLocation.Name)
	errs = appendMismatch(errs, "description", locationCR.Definition.Spec.Description, apiLocation.Description)
	errs = appendMismatch(errs, "address", locationCR.Definition.Spec.Address, apiLocation.Address)
	errs = appendError(errs, verifyLocationCoordinate(apiLocation, locationCR.Definition.Spec.Coordinate))
	errs = appendError(errs, verifyLocationCivicAddress(apiLocation, locationCR.Definition.Spec.CivicAddress))
	errs = appendError(errs, verifyStringSetEqual(
		expectedSiteIDsForLocation(locationCR, readySites), apiSiteIDs(apiLocation),
		fmt.Sprintf("oCloudSiteIds for Location %s", locationCR.Definition.Name)))

	return errors.Join(errs...)
}

// expectedSiteIDsForLocation returns UIDs of ready OCloudSites bound to the given Location.
func expectedSiteIDsForLocation(
	locationCR *oran.LocationBuilder, readySites []*oran.OCloudSiteBuilder) []string {
	var siteIDs []string

	for _, site := range readySites {
		if site.Definition.Spec.GlobalLocationName == locationCR.Definition.Name {
			siteIDs = append(siteIDs, string(site.Definition.UID))
		}
	}

	return siteIDs
}

// apiSiteIDs returns string UIDs from an API Location's OCloudSiteIds.
func apiSiteIDs(apiLocation oranapi.LocationInfo) []string {
	siteIDs := make([]string, 0, len(apiLocation.OCloudSiteIds))
	for _, siteID := range apiLocation.OCloudSiteIds {
		siteIDs = append(siteIDs, siteID.String())
	}

	return siteIDs
}

// verifyLocationCoordinate checks that apiLocation.Coordinate matches the CR coordinate.
func verifyLocationCoordinate(apiLocation oranapi.LocationInfo, coordinate *inventoryv1alpha1.GeoLocation) error {
	if coordinate == nil {
		if apiLocation.Coordinate != nil {
			return fmt.Errorf("coordinate: want nil, got %#v", apiLocation.Coordinate)
		}

		return nil
	}

	var errs []error

	if apiLocation.Coordinate == nil {
		errs = append(errs, fmt.Errorf("coordinate: want non-nil, got nil"))
	} else {
		errs = appendMismatch(errs, "coordinate.type", "Point", string(apiLocation.Coordinate.Type))

		lat, latErr := strconv.ParseFloat(coordinate.Latitude, 64)
		if latErr != nil {
			errs = append(errs, fmt.Errorf("coordinate.latitude: invalid value %q: %w", coordinate.Latitude, latErr))
		}

		lon, lonErr := strconv.ParseFloat(coordinate.Longitude, 64)
		if lonErr != nil {
			errs = append(errs, fmt.Errorf("coordinate.longitude: invalid value %q: %w", coordinate.Longitude, lonErr))
		}

		coords := apiLocation.Coordinate.Coordinates
		if len(coords) != 2 && len(coords) != 3 {
			errs = append(errs, fmt.Errorf("coordinate.coordinates: want length 2 or 3, got %d", len(coords)))
		} else if latErr == nil && lonErr == nil {
			want := []float64{lon, lat}
			if !areCoordinatesEqual(want, coords[:2]) {
				errs = append(errs, fmt.Errorf("coordinate.coordinates: want ~%v, got %v", want, coords[:2]))
			}
		}
	}

	return errors.Join(errs...)
}

// verifyLocationCivicAddress checks that apiLocation.CivicAddress matches the CR civic address.
func verifyLocationCivicAddress(
	apiLocation oranapi.LocationInfo, civicAddress []inventoryv1alpha1.CivicAddressElement) error {
	apiCivicAddress := derefSlice(apiLocation.CivicAddress)

	if len(civicAddress) == 0 {
		if len(apiCivicAddress) > 0 {
			return fmt.Errorf("civicAddress: want empty, got %#v", apiCivicAddress)
		}

		return nil
	}

	var errs []error

	if len(apiCivicAddress) != len(civicAddress) {
		errs = append(errs, fmt.Errorf("civicAddress: want length %d, got %d", len(civicAddress), len(apiCivicAddress)))
	} else {
		for i, element := range civicAddress {
			errs = appendMismatch(errs, fmt.Sprintf("civicAddress[%d].caType", i), element.CaType, apiCivicAddress[i].CaType)
			errs = appendMismatch(errs, fmt.Sprintf("civicAddress[%d].caValue", i), element.CaValue, apiCivicAddress[i].CaValue)
		}
	}

	return errors.Join(errs...)
}
