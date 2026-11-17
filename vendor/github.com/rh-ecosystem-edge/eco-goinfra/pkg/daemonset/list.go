package daemonset

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// List returns DaemonSets in the namespace, preserving API errors for absence checks.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*Builder, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("daemonsetList 'apiClient' cannot be empty")
	}

	if nsname == "" {
		return nil, fmt.Errorf("daemonsetList 'nsname' cannot be empty")
	}

	if len(options) > 1 {
		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	passedOptions := metav1.ListOptions{}
	if len(options) == 1 {
		passedOptions = options[0]
	}

	klog.V(100).Infof("Listing DaemonSets in namespace %s with options %v", nsname, passedOptions)
	client := apiClient.DaemonSets(nsname)

	list, err := client.List(logging.DiscardContext(), passedOptions)
	if err != nil {
		return nil, err
	}

	builders := make([]*Builder, 0, len(list.Items))
	for i := range list.Items {
		object := list.Items[i].DeepCopy()
		builders = append(builders, &Builder{apiClient: client, Object: object, Definition: object.DeepCopy()})
	}

	return builders, nil
}
