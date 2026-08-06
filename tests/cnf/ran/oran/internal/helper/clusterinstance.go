package helper

import (
	"fmt"
	"os"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"sigs.k8s.io/yaml"
)

// clusterInstanceFile is a minimal decode target for site-config ClusterInstance YAML. Only the fields needed to
// derive ProvisioningRequest node hostnames (and optional cluster name) are parsed.
type clusterInstanceFile struct {
	Spec struct {
		ClusterName string `json:"clusterName"`
		Nodes       []struct {
			HostName string `json:"hostName"`
		} `json:"nodes"`
	} `json:"spec"`
}

// LoadSpokeHostnamesFromClusterInstance reads a ClusterInstance YAML and returns spec.nodes[].hostName in file order.
func LoadSpokeHostnamesFromClusterInstance(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ClusterInstance file %s: %w", path, err)
	}

	var clusterInstance clusterInstanceFile

	if err := yaml.Unmarshal(data, &clusterInstance); err != nil {
		return nil, fmt.Errorf("parse ClusterInstance file %s: %w", path, err)
	}

	if len(clusterInstance.Spec.Nodes) == 0 {
		return nil, fmt.Errorf("ClusterInstance file %s has no spec.nodes entries", path)
	}

	hostnames := make([]string, 0, len(clusterInstance.Spec.Nodes))

	for i, node := range clusterInstance.Spec.Nodes {
		if node.HostName == "" {
			return nil, fmt.Errorf("ClusterInstance file %s spec.nodes[%d] has empty hostName", path, i)
		}

		hostnames = append(hostnames, node.HostName)
	}

	return hostnames, nil
}

// GetSpokeHostnames returns the hostnames for the primary ProvisioningRequest nodes[]. When
// ECO_CNF_RAN_CLUSTERINSTANCE_PATH is set, hostnames are read from that ClusterInstance YAML. Otherwise the single
// ECO_CNF_RAN_SPOKE1_HOSTNAME value is used (SNO).
func GetSpokeHostnames() ([]string, error) {
	if RANConfig.ClusterInstancePath != "" {
		return LoadSpokeHostnamesFromClusterInstance(RANConfig.ClusterInstancePath)
	}

	if RANConfig.Spoke1Hostname == "" {
		return nil, fmt.Errorf("ECO_CNF_RAN_SPOKE1_HOSTNAME must be set when ECO_CNF_RAN_CLUSTERINSTANCE_PATH is unset")
	}

	return []string{RANConfig.Spoke1Hostname}, nil
}

// GetClusterTemplateName returns the ClusterTemplate base name for O-RAN ProvisioningRequests.
// Precedence: ECO_CNF_RAN_CLUSTER_TEMPLATE_NAME if set; else mno-ran-du when a ClusterInstance file yields more than
// one hostname; else sno-ran-du.
func GetClusterTemplateName() string {
	if RANConfig.ClusterTemplateName != "" {
		return RANConfig.ClusterTemplateName
	}

	if RANConfig.ClusterInstancePath != "" {
		hostnames, err := LoadSpokeHostnamesFromClusterInstance(RANConfig.ClusterInstancePath)
		if err == nil && len(hostnames) > 1 {
			return tsparams.MNOClusterTemplateName
		}
	}

	return tsparams.ClusterTemplateName
}

// GetExtraManifestsName returns the expected extra-manifests ConfigMap name for the resolved ClusterTemplate.
func GetExtraManifestsName() string {
	return GetClusterTemplateName() + "-extra-manifest-1"
}

// buildClusterInstanceNodes builds the clusterInstanceParameters.nodes slice for a ProvisioningRequest.
func buildClusterInstanceNodes(hostnames []string) []map[string]any {
	nodes := make([]map[string]any, 0, len(hostnames))
	for _, hostname := range hostnames {
		nodes = append(nodes, map[string]any{
			"hostName": hostname,
		})
	}

	return nodes
}
