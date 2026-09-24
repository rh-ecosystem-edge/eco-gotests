//go:build unit_test

package o2imstest

import (
	"testing"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
)

func TestVerifyAPIVersions(t *testing.T) {
	t.Parallel()

	match := func(version, uriPrefix string) oranapi.APIVersions {
		return oranapi.APIVersions{
			ApiVersions: &[]oranapi.APIVersion{{Version: new(version)}},
			UriPrefix:   new(uriPrefix),
		}
	}

	t.Run("valid cluster", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, VerifyAPIVersions(
			match(tsparams.ClusterAPIVersion, tsparams.ClusterAPIURIPrefix),
			tsparams.ClusterAPIVersion,
			tsparams.ClusterAPIURIPrefix,
		))
	})

	t.Run("valid inventory", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, VerifyAPIVersions(
			match(tsparams.InventoryAPIVersion, tsparams.InventoryAPIURIPrefix),
			tsparams.InventoryAPIVersion,
			tsparams.InventoryAPIURIPrefix,
		))
	})

	for _, testCase := range []struct {
		name     string
		versions oranapi.APIVersions
	}{
		{
			name:     "wrong version",
			versions: match(tsparams.InventoryAPIVersion, tsparams.ClusterAPIURIPrefix),
		},
		{
			name:     "wrong uriPrefix",
			versions: match(tsparams.ClusterAPIVersion, "/wrong"),
		},
		{name: "missing fields", versions: oranapi.APIVersions{}},
		{
			name: "empty apiVersions",
			versions: oranapi.APIVersions{
				ApiVersions: &[]oranapi.APIVersion{},
				UriPrefix:   new(tsparams.ClusterAPIURIPrefix),
			},
		},
		{
			name: "nil version",
			versions: oranapi.APIVersions{
				ApiVersions: &[]oranapi.APIVersion{{}},
				UriPrefix:   new(tsparams.ClusterAPIURIPrefix),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, VerifyAPIVersions(testCase.versions, tsparams.ClusterAPIVersion, tsparams.ClusterAPIURIPrefix))
		})
	}
}
