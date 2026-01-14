package profiles

import (
	"context"
	"fmt"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

// GetHAProfiles returns a list of HA profile names matching the specified status for a given node.
// Use metrics.HAProfileStatusActive or metrics.HAProfileStatusInactive as the status parameter.
func GetHAProfiles(
	ctx context.Context,
	prometheusAPI prometheusv1.API,
	nodeName string,
	status metrics.PtpHAProfileStatus,
) ([]string, error) {
	query := metrics.HAProfileStatusQuery{
		Node: metrics.Equals(nodeName),
	}

	result, err := metrics.ExecuteQuery(ctx, prometheusAPI, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HA profile status query for node %s: %w", nodeName, err)
	}

	var profiles []string

	for _, sample := range result {
		// Filter by the requested status value
		if metrics.PtpHAProfileStatus(sample.Value) != status {
			continue
		}

		profileName := string(sample.Metric["profile"])
		if profileName == "" {
			return nil, fmt.Errorf("HA profile metric missing required 'profile' label on node %s: %v",
				nodeName, sample.Metric)
		}

		profiles = append(profiles, profileName)
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("no HA profiles with status %d found for node %s", status, nodeName)
	}

	return profiles, nil
}
