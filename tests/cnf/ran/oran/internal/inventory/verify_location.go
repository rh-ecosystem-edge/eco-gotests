package inventory

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/google/go-cmp/cmp"
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
	type view struct {
		Name        string
		Description string
		Address     *string
	}

	var comparisonErr error
	if diff := cmp.Diff(
		view{
			Name:        locationCR.Definition.Name,
			Description: locationCR.Definition.Spec.Description,
			Address:     locationCR.Definition.Spec.Address,
		},
		view{Name: apiLocation.Name, Description: apiLocation.Description, Address: apiLocation.Address},
	); diff != "" {
		comparisonErr = fmt.Errorf("location mismatch (-want +got):\n%s", diff)
	}

	return errors.Join(
		comparisonErr,
		verifyLocationCoordinate(apiLocation, locationCR.Definition.Spec.Coordinate),
		verifyLocationCivicAddress(apiLocation, locationCR.Definition.Spec.CivicAddress),
		verifyStringSetEqual(
			collectExpectedSiteIDsForLocation(locationCR, readySites), extractAPISiteIDs(apiLocation),
			fmt.Sprintf("oCloudSiteIds for Location %s", locationCR.Definition.Name)),
	)
}

// collectExpectedSiteIDsForLocation returns UIDs of ready OCloudSites bound to the given Location.
func collectExpectedSiteIDsForLocation(
	locationCR *oran.LocationBuilder, readySites []*oran.OCloudSiteBuilder) []string {
	var siteIDs []string

	for _, site := range readySites {
		if site.Definition.Spec.GlobalLocationName == locationCR.Definition.Name {
			siteIDs = append(siteIDs, string(site.Definition.UID))
		}
	}

	return siteIDs
}

// extractAPISiteIDs returns string UIDs from an API Location's OCloudSiteIds.
func extractAPISiteIDs(apiLocation oranapi.LocationInfo) []string {
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

	if apiLocation.Coordinate == nil {
		return fmt.Errorf("coordinate: want non-nil, got nil")
	}

	return verifyGeoJSONPoint(apiLocation, coordinate)
}

// parseCRGeoLocation parses latitude and longitude strings from a Location CR GeoLocation.
func parseCRGeoLocation(coordinate *inventoryv1alpha1.GeoLocation) (float64, float64, []error) {
	latitude, latitudeErr := strconv.ParseFloat(coordinate.Latitude, 64)
	longitude, longitudeErr := strconv.ParseFloat(coordinate.Longitude, 64)

	var parseErrors []error

	if latitudeErr != nil {
		parseErrors = append(parseErrors, fmt.Errorf(
			"coordinate.latitude: invalid value %q: %w", coordinate.Latitude, latitudeErr))
	}

	if longitudeErr != nil {
		parseErrors = append(parseErrors, fmt.Errorf(
			"coordinate.longitude: invalid value %q: %w", coordinate.Longitude, longitudeErr))
	}

	return longitude, latitude, parseErrors
}

// verifyGeoJSONPoint checks that apiLocation's GeoJSON Point matches the CR coordinate.
func verifyGeoJSONPoint(apiLocation oranapi.LocationInfo, crCoordinate *inventoryv1alpha1.GeoLocation) error {
	apiCoordinate := apiLocation.Coordinate

	var errs []error

	if coordinateType := string(apiCoordinate.Type); coordinateType != "Point" {
		errs = append(errs, fmt.Errorf("coordinate.type: want %q, got %q", "Point", coordinateType))
	}

	longitude, latitude, parseErrors := parseCRGeoLocation(crCoordinate)
	errs = append(errs, parseErrors...)

	coords := apiCoordinate.Coordinates
	if len(coords) != 2 && len(coords) != 3 {
		errs = append(errs, fmt.Errorf("coordinate.coordinates: want length 2 or 3, got %d", len(coords)))

		return errors.Join(errs...)
	}

	if len(parseErrors) == 0 {
		// GeoJSON Point: [longitude, latitude], with optional elevation as a third element.
		want := []float64{longitude, latitude}
		if !areCoordinatesEqual(want, coords[:2]) {
			errs = append(errs, fmt.Errorf("coordinate.coordinates: want ~%v, got %v", want, coords[:2]))
		}
	}

	return errors.Join(errs...)
}

// verifyLocationCivicAddress checks that apiLocation.CivicAddress matches the CR civic address.
func verifyLocationCivicAddress(
	apiLocation oranapi.LocationInfo, civicAddress []inventoryv1alpha1.CivicAddressElement) error {
	apiCivicAddress := derefSlice(apiLocation.CivicAddress)

	if len(civicAddress) == 0 && len(apiCivicAddress) > 0 {
		return fmt.Errorf("civicAddress: want empty, got %#v", apiCivicAddress)
	}

	if len(apiCivicAddress) != len(civicAddress) {
		return fmt.Errorf("civicAddress: want length %d, got %d", len(civicAddress), len(apiCivicAddress))
	}

	type view struct {
		CaType  int
		CaValue string
	}

	want := make([]view, 0, len(civicAddress))
	got := make([]view, 0, len(apiCivicAddress))

	for i, element := range civicAddress {
		want = append(want, view{CaType: element.CaType, CaValue: element.CaValue})
		got = append(got, view{CaType: apiCivicAddress[i].CaType, CaValue: apiCivicAddress[i].CaValue})
	}

	if diff := cmp.Diff(want, got); diff != "" {
		return fmt.Errorf("civic address mismatch (-want +got):\n%s", diff)
	}

	return nil
}
