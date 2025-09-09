package find

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterVersion returns the Major.Minor part of a cluster's OCP version.
func ClusterVersion(clusterObj cluster.APIClientGetter) (string, error) {
	clusterVersion, err := cluster.GetOCPClusterVersion(clusterObj)
	if err != nil {
		return "", err
	}

	if !clusterVersion.Exists() {
		return "", fmt.Errorf("cluster version not found")
	}

	splitVersion := strings.Split(clusterVersion.Object.Status.Desired.Version, ".")

	return fmt.Sprintf("%s.%s", splitVersion[0], splitVersion[1]), nil
}

// SpokeClusterName returns the spoke cluster name based on hub and spoke cluster apiclients.
func SpokeClusterName(hubAPIClient, spokeAPIClient *clients.Settings) (string, error) {
	spokeClusterVersion, err := cluster.GetOCPClusterVersion(spokeAPIClient)
	if err != nil {
		return "", err
	}

	if !spokeClusterVersion.Exists() {
		return "", fmt.Errorf("spoke cluster version not found")
	}

	spokeClusterID := spokeClusterVersion.Object.Spec.ClusterID

	clusterDeployments, err := hive.ListClusterDeploymentsInAllNamespaces(hubAPIClient)
	if err != nil {
		return "", err
	}

	for _, clusterDeploymentBuilder := range clusterDeployments {
		if clusterDeploymentBuilder.Object.Spec.ClusterMetadata != nil &&
			clusterDeploymentBuilder.Object.Spec.ClusterMetadata.ClusterID == string(spokeClusterID) {
			return clusterDeploymentBuilder.Object.Spec.ClusterName, nil
		}
	}

	return "", fmt.Errorf("could not find ClusterDeployment from provided API clients")
}

// AssistedServicePod returns pod running assisted-service.
func AssistedServicePod(apiClient *clients.Settings) (*pod.Builder, error) {
	return getPodBuilder(apiClient, "app=assisted-service")
}

// AssistedImageServicePod returns pod running assisted-image-service.
func AssistedImageServicePod(apiClient *clients.Settings) (*pod.Builder, error) {
	return getPodBuilder(apiClient, "app=assisted-image-service")
}

// InfrastructureOperatorPod returns pod running infrastructure-operator.
func InfrastructureOperatorPod(apiClient *clients.Settings) (*pod.Builder, error) {
	return getPodBuilder(apiClient, "control-plane=infrastructure-operator")
}

// getPodBuilder returns a podBuilder of a pod based on provided label.
func getPodBuilder(apiClient *clients.Settings, label string) (*pod.Builder, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("apiClient is nil")
	}

	podList, err := pod.ListInAllNamespaces(apiClient, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods on cluster: %w", err)
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf("pod with label '%s' not currently running", label)
	}

	if len(podList) > 1 {
		return nil, fmt.Errorf("got unexpected pods when checking for pods with label '%s'", label)
	}

	return podList[0], nil
}
