package cluster

import (
	"errors"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/proxy"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"k8s.io/klog/v2"
)

// APIClientGetter is an interface that returns an APIClient from a struct.
type APIClientGetter interface {
	GetAPIClient() (*clients.Settings, error)
}

// GetOCPClusterVersion leverages APIClientGetter to retrieve the OCP clusterversion from an arbitrary cluster.
func GetOCPClusterVersion(clusterObj APIClientGetter) (*clusterversion.Builder, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Gathering OCP clusterversion from cluster at %s", apiClient.KubeconfigPath)

	clusterVersion, err := clusterversion.Pull(apiClient)
	if err != nil {
		return nil, err
	}

	return clusterVersion, nil
}

// GetOCPNetworkConfig leverages APIClientGetter to retrieve the OCP cluster network from an arbitrary cluster.
func GetOCPNetworkConfig(clusterObj APIClientGetter) (*network.ConfigBuilder, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Gathering OCP network from cluster at %s", apiClient.KubeconfigPath)

	clusterNetwork, err := network.PullConfig(apiClient)
	if err != nil {
		return nil, err
	}

	return clusterNetwork, nil
}

// GetOCPNetworkOperatorConfig leverages APIClientGetter to retrieve the OCP cluster network from an arbitrary cluster.
func GetOCPNetworkOperatorConfig(clusterObj APIClientGetter) (*network.OperatorBuilder, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Gathering OCP network from cluster at %s", apiClient.KubeconfigPath)

	clusterNetwork, err := network.PullOperator(apiClient)
	if err != nil {
		return nil, err
	}

	return clusterNetwork, nil
}

// GetOCPPullSecret leverages APIClientGetter to retrieve the OCP pull-secret from an arbitrary cluster.
func GetOCPPullSecret(clusterObj APIClientGetter) (*secret.Builder, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Gathering OCP pull-secret from cluster at %s", apiClient.KubeconfigPath)

	pullSecret, err := secret.Pull(apiClient, "pull-secret", "openshift-config")
	if err != nil {
		return nil, err
	}

	_, ok := pullSecret.Object.Data[".dockerconfigjson"]
	if !ok {
		return nil, errors.New("pull-secret does not contain .dockerconfigjson")
	}

	return pullSecret, nil
}

// GetOCPProxy leverages APIClientGetter to retrieve the OCP cluster proxy from an arbitrary cluster.
func GetOCPProxy(clusterObj APIClientGetter) (*proxy.Builder, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Gathering OCP proxy from cluster at %s", apiClient.KubeconfigPath)

	clusterProxy, err := proxy.Pull(apiClient)
	if err != nil {
		return nil, err
	}

	return clusterProxy, err
}

// Connected checks that the OCP cluster of the provided client is connected.
func Connected(clusterObj APIClientGetter) (bool, error) {
	clusterVersion, err := GetOCPClusterVersion(clusterObj)
	if err != nil {
		return false, fmt.Errorf("failed to get clusterversion from cluster: %w", err)
	}

	for _, condition := range clusterVersion.Object.Status.Conditions {
		if condition.Type == configv1.RetrievedUpdates {
			if condition.Reason == "RemoteFailed" {
				return false, nil
			}

			return true, nil
		}
	}

	return false, fmt.Errorf("failed to determine if cluster is connected, "+
		"could not find '%s' condition", configv1.RetrievedUpdates)
}

// Disconnected checks that the OCP cluster of the provided client is disconnected.
func Disconnected(clusterObj APIClientGetter) (bool, error) {
	clusterVersion, err := GetOCPClusterVersion(clusterObj)
	if err != nil {
		return false, fmt.Errorf("failed to get clusterversion from cluster: %w", err)
	}

	for _, condition := range clusterVersion.Object.Status.Conditions {
		if condition.Type == configv1.RetrievedUpdates {
			if condition.Reason == "RemoteFailed" {
				return true, nil
			}

			return false, nil
		}
	}

	return false, fmt.Errorf("failed to determine if cluster is disconnected, "+
		"could not find '%s' condition", configv1.RetrievedUpdates)
}

// checkAPIClient determines if the APIClient returned is not nil.
func checkAPIClient(clusterObj APIClientGetter) (*clients.Settings, error) {
	klog.V(90).Infof("Getting APIClient from provided object")

	apiClient, err := clusterObj.GetAPIClient()
	if err != nil {
		return nil, err
	}

	if apiClient == nil {
		klog.V(90).Infof("The returned APIClient is nil")

		return nil, fmt.Errorf("cannot discover cluster information when APIClient is nil")
	}

	return apiClient, nil
}

// GetOCPClusterName retrieves the OCP cluster name from an arbitrary cluster.
func GetOCPClusterName(clusterObj APIClientGetter) (string, error) {
	apiClient, err := checkAPIClient(clusterObj)
	if err != nil {
		return "", err
	}

	klog.V(90).Infof("Gathering OCP cluster name from infrastructure at %s", apiClient.KubeconfigPath)

	infraConfig, err := infrastructure.Pull(apiClient)
	if err != nil {
		return "", err
	}

	// The cluster name is the infrastructure name without the last part, e.g. "kni-qe-12-p746q" -> "kni-qe-12"
	parts := strings.Split(infraConfig.Object.Status.InfrastructureName, "-")

	return strings.Join(parts[:len(parts)-1], "-"), nil
}
