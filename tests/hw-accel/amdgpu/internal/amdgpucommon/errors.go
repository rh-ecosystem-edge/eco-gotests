package amdgpucommon

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
)

// IsCRDNotAvailable checks if the error indicates that a CRD is not available.
func IsCRDNotAvailable(err error) bool {
	if err == nil {
		return false
	}

	var noResourceMatchErr *apimeta.NoResourceMatchError
	resourceMatched := errors.As(err, &noResourceMatchErr)

	return apierrors.IsNotFound(err) ||
		apimeta.IsNoMatchError(err) || resourceMatched
}
