package helper

import (
	"fmt"
	"os"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"gopkg.in/yaml.v3"
)

// clusterInstanceFile is a minimal decode target for site-config ClusterInstance YAML. Only the fields needed to
// derive ProvisioningRequest node hostnames (and optional cluster name) are parsed.
type clusterInstanceFile struct {
	Spec struct {
		ClusterName string `yaml:"clusterName"`
		Nodes       []struct {
			HostName    string         `yaml:"hostName"`
			Role        string         `yaml:"role"`
			NodeNetwork map[string]any `yaml:"nodeNetwork,omitempty"`
		} `yaml:"nodes"`
	} `yaml:"spec"`
}

// SpokeNode is a hostname, optional role, and optional nodeNetwork from a ClusterInstance file or SNO hostname input.
type SpokeNode struct {
	HostName    string
	Role        string
	NodeNetwork map[string]any
}

// loadClusterInstanceFile reads and parses a ClusterInstance YAML from path.
func loadClusterInstanceFile(path string) (*clusterInstanceFile, error) {
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

	return &clusterInstance, nil
}

// LoadSpokeNodesFromClusterInstance reads a ClusterInstance YAML and returns spec.nodes[] with hostname and role.
func LoadSpokeNodesFromClusterInstance(path string) ([]SpokeNode, error) {
	clusterInstance, err := loadClusterInstanceFile(path)
	if err != nil {
		return nil, err
	}

	nodes := make([]SpokeNode, 0, len(clusterInstance.Spec.Nodes))

	for i, node := range clusterInstance.Spec.Nodes {
		if node.HostName == "" {
			return nil, fmt.Errorf("ClusterInstance file %s spec.nodes[%d] has empty hostName", path, i)
		}

		nodes = append(nodes, SpokeNode{
			HostName:    node.HostName,
			Role:        node.Role,
			NodeNetwork: node.NodeNetwork,
		})
	}

	return nodes, nil
}

// LoadSpokeHostnamesFromClusterInstance reads a ClusterInstance YAML and returns spec.nodes[].hostName in file order.
func LoadSpokeHostnamesFromClusterInstance(path string) ([]string, error) {
	nodes, err := LoadSpokeNodesFromClusterInstance(path)
	if err != nil {
		return nil, err
	}

	hostnames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		hostnames = append(hostnames, node.HostName)
	}

	return hostnames, nil
}

// GetClusterInstanceClusterName returns spec.clusterName from the ClusterInstance file when
// ECO_CNF_RAN_CLUSTERINSTANCE_PATH is set and the field is non-empty; otherwise RANConfig.Spoke1Name.
func GetClusterInstanceClusterName() (string, error) {
	if RANConfig.ClusterInstancePath != "" {
		clusterInstance, err := loadClusterInstanceFile(RANConfig.ClusterInstancePath)
		if err != nil {
			return "", err
		}

		if clusterInstance.Spec.ClusterName != "" {
			return clusterInstance.Spec.ClusterName, nil
		}
	}

	if RANConfig.Spoke1Name == "" {
		return "", fmt.Errorf("ECO_CNF_RAN_SPOKE1_NAME must be set when ClusterInstance file omits spec.clusterName")
	}

	return RANConfig.Spoke1Name, nil
}

// GetSpokeNodes returns nodes for ProvisioningRequest generation. When ECO_CNF_RAN_CLUSTERINSTANCE_PATH is set, nodes
// are read from that file; otherwise a single SNO node is built from ECO_CNF_RAN_SPOKE1_HOSTNAME.
func GetSpokeNodes() ([]SpokeNode, error) {
	if RANConfig.ClusterInstancePath != "" {
		return LoadSpokeNodesFromClusterInstance(RANConfig.ClusterInstancePath)
	}

	if RANConfig.Spoke1Hostname == "" {
		return nil, fmt.Errorf("ECO_CNF_RAN_SPOKE1_HOSTNAME must be set when ECO_CNF_RAN_CLUSTERINSTANCE_PATH is unset")
	}

	return []SpokeNode{{
		HostName: RANConfig.Spoke1Hostname,
		Role:     "master",
	}}, nil
}

// GetSpokeHostnames returns the hostnames for the primary ProvisioningRequest nodes[]. When
// ECO_CNF_RAN_CLUSTERINSTANCE_PATH is set, hostnames are read from that ClusterInstance YAML. Otherwise the single
// ECO_CNF_RAN_SPOKE1_HOSTNAME value is used (SNO).
func GetSpokeHostnames() ([]string, error) {
	nodes, err := GetSpokeNodes()
	if err != nil {
		return nil, err
	}

	hostnames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		hostnames = append(hostnames, node.HostName)
	}

	return hostnames, nil
}

// GetClusterTemplateName returns the ClusterTemplate base name for O-RAN ProvisioningRequests.
// Precedence: ECO_CNF_RAN_CLUSTER_TEMPLATE_NAME if set; else mno-ran-du when a ClusterInstance file yields more than
// one hostname; else sno-ran-du. When ECO_CNF_RAN_CLUSTERINSTANCE_PATH is set, a load error is returned instead of
// falling back to sno-ran-du.
func GetClusterTemplateName() (string, error) {
	if RANConfig.ClusterTemplateName != "" {
		return RANConfig.ClusterTemplateName, nil
	}

	if RANConfig.ClusterInstancePath != "" {
		hostnames, err := LoadSpokeHostnamesFromClusterInstance(RANConfig.ClusterInstancePath)
		if err != nil {
			return "", err
		}

		if len(hostnames) > 1 {
			return tsparams.MNOClusterTemplateName, nil
		}

		return tsparams.ClusterTemplateName, nil
	}

	return tsparams.ClusterTemplateName, nil
}

// GetExtraManifestsName returns the expected extra-manifests ConfigMap name for the resolved ClusterTemplate.
func GetExtraManifestsName() (string, error) {
	clusterTemplateName, err := GetClusterTemplateName()
	if err != nil {
		return "", err
	}

	return clusterTemplateName + "-extra-manifest-1", nil
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

// buildClusterInstanceNodeGroups builds clusterInstanceParameters.nodeGroups for MNO templates.
func buildClusterInstanceNodeGroups(nodes []SpokeNode) []map[string]any {
	grouped := map[string][]map[string]any{
		"master": {},
		"worker": {},
	}

	for _, node := range nodes {
		role := node.Role
		if role != "master" && role != "worker" {
			role = "master"
		}

		nodeEntry := map[string]any{
			"hostName": node.HostName,
		}
		if node.NodeNetwork != nil {
			nodeEntry["nodeNetwork"] = node.NodeNetwork
		}

		grouped[role] = append(grouped[role], nodeEntry)
	}

	nodeGroups := make([]map[string]any, 0, 2)
	if len(grouped["master"]) > 0 {
		nodeGroups = append(nodeGroups, map[string]any{
			"name":  "master",
			"role":  "master",
			"nodes": grouped["master"],
		})
	}

	if len(grouped["worker"]) > 0 {
		nodeGroups = append(nodeGroups, map[string]any{
			"name":  "worker",
			"role":  "worker",
			"nodes": grouped["worker"],
		})
	}

	return nodeGroups
}

// buildSecondarySpokeNodes returns spoke nodes with distinct hostnames for a secondary ProvisioningRequest.
func buildSecondarySpokeNodes(spokeNodes []SpokeNode) []SpokeNode {
	if len(spokeNodes) == 0 {
		return []SpokeNode{{HostName: tsparams.TestName2, Role: "master"}}
	}

	secondaryNodes := make([]SpokeNode, 0, len(spokeNodes))
	for i, node := range spokeNodes {
		secondaryNodes = append(secondaryNodes, SpokeNode{
			HostName: fmt.Sprintf("%s-%d", tsparams.TestName2, i),
			Role:     node.Role,
		})
	}

	return secondaryNodes
}
