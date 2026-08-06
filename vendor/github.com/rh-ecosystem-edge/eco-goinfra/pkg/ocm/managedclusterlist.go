package ocm

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/clusterv1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterVendorLabel            = "vendor"
	clusterIDLabel                = "clusterID"
	openshiftVersionLabel         = "openshiftVersion"
	localClusterLabel             = "local-cluster"
	clusterTemplateArtifactsLabel = "clustertemplates.clcm.openshift.io/templateId"
	openshiftVendor               = "OpenShift"
)

// ListManagedClusters returns all ManagedCluster CRs on the hub.
func ListManagedClusters(apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ManagedClusterBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("ManagedClusters 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list managedClusters, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(clusterv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add ManagedCluster scheme to client schemes")

		return nil, err
	}

	klog.V(100).Info("Listing all managedClusters")

	managedClusterList := new(clusterv1.ManagedClusterList)

	err = apiClient.List(logging.DiscardContext(), managedClusterList, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list all managedClusters due to %s", err.Error())

		return nil, err
	}

	var managedClusterObjects []*ManagedClusterBuilder

	for _, managedCluster := range managedClusterList.Items {
		copiedManagedCluster := managedCluster
		managedClusterBuilder := &ManagedClusterBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedManagedCluster,
			Definition: &copiedManagedCluster,
		}

		managedClusterObjects = append(managedClusterObjects, managedClusterBuilder)
	}

	return managedClusterObjects, nil
}

// ListNodeClusterEligibleManagedClusters returns ManagedClusters eligible for the cluster API (NodeCluster): available
// (ManagedClusterConditionAvailable is not nil or False), provisioned for spokes
// (clustertemplates.clcm.openshift.io/templateId label is present; the hub cluster is exempt), and convertible (vendor
// and clusterID labels are present, clusterID is a valid UUID, and openshiftVersion is present when vendor is
// OpenShift).
func ListNodeClusterEligibleManagedClusters(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ManagedClusterBuilder, error) {
	managedClusters, err := ListManagedClusters(apiClient, options...)
	if err != nil {
		return nil, err
	}

	return filterEligibleManagedClusters(managedClusters, IsNodeClusterEligibleManagedCluster), nil
}

// ListDeploymentManagerEligibleManagedClusters returns ManagedClusters eligible for the inventory API
// (DeploymentManager): available (ManagedClusterConditionAvailable is not nil or False), provisioned
// (clustertemplates.clcm.openshift.io/templateId label is present), labeled with clusterID, and exposing a non-empty
// client URL in ManagedClusterClientConfigs.
func ListDeploymentManagerEligibleManagedClusters(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ManagedClusterBuilder, error) {
	managedClusters, err := ListManagedClusters(apiClient, options...)
	if err != nil {
		return nil, err
	}

	return filterEligibleManagedClusters(managedClusters, IsDeploymentManagerEligibleManagedCluster), nil
}

func filterEligibleManagedClusters(
	managedClusters []*ManagedClusterBuilder,
	isEligible func(*clusterv1.ManagedCluster) bool,
) []*ManagedClusterBuilder {
	eligibleManagedClusters := make([]*ManagedClusterBuilder, 0, len(managedClusters))

	for _, managedCluster := range managedClusters {
		if isEligible(managedCluster.Definition) {
			eligibleManagedClusters = append(eligibleManagedClusters, managedCluster)
		}
	}

	return eligibleManagedClusters
}

// IsNodeClusterEligibleManagedCluster reports whether a ManagedCluster is eligible for the cluster API (NodeCluster).
func IsNodeClusterEligibleManagedCluster(cluster *clusterv1.ManagedCluster) bool {
	if cluster == nil {
		return false
	}

	if !isManagedClusterAvailable(cluster) {
		return false
	}

	if !isLocalManagedCluster(cluster) {
		if cluster.Labels == nil {
			return false
		}

		if _, found := cluster.Labels[clusterTemplateArtifactsLabel]; !found {
			return false
		}
	}

	return canConvertManagedClusterToNodeCluster(cluster)
}

// IsDeploymentManagerEligibleManagedCluster reports whether a ManagedCluster is eligible for the inventory API
// (DeploymentManager).
func IsDeploymentManagerEligibleManagedCluster(cluster *clusterv1.ManagedCluster) bool {
	if cluster == nil {
		return false
	}

	if !isManagedClusterAvailable(cluster) {
		return false
	}

	if cluster.Labels == nil {
		return false
	}

	if _, found := cluster.Labels[clusterTemplateArtifactsLabel]; !found {
		return false
	}

	if _, found := cluster.Labels[clusterIDLabel]; !found {
		return false
	}

	return hasManagedClusterClientURL(cluster)
}

// isManagedClusterAvailable reports whether the O2IMS operator would consider a cluster available. The operator filters
// out only nil or False ManagedClusterConditionAvailable; Unknown is treated as eligible.
func isManagedClusterAvailable(cluster *clusterv1.ManagedCluster) bool {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable)

	return condition != nil && condition.Status != metav1.ConditionFalse
}

// hasManagedClusterClientURL reports whether the ManagedCluster has a client URL. This corresponds to the API endpoint
// of the managed cluster.
func hasManagedClusterClientURL(cluster *clusterv1.ManagedCluster) bool {
	for _, config := range cluster.Spec.ManagedClusterClientConfigs {
		if config.URL != "" {
			return true
		}
	}

	return false
}

// isLocalManagedCluster reports whether cluster is the ACM hub cluster (local-cluster=true).
func isLocalManagedCluster(cluster *clusterv1.ManagedCluster) bool {
	if cluster.Labels == nil {
		return false
	}

	value, found := cluster.Labels[localClusterLabel]
	if !found {
		return false
	}

	localCluster, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}

	return localCluster
}

// canConvertManagedClusterToNodeCluster reports whether cluster has the labels required for O2IMS node cluster conversion.
func canConvertManagedClusterToNodeCluster(cluster *clusterv1.ManagedCluster) bool {
	if cluster.Labels == nil {
		return false
	}

	vendor, found := cluster.Labels[clusterVendorLabel]
	if !found {
		return false
	}

	if vendor == openshiftVendor {
		if _, found := cluster.Labels[openshiftVersionLabel]; !found {
			return false
		}
	}

	clusterID, found := cluster.Labels[clusterIDLabel]
	if !found {
		return false
	}

	_, err := uuid.Parse(clusterID)

	return err == nil
}
