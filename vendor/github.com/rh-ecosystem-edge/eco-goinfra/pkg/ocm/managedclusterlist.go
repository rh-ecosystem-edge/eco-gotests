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

// ListORANEligibleManagedClusters returns ManagedClusters that match the O2IMS node cluster collector criteria.
func ListORANEligibleManagedClusters(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ManagedClusterBuilder, error) {
	managedClusters, err := ListManagedClusters(apiClient, options...)
	if err != nil {
		return nil, err
	}

	var eligibleManagedClusters []*ManagedClusterBuilder

	for _, managedCluster := range managedClusters {
		if IsORANEligibleManagedCluster(managedCluster.Definition) {
			eligibleManagedClusters = append(eligibleManagedClusters, managedCluster)
		}
	}

	return eligibleManagedClusters, nil
}

// IsORANEligibleManagedCluster reports whether a ManagedCluster would be ingested by the O2IMS cluster collector.
func IsORANEligibleManagedCluster(cluster *clusterv1.ManagedCluster) bool {
	if cluster == nil {
		return false
	}

	condition := meta.FindStatusCondition(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable)
	if condition == nil || condition.Status == metav1.ConditionFalse {
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
