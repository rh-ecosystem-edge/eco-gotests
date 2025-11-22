package platform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ingress"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
)

// GetOCPVersion retrieves the OCP clusterversion object from an arbitrary cluster and get current ocp version value.
func GetOCPVersion(apiClient *clients.Settings) (string, error) {
	klog.V(90).Infof("Gathering OCP version from cluster at %s", apiClient.KubeconfigPath)

	clusterVersionObj, err := cluster.GetOCPClusterVersion(apiClient)
	if err != nil {
		return "", err
	}

	ocpVersion := clusterVersionObj.Object.Status.Desired.Version

	return ocpVersion, nil
}

// GetOCPNetworkType retrieves the OCP cluster network from an arbitrary cluster and get ocp network type.
func GetOCPNetworkType(apiClient *clients.Settings) (string, error) {
	klog.V(90).Infof("Gathering OCP network type from cluster at %s", apiClient.KubeconfigPath)

	clusterNetworkConfigObj, err := cluster.GetOCPNetworkConfig(apiClient)
	if err != nil {
		return "", err
	}

	networkType := clusterNetworkConfigObj.Object.Status.NetworkType

	return networkType, nil
}

// GetOCPClusterName retrieves the OCP cluster name from an arbitrary cluster.
func GetOCPClusterName(apiClient *clients.Settings) (string, error) {
	klog.V(100).Infof("Gathering OCP cluster name from cluster at %s", apiClient.KubeconfigPath)

	infraConfig, err := infrastructure.Pull(apiClient)
	if err != nil {
		return "", err
	}

	// The cluster name is the infrastructure name without the last part, e.g. "kni-qe-12-p746q" -> "kni-qe-12"
	parts := strings.Split(infraConfig.Object.Status.InfrastructureName, "-")

	return strings.Join(parts[:len(parts)-1], "-"), nil
}

// GetOCPPlatformType retrieves the OCP cluster platform type from an arbitrary cluster.
func GetOCPPlatformType(apiClient *clients.Settings) (configv1.PlatformType, error) {
	klog.V(100).Infof("Gathering OCP cluster platform type from cluster at %s", apiClient.KubeconfigPath)

	infraConfig, err := infrastructure.Pull(apiClient)
	if err != nil {
		return "", err
	}

	return infraConfig.Object.Spec.PlatformSpec.Type, nil
}

// GetOCPIngressDomain retrieves the OCP cluster ingress domain from an arbitrary cluster.
func GetOCPIngressDomain(apiClient *clients.Settings) (string, error) {
	klog.V(100).Infof("Gathering OCP cluster ingress domain from cluster at %s", apiClient.KubeconfigPath)

	ingressController, err := ingress.Pull(apiClient, "default", "openshift-ingress-operator")
	if err != nil {
		return "", err
	}

	return strings.ToLower(ingressController.Object.Status.Domain), nil
}

// GetLocalMirrorRegistryURL retrieves the OCP local mirror registry url from an arbitrary cluster.
func GetLocalMirrorRegistryURL(apiClient *clients.Settings) (string, error) {
	if SystemTestsTestConfig.DestinationRegistryURL != "" {
		return SystemTestsTestConfig.DestinationRegistryURL, nil
	}

	mirrorRegistryMap, err := getMirrorRegistryMap(apiClient)
	if err != nil {
		return "", err
	}

	for registryURL, auth := range mirrorRegistryMap {
		klog.V(90).Infof("registry URL: %s, auth: %s", registryURL, auth)

		return registryURL, nil
	}

	return "", fmt.Errorf("local mirror registry url not found")
}

// IsDisconnectedDeployment retrieve the OCP mirror registry url and verify if the deployment type is disconnected.
func IsDisconnectedDeployment(apiClient *clients.Settings) (bool, error) {
	klog.V(100).Info("Check if cluster deployment type is disconnected")

	connectedDeploymentPattern := "cloud.openshift.com"

	mirrorRegistryMap, err := getMirrorRegistryMap(apiClient)
	if err != nil {
		return false, err
	}

	for registryURL, auth := range mirrorRegistryMap {
		klog.V(90).Infof("registry URL: %s, auth: %s", registryURL, auth)

		if strings.Contains(registryURL, connectedDeploymentPattern) {
			return false, nil
		}
	}

	return true, nil
}

// getMirrorRegistryMap retrieves the OCP mirror registry map from an arbitrary cluster.
func getMirrorRegistryMap(apiClient *clients.Settings) (map[string]interface{}, error) {
	pullSecret, err := cluster.GetOCPPullSecret(apiClient)
	if err != nil {
		return nil, err
	}

	data, ok := pullSecret.Object.Data[".dockerconfigjson"]
	if !ok {
		return nil, err
	}

	dataMap := make(map[string]interface{})

	err = json.Unmarshal(data, &dataMap)
	if err != nil {
		return nil, err
	}

	authsList := dataMap["auths"]

	registryMap := make(map[string]interface{})
	authsMap, passed := authsList.(map[string]interface{})

	if !passed {
		return nil, err
	}

	for key, value := range authsMap {
		authMap, passed := value.(map[string]interface{})

		if !passed {
			return nil, err
		}

		for _, authValue := range authMap {
			registryMap[key] = authValue
		}
	}

	return registryMap, nil
}

// CompareOCPVersionWithCurrent compares current OCP versions with the provided value.
func CompareOCPVersionWithCurrent(apiClient *clients.Settings,
	referenceOCPVersion string,
	isGreater, orEqual bool) (bool, error) {
	if apiClient == nil {
		return false, fmt.Errorf("'apiClient' cannot be empty")
	}

	if referenceOCPVersion == "" {
		return false, fmt.Errorf("'referenceOCPVersion' cannot be empty")
	}

	currentOCPVersion, err := GetOCPVersion(apiClient)
	if err != nil {
		return false, err
	}

	klog.V(100).Infof("The apiClient is empty")

	currentVersion, err := version.NewVersion(currentOCPVersion)
	if err != nil {
		return false, err
	}

	referenceVersion, err := version.NewVersion(referenceOCPVersion)
	if err != nil {
		return false, err
	}

	if isGreater {
		if orEqual {
			if currentVersion.GreaterThanOrEqual(referenceVersion) {
				return true, nil
			}
		} else {
			if currentVersion.GreaterThan(referenceVersion) {
				return true, nil
			}
		}
	}

	if orEqual {
		if currentVersion.LessThanOrEqual(referenceVersion) {
			return true, nil
		}
	}

	if currentVersion.LessThan(referenceVersion) {
		return true, nil
	}

	return false, nil
}
