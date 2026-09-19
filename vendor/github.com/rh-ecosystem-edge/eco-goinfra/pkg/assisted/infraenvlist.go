package assisted

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListInfraEnvsInAllNamespaces returns a cluster-wide InfraEnv inventory.
func ListInfraEnvsInAllNamespaces(
	apiClient *clients.Settings, options ...goclient.ListOption) ([]*InfraEnvBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	infraEnvList := &agentInstallV1Beta1.InfraEnvList{}

	err := apiClient.List(logging.DiscardContext(), infraEnvList, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list infraenvs across all namespaces due to %s", err.Error())

		return nil, err
	}

	infraEnvs := make([]*InfraEnvBuilder, 0, len(infraEnvList.Items))

	for _, infraEnvObj := range infraEnvList.Items {
		copiedInfraEnv := infraEnvObj
		infraEnvs = append(infraEnvs, &InfraEnvBuilder{
			apiClient:  apiClient.Client,
			Definition: &copiedInfraEnv,
			Object:     &copiedInfraEnv,
		})
	}

	return infraEnvs, nil
}

// ListInfraEnvs returns an InfraEnv inventory in a given namespace.
func ListInfraEnvs(apiClient *clients.Settings, namespace string) ([]*InfraEnvBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	if namespace == "" {
		return nil, fmt.Errorf("namespace to list infraenvs cannot be empty")
	}

	return ListInfraEnvsInAllNamespaces(apiClient, &goclient.ListOptions{Namespace: namespace})
}
