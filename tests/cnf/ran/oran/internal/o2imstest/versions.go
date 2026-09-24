package o2imstest

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyAPIVersions checks that versions matches the expected first API version and URI prefix.
func VerifyAPIVersions(versions oranapi.APIVersions, expectedVersion, expectedURIPrefix string) error {
	type view struct {
		Version   string
		URIPrefix string
	}

	if versions.ApiVersions == nil || len(*versions.ApiVersions) == 0 || (*versions.ApiVersions)[0].Version == nil {
		return fmt.Errorf("apiVersions[0].version: want non-nil, got nil")
	}

	if versions.UriPrefix == nil {
		return fmt.Errorf("uriPrefix: want non-nil, got nil")
	}

	got := view{
		Version:   *(*versions.ApiVersions)[0].Version,
		URIPrefix: *versions.UriPrefix,
	}
	want := view{Version: expectedVersion, URIPrefix: expectedURIPrefix}

	if diff := cmp.Diff(want, got); diff != "" {
		return fmt.Errorf("API versions mismatch (-want +got):\n%s", diff)
	}

	return nil
}
