package o2imscluster

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
)

// NodeClusterTypeKey identifies a distinct NodeClusterType derived from eligible ManagedClusters.
type NodeClusterTypeKey struct {
	Model   string
	Vendor  string
	Version string
}

// Name returns the NodeClusterType name pattern {model}-{vendor}-{version}.
func (key NodeClusterTypeKey) Name() string {
	return fmt.Sprintf("%s-%s-%s", key.Model, key.Vendor, key.Version)
}

// ClusterResourceTypeKey identifies a distinct ClusterResourceType derived from eligible Agents.
type ClusterResourceTypeKey struct {
	Architecture string
	Cores        int64
}

// Name returns the ClusterResourceType name pattern "{ARCH} CPU with {N} Cores".
func (key ClusterResourceTypeKey) Name() string {
	return fmt.Sprintf("%s CPU with %d Cores", strings.ToUpper(key.Architecture), key.Cores)
}

// ManagedClusterModel returns hub-cluster or managed-cluster for the given ManagedCluster.
func ManagedClusterModel(cluster *ocm.ManagedClusterBuilder) string {
	localCluster := false

	if cluster != nil && cluster.Definition != nil && cluster.Definition.Labels != nil {
		if value, found := cluster.Definition.Labels[tsparams.LocalClusterLabel]; found {
			localCluster, _ = strconv.ParseBool(value)
		}
	}

	if localCluster {
		return tsparams.ClusterModelHubCluster
	}

	return tsparams.ClusterModelManagedCluster
}

// NodeClusterTypeKeyFromManagedCluster builds the type key from an eligible ManagedCluster's labels.
func NodeClusterTypeKeyFromManagedCluster(cluster *ocm.ManagedClusterBuilder) (NodeClusterTypeKey, error) {
	if cluster == nil || cluster.Definition == nil || cluster.Definition.Labels == nil {
		return NodeClusterTypeKey{}, fmt.Errorf("managed cluster or labels are nil")
	}

	vendor, found := cluster.Definition.Labels[tsparams.ClusterVendorLabel]
	if !found {
		return NodeClusterTypeKey{}, fmt.Errorf("managed cluster %s missing %s label",
			cluster.Definition.Name, tsparams.ClusterVendorLabel)
	}

	version := cluster.Definition.Labels[tsparams.OpenshiftVersionLabel]

	return NodeClusterTypeKey{
		Model:   ManagedClusterModel(cluster),
		Vendor:  vendor,
		Version: version,
	}, nil
}

// CollectNodeClusterTypeKeys returns distinct NodeClusterType keys from eligible ManagedClusters.
func CollectNodeClusterTypeKeys(hubClient *clients.Settings) (map[NodeClusterTypeKey]struct{}, error) {
	eligible, err := ocm.ListNodeClusterEligibleManagedClusters(hubClient)
	if err != nil {
		return nil, fmt.Errorf("failed to list node-cluster-eligible ManagedClusters: %w", err)
	}

	keys := make(map[NodeClusterTypeKey]struct{}, len(eligible))

	for _, cluster := range eligible {
		key, keyErr := NodeClusterTypeKeyFromManagedCluster(cluster)
		if keyErr != nil {
			return nil, keyErr
		}

		keys[key] = struct{}{}
	}

	return keys, nil
}

// ManagedClusterID returns the clusterID label UUID for a ManagedCluster.
func ManagedClusterID(cluster *ocm.ManagedClusterBuilder) (uuid.UUID, error) {
	if cluster == nil || cluster.Definition == nil || cluster.Definition.Labels == nil {
		return uuid.Nil, fmt.Errorf("managed cluster or labels are nil")
	}

	clusterID, found := cluster.Definition.Labels[tsparams.ClusterIDLabel]
	if !found {
		return uuid.Nil, fmt.Errorf("managed cluster %s missing %s label",
			cluster.Definition.Name, tsparams.ClusterIDLabel)
	}

	parsed, err := uuid.Parse(clusterID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("managed cluster %s has invalid %s label %q: %w",
			cluster.Definition.Name, tsparams.ClusterIDLabel, clusterID, err)
	}

	return parsed, nil
}

// CollectClusterResourceTypeKeys returns distinct ClusterResourceType keys from ORAN-eligible Agents.
func CollectClusterResourceTypeKeys(hubClient *clients.Settings) (map[ClusterResourceTypeKey]struct{}, error) {
	agents, err := assisted.ListORANEligibleAgents(hubClient)
	if err != nil {
		return nil, fmt.Errorf("failed to list ORAN-eligible Agents: %w", err)
	}

	keys := make(map[ClusterResourceTypeKey]struct{}, len(agents))

	for _, agent := range agents {
		key := ClusterResourceTypeKeyFromAgent(agent.Definition)
		keys[key] = struct{}{}
	}

	return keys, nil
}

// ClusterResourceTypeKeyFromAgent builds the type key from an Agent's inventory CPU fields.
func ClusterResourceTypeKeyFromAgent(agent *agentInstallV1Beta1.Agent) ClusterResourceTypeKey {
	return ClusterResourceTypeKey{
		Architecture: agent.Status.Inventory.Cpu.Architecture,
		Cores:        agent.Status.Inventory.Cpu.Count,
	}
}

// ExpectedAgentExternalID returns the namespace/name identity the collector uses as ExternalID.
// The public Cluster API does not expose ExternalID; this is used for stable error messages and matching.
func ExpectedAgentExternalID(agent *agentInstallV1Beta1.Agent) string {
	return fmt.Sprintf("%s/%s", agent.Namespace, agent.Name)
}

// ExpectedInventoryResourceID returns the ClusterResource.resourceId derived from the Agent hwMgrNodeId label.
func ExpectedInventoryResourceID(agent *agentInstallV1Beta1.Agent) (uuid.UUID, error) {
	if agent == nil {
		return uuid.Nil, fmt.Errorf("agent is nil")
	}

	if agent.Labels == nil || agent.Labels[tsparams.HardwareManagerNodeIDLabel] == "" {
		return uuid.Nil, fmt.Errorf("agent %s/%s is missing %s label",
			agent.Namespace, agent.Name, tsparams.HardwareManagerNodeIDLabel)
	}

	hwMgrNodeID := agent.Labels[tsparams.HardwareManagerNodeIDLabel]

	parsed, err := uuid.Parse(hwMgrNodeID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("agent %s/%s has invalid %s label %q: %w",
			agent.Namespace, agent.Name, tsparams.HardwareManagerNodeIDLabel, hwMgrNodeID, err)
	}

	return parsed, nil
}

// FindClusterResourceForAgent returns the ClusterResource corresponding to agent's hwMgrNodeId label.
func FindClusterResourceForAgent(
	resources []oranapi.ClusterResource,
	agent *agentInstallV1Beta1.Agent,
) (oranapi.ClusterResource, error) {
	expectedResourceID, err := ExpectedInventoryResourceID(agent)
	if err != nil {
		return oranapi.ClusterResource{}, err
	}

	for _, resource := range resources {
		if resource.ResourceId == expectedResourceID {
			return resource, nil
		}
	}

	return oranapi.ClusterResource{}, fmt.Errorf(
		"no ClusterResource matched Agent %s (resourceId %s)",
		ExpectedAgentExternalID(agent), expectedResourceID)
}
