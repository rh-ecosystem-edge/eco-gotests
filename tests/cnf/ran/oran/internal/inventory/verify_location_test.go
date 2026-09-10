//go:build unit_test

package inventory

import (
	"encoding/json"
	"testing"

	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustUnmarshalLocationInfo decodes a generated API location fixture.
func mustUnmarshalLocationInfo(t *testing.T, raw string) oranapi.LocationInfo {
	t.Helper()

	// LocationInfo.Coordinate is an anonymous generated type, so JSON is the only practical way to construct these
	// focused fixtures without duplicating the generated type definition.
	var location oranapi.LocationInfo
	require.NoError(t, json.Unmarshal([]byte(raw), &location))

	return location
}

// crGeoLocation creates a GeoLocation fixture from latitude and longitude strings.
func crGeoLocation(latitude, longitude string) *inventoryv1alpha1.GeoLocation {
	return &inventoryv1alpha1.GeoLocation{
		Latitude:  latitude,
		Longitude: longitude,
	}
}

//nolint:funlen // The table keeps all coordinate validation cases together.
func TestVerifyLocationCoordinate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apiLocation string
		crLocation  *inventoryv1alpha1.GeoLocation
		wantErr     bool
		errContains []string
	}{
		{
			name:        "both nil",
			apiLocation: `{}`,
		},
		{
			name: "cr nil api has coordinate",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610]
				}
			}`,
			wantErr:     true,
			errContains: []string{"coordinate: want nil"},
		},
		{
			name:        "cr has coordinate api nil",
			apiLocation: `{}`,
			crLocation:  crGeoLocation("40.730610", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate: want non-nil, got nil"},
		},
		{
			name: "matching point with two coordinates",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610]
				}
			}`,
			crLocation: crGeoLocation("40.730610", "-73.935242"),
		},
		{
			name: "matching point with three coordinates",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610, 12.5]
				}
			}`,
			crLocation: crGeoLocation("40.730610", "-73.935242"),
		},
		{
			name: "matching coordinates within epsilon",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730615]
				}
			}`,
			crLocation: crGeoLocation("40.730610", "-73.935242"),
		},
		{
			name: "wrong geometry type",
			apiLocation: `{
				"coordinate": {
					"type": "Polygon",
					"coordinates": [-73.935242, 40.730610]
				}
			}`,
			crLocation:  crGeoLocation("40.730610", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate.type: want \"Point\", got \"Polygon\""},
		},
		{
			name: "invalid latitude",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610]
				}
			}`,
			crLocation:  crGeoLocation("not-a-number", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate.latitude: invalid value"},
		},
		{
			name: "invalid longitude",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610]
				}
			}`,
			crLocation:  crGeoLocation("40.730610", "not-a-number"),
			wantErr:     true,
			errContains: []string{"coordinate.longitude: invalid value"},
		},
		{
			name: "coordinates length one",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242]
				}
			}`,
			crLocation:  crGeoLocation("40.730610", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate.coordinates: want length 2 or 3, got 1"},
		},
		{
			name: "coordinates length four",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 40.730610, 12.5, 99.9]
				}
			}`,
			crLocation:  crGeoLocation("40.730610", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate.coordinates: want length 2 or 3, got 4"},
		},
		{
			name: "coordinate values mismatch",
			apiLocation: `{
				"coordinate": {
					"type": "Point",
					"coordinates": [-73.935242, 41.0]
				}
			}`,
			crLocation:  crGeoLocation("40.730610", "-73.935242"),
			wantErr:     true,
			errContains: []string{"coordinate.coordinates: want"},
		},
		{
			name: "multiple validation errors",
			apiLocation: `{
				"coordinate": {
					"type": "Polygon",
					"coordinates": [-73.935242]
				}
			}`,
			crLocation: crGeoLocation("not-a-number", "-73.935242"),
			wantErr:    true,
			errContains: []string{
				"coordinate.type: want \"Point\", got \"Polygon\"",
				"coordinate.latitude: invalid value",
				"coordinate.coordinates: want length 2 or 3, got 1",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			apiLocation := mustUnmarshalLocationInfo(t, testCase.apiLocation)
			err := verifyLocationCoordinate(apiLocation, testCase.crLocation)

			if testCase.wantErr {
				require.Error(t, err)

				for _, fragment := range testCase.errContains {
					assert.ErrorContains(t, err, fragment)
				}

				return
			}

			assert.NoError(t, err)
		})
	}
}
