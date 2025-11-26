package sriovenv

import (
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// GetOCPVersion retrieves the OpenShift cluster version.
func GetOCPVersion(apiClient *clients.Settings) (string, error) {
	klog.V(90).Info("Retrieving OpenShift cluster version")

	clusterVersion, err := clusterversion.Pull(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster version: %w", err)
	}

	if clusterVersion.Object == nil {
		return "", fmt.Errorf("cluster version object is nil")
	}

	// Try to get completed version from history first
	// History is ordered by recency with newest update first (index 0 is most recent)
	histories := clusterVersion.Object.Status.History
	for i := 0; i < len(histories); i++ {
		if histories[i].State == configv1.CompletedUpdate {
			version := histories[i].Version
			klog.V(90).Infof("Found completed cluster version: %s", version)

			return version, nil
		}
	}

	// Fall back to desired version
	version := clusterVersion.Object.Status.Desired.Version
	klog.V(90).Infof("Using desired cluster version: %s", version)

	return version, nil
}

// GetSriovOperatorVersion retrieves the SR-IOV operator version from CSV.
func GetSriovOperatorVersion(apiClient *clients.Settings, namespace string) (string, error) {
	klog.V(90).Infof("Retrieving SR-IOV operator version from namespace %s", namespace)

	// List CSVs in the SR-IOV operator namespace
	csvList, err := olm.ListClusterServiceVersion(apiClient, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to list CSVs in namespace %s: %w", namespace, err)
	}

	// Look for SR-IOV operator CSV using more specific matching
	// The SR-IOV operator CSV typically contains "sriov-network-operator" in the name
	const expectedCSVNameSubstring = "sriov-network-operator"

	for _, csv := range csvList {
		csvName := strings.ToLower(csv.Object.Name)
		csvDisplayName := strings.ToLower(csv.Object.Spec.DisplayName)

		// Prefer exact match on expected substring, fall back to generic "sriov" match
		if strings.Contains(csvName, expectedCSVNameSubstring) ||
			strings.Contains(csvDisplayName, expectedCSVNameSubstring) ||
			strings.Contains(csvName, "sriov") ||
			strings.Contains(csvDisplayName, "sriov") {
			version := csv.Object.Spec.Version.String()
			klog.V(90).Infof("Found SR-IOV operator CSV: %s, Version: %s", csv.Object.Name, version)

			return version, nil
		}
	}

	return "", fmt.Errorf("SR-IOV operator CSV not found in namespace %s", namespace)
}

// PodContainerInfo represents container information from a pod.
type PodContainerInfo struct {
	PodName       string
	ContainerName string
	Image         string
}

// GetSriovOperatorPodContainers retrieves container information from all pods in the SR-IOV operator namespace.
// This includes both regular containers and init containers.
func GetSriovOperatorPodContainers(apiClient *clients.Settings, namespace string) ([]PodContainerInfo, error) {
	klog.V(90).Infof("Retrieving SR-IOV operator pod container information from namespace %s", namespace)

	podList, err := pod.List(apiClient, namespace, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	var containerInfo []PodContainerInfo

	for _, podBuilder := range podList {
		if podBuilder.Object == nil {
			continue
		}

		podName := podBuilder.Object.Name

		// Get init containers from pod spec
		for _, container := range podBuilder.Object.Spec.InitContainers {
			containerInfo = append(containerInfo, PodContainerInfo{
				PodName:       podName,
				ContainerName: container.Name + " (init)",
				Image:         container.Image,
			})
			klog.V(90).Infof("Found init container: pod=%s, container=%s, image=%s", podName, container.Name, container.Image)
		}

		// Get regular containers from pod spec
		for _, container := range podBuilder.Object.Spec.Containers {
			containerInfo = append(containerInfo, PodContainerInfo{
				PodName:       podName,
				ContainerName: container.Name,
				Image:         container.Image,
			})
			klog.V(90).Infof("Found container: pod=%s, container=%s, image=%s", podName, container.Name, container.Image)
		}
	}

	if len(containerInfo) == 0 {
		return nil, fmt.Errorf("no containers found in pods in namespace %s", namespace)
	}

	return containerInfo, nil
}
