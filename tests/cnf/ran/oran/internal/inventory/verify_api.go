package inventory

import (
	"errors"
	"fmt"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// VerifyAPIVersions checks that the inventory API version response matches expected values.
func VerifyAPIVersions(versions oranapi.APIVersions) error {
	var errs []error

	apiVersions := derefSlice(versions.ApiVersions)

	var firstVersion *string
	if len(apiVersions) > 0 {
		firstVersion = apiVersions[0].Version
	}

	errs = appendError(errs, verifyAPIVersion(firstVersion))
	errs = appendError(errs, verifyAPIURIPrefix(versions.UriPrefix))

	return errors.Join(errs...)
}

// verifyAPIVersion checks that the first API version matches the expected value.
func verifyAPIVersion(version *string) error {
	if version == nil {
		return fmt.Errorf("apiVersions[0].version: want non-nil, got nil")
	}

	var errs []error

	errs = appendMismatch(errs, "apiVersions[0].version", "2.0.0", *version)

	return errors.Join(errs...)
}

// verifyAPIURIPrefix checks that the API URI prefix matches the expected value.
func verifyAPIURIPrefix(uriPrefix *string) error {
	if uriPrefix == nil {
		return fmt.Errorf("uriPrefix: want non-nil, got nil")
	}

	var errs []error

	errs = appendMismatch(errs, "uriPrefix", "/o2ims-infrastructureInventory/v2", *uriPrefix)

	return errors.Join(errs...)
}
