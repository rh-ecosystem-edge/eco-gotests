package csv

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"k8s.io/klog/v2"
)

// GetCurrentCSVNameFromSubscription returns operator's CSV name from the subscription.
func GetCurrentCSVNameFromSubscription(apiClient *clients.Settings,
	subscriptionName, subscriptionNamespace string) (string, error) {
	klog.V(100).Infof("Get CSV name from the subscription %s in the namespace %s",
		subscriptionName, subscriptionNamespace)

	subscriptionObj, err := olm.PullSubscription(apiClient, subscriptionName, subscriptionNamespace)
	if err != nil {
		klog.V(100).Infof("error pulling subscription %s from cluster in namespace %s",
			subscriptionName, subscriptionNamespace)

		return "", err
	}

	return subscriptionObj.Object.Status.CurrentCSV, nil
}
