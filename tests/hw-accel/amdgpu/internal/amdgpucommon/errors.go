package amdgpucommon

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
)

// IsCRDNotAvailable checks if the error indicates that a CRD is not available.
func IsCRDNotAvailable(err error) bool {
	return errors.IsNotFound(err) ||
		strings.Contains(err.Error(), "no matches for kind") ||
		strings.Contains(err.Error(), "resource mapping not found")
}
