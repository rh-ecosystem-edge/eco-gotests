package safeapirequest

import (
	"strings"
)

const (
	retries = 5
)

// Request is a user provided function that performs an API request and returns an error
// Existing functions can be adapted to meet the signature requirement of the Requst func type.
type Request func() error

// Do will perform the user supplied function up to the value of retries before giving up
// Explicit error messages are allowed to make testing more fault tolerant in unstable environments.
func Do(req Request) error {
	var err error
	for attempt := 0; attempt < retries; attempt++ {
		err := req()
		if err == nil {
			return nil
		}

		errorStatus := err.Error()

		switch {
		case strings.Contains(errorStatus, "TLS handshake timeout"):
			continue
		case strings.Contains(errorStatus, "connection reset by peer"):
			continue
		case strings.Contains(errorStatus, "did you specify the right host or port?"):
			continue
		case strings.Contains(errorStatus, "connection refused"):
			continue
		case strings.Contains(errorStatus,
			"Operation cannot be fulfilled on imagebasedupgrades.lca.openshift.io \"upgrade\": "+
				"the object has been modified; please apply your changes to the latest version and try again"):
			continue
		default:
			return err
		}
	}

	return err
}
