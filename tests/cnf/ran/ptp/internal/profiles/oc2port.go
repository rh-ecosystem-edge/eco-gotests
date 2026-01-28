package profiles

import (
	"context"
	"fmt"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

// Oc2PortInfo stores derived OC 2-port profile and interface details.
type Oc2PortInfo struct {
	Interfaces       []*InterfaceInfo
	IfaceGroup       iface.NICName
	ActiveInterface  iface.Name
	PassiveInterface iface.Name
}

// Oc2PortDetermineActivePassiveInterfaces queries Prometheus metrics to identify which interface
// in an OC 2-port configuration is currently active (FOLLOWER/SLAVE) and which is passive
// (LISTENING). It returns an error if roles cannot be determined or are unexpected.
func Oc2PortDetermineActivePassiveInterfaces(
	ctx context.Context,
	prometheusAPI prometheusv1.API,
	nodeName string,
	clientInterfaces []*InterfaceInfo,
) (active iface.Name, passive iface.Name, err error) {
	if len(clientInterfaces) != 2 {
		return "", "", fmt.Errorf("expected 2 client interfaces, got %d", len(clientInterfaces))
	}

	interfaceRoles := make(map[iface.Name]metrics.PtpInterfaceRole)

	for _, clientIface := range clientInterfaces {
		roleQuery := metrics.InterfaceRoleQuery{
			Interface: metrics.Equals(clientIface.Name),
			Node:      metrics.Equals(nodeName),
			Process:   metrics.Equals(metrics.ProcessPTP4L),
		}

		result, err := metrics.ExecuteQuery(ctx, prometheusAPI, roleQuery)
		if err != nil {
			return "", "", fmt.Errorf("failed to query role for interface %s on node %s: %w",
				clientIface.Name, nodeName, err)
		}

		switch len(result) {
		case 0:
			return "", "", fmt.Errorf("no metrics found for interface %s on node %s",
				clientIface.Name, nodeName)
		case 1:
		default:
			return "", "", fmt.Errorf("expected 1 metric for interface %s on node %s, got %d",
				clientIface.Name, nodeName, len(result))
		}

		role := metrics.PtpInterfaceRole(result[0].Value)
		interfaceRoles[clientIface.Name] = role
	}

	var activeIface, passiveIface iface.Name

	for _, clientIface := range clientInterfaces {
		role := interfaceRoles[clientIface.Name]

		//nolint:exhaustive // Only two roles are valid for this logic.
		switch role {
		case metrics.InterfaceRoleFollower:
			// Follower = SLAVE = Active
			activeIface = clientIface.Name
		case metrics.InterfaceRoleListening:
			// Listening = Passive
			passiveIface = clientIface.Name
		default:
			return "", "", fmt.Errorf("unexpected role %d for interface %s in OC 2-port profile",
				role, clientIface.Name)
		}
	}

	if activeIface == "" || passiveIface == "" {
		return "", "", fmt.Errorf("could not determine active/passive interfaces. Active: %s, Passive: %s",
			activeIface, passiveIface)
	}

	return activeIface, passiveIface, nil
}
