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
	IfaceGroup       iface.Alias
	ActiveInterface  iface.Iface
	PassiveInterface iface.Iface
}

// DetermineActivePassiveInterfaces queries Prometheus metrics to identify which interface
// in a multi-port configuration (like OC 2-port or Dual T-BC) is currently active (FOLLOWER/SLAVE)
// and which is passive (LISTENING/PASSIVE). It returns an error if roles cannot be determined or are unexpected.
func DetermineActivePassiveInterfaces(
	ctx context.Context,
	prometheusAPI prometheusv1.API,
	nodeName string,
	clientInterfaces []*InterfaceInfo,
) (active iface.Iface, passive iface.Iface, err error) {
	if len(clientInterfaces) != 2 {
		return "", "", fmt.Errorf("expected 2 client interfaces, got %d", len(clientInterfaces))
	}

	interfaceRoles := make(map[iface.Iface]metrics.PtpInterfaceRole)

	for _, clientIface := range clientInterfaces {
		roleQuery := metrics.InterfaceRoleQuery{
			Interface: metrics.Equals(clientIface.Iface),
			Node:      metrics.Equals(nodeName),
			Process:   metrics.Equals(metrics.ProcessPTP4L),
		}

		result, err := metrics.ExecuteQuery(ctx, prometheusAPI, roleQuery)
		if err != nil {
			return "", "", fmt.Errorf("failed to query role for interface %s on node %s: %w",
				clientIface.Iface, nodeName, err)
		}

		switch len(result) {
		case 0:
			return "", "", fmt.Errorf("no metrics found for interface %s on node %s",
				clientIface.Iface, nodeName)
		case 1:
		default:
			return "", "", fmt.Errorf("expected 1 metric for interface %s on node %s, got %d",
				clientIface.Iface, nodeName, len(result))
		}

		role := metrics.PtpInterfaceRole(result[0].Value)
		interfaceRoles[clientIface.Iface] = role
	}

	var activeIface, passiveIface iface.Iface

	for _, clientIface := range clientInterfaces {
		role := interfaceRoles[clientIface.Iface]

		//nolint:exhaustive // Only two roles are valid for this logic.
		switch role {
		case metrics.InterfaceRoleFollower:
			activeIface = clientIface.Iface
		case metrics.InterfaceRoleListening, metrics.InterfaceRolePassive:
			passiveIface = clientIface.Iface
		default:
			return "", "", fmt.Errorf("unexpected role %d for interface %s",
				role, clientIface.Iface)
		}
	}

	if activeIface == "" || passiveIface == "" {
		return "", "", fmt.Errorf("could not determine active/passive interfaces. Active: %s, Passive: %s",
			activeIface, passiveIface)
	}

	return activeIface, passiveIface, nil
}
