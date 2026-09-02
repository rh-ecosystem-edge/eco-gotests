package inventory

import (
	"errors"
	"fmt"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyAPIVersions checks that the inventory API version response matches expected values.
func VerifyAPIVersions(versions oranapi.APIVersions) error {
	apiVersions := derefSlice(versions.ApiVersions)

	var firstVersion *string
	if len(apiVersions) > 0 {
		firstVersion = apiVersions[0].Version
	}

	return errors.Join(
		verifyAPIVersion(firstVersion),
		verifyAPIURIPrefix(versions.UriPrefix),
	)
}

// verifyAPIVersion checks that the first API version matches the expected value.
func verifyAPIVersion(version *string) error {
	if version == nil {
		return fmt.Errorf("apiVersions[0].version: want non-nil, got nil")
	}

	if *version != "2.0.0" {
		return fmt.Errorf("apiVersions[0].version: want %q, got %q", "2.0.0", *version)
	}

	return nil
}

// verifyAPIURIPrefix checks that the API URI prefix matches the expected value.
func verifyAPIURIPrefix(uriPrefix *string) error {
	if uriPrefix == nil {
		return fmt.Errorf("uriPrefix: want non-nil, got nil")
	}

	if *uriPrefix != "/o2ims-infrastructureInventory/v2" {
		return fmt.Errorf("uriPrefix: want %q, got %q", "/o2ims-infrastructureInventory/v2", *uriPrefix)
	}

	return nil
}
