package networkpolicy

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List returns networkpolicy inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*NetworkPolicyBuilder, error) {
	if nsname == "" {
		glog.V(100).Infof("networkpolicy 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list networkpolicies, 'nsname' parameter is empty")
	}

	passedOptions := metav1.ListOptions{}
	logMessage := fmt.Sprintf("Listing networkpolicies in the namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	networkpolicyList, err := apiClient.NetworkPolicies(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list networkpolicies in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var networkpolicyObjects []*NetworkPolicyBuilder

	for _, runningNetworkPolicy := range networkpolicyList.Items {
		copiedNetworkPolicy := runningNetworkPolicy
		networkpolicyBuilder := &NetworkPolicyBuilder{
			apiClient:  apiClient,
			Object:     &copiedNetworkPolicy,
			Definition: &copiedNetworkPolicy,
		}

		networkpolicyObjects = append(networkpolicyObjects, networkpolicyBuilder)
	}

	return networkpolicyObjects, nil
}
