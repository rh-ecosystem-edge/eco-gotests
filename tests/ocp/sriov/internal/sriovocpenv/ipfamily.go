package sriovocpenv

import (
	"encoding/json"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"k8s.io/klog/v2"
)

const ovnExternalAddresses = "k8s.ovn.org/node-primary-ifaddr"

// GetClusterIPFamily detects the cluster's IP family by checking node external OVN addresses.
// Returns tsparams.IPV4Family, tsparams.IPV6Family, or tsparams.DualIPFamily.
func GetClusterIPFamily(apiClient *clients.Settings) (string, error) {
	klog.V(90).Infof("Detecting cluster IP family from OVN external addresses")

	nodesList, err := nodes.List(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodesList) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	// OVN's node-primary-ifaddr is expected to be consistent cluster-wide; use the first listed node only.
	node := nodesList[0]

	raw := node.Object.Annotations[ovnExternalAddresses]
	if raw == "" {
		return "", fmt.Errorf("node %q missing %q annotation", node.Object.Name, ovnExternalAddresses)
	}

	var extNetwork nodes.ExternalNetworks

	err = json.Unmarshal([]byte(raw), &extNetwork)
	if err != nil {
		return "", fmt.Errorf("failed to parse external network annotation on node %s: %w",
			node.Object.Name, err)
	}

	var clusterIPFamily string

	switch {
	case extNetwork.IPv4 != "" && extNetwork.IPv6 != "":
		clusterIPFamily = tsparams.DualIPFamily
	case extNetwork.IPv4 != "":
		clusterIPFamily = tsparams.IPV4Family
	case extNetwork.IPv6 != "":
		clusterIPFamily = tsparams.IPV6Family
	default:
		return "", fmt.Errorf("no IPv4 or IPv6 external address found on node %s", node.Object.Name)
	}

	klog.V(90).Infof("Cluster IP family: %s", clusterIPFamily)

	return clusterIPFamily, nil
}

// ClusterSupportsIPv4 returns true if the cluster supports IPv4 (single-stack or dual-stack).
func ClusterSupportsIPv4(ipFamily string) bool {
	return ipFamily == tsparams.IPV4Family || ipFamily == tsparams.DualIPFamily
}
