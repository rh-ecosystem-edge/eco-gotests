package csv

import (
	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
)

// GetCurrentCSVNameFromSubscription returns operator's CSV name from the subscription.
func GetCurrentCSVNameFromSubscription(apiClient *clients.Settings,
	subscriptionName, subscriptionNamespace string) (string, error) {
	glog.V(100).Infof("Get CSV name from the subscription %s in the namespace %s",
		subscriptionName, subscriptionNamespace)

	subscriptionObj, err := olm.PullSubscription(apiClient, subscriptionName, subscriptionNamespace)

	if err != nil {
		glog.V(100).Infof("error pulling subscription %s from cluster in namespace %s",
			subscriptionName, subscriptionNamespace)

		return "", err
	}

	return subscriptionObj.Object.Status.CurrentCSV, nil
}
