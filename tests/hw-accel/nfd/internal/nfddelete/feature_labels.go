package nfddelete

import (
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

// NfdLabelsByKeys delete labels from a given list.
func NfdLabelsByKeys(apiClient *clients.Settings, labelsRemove ...string) error {
	nodes, err := nodes.List(apiClient, metav1.ListOptions{
		LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
	if err != nil {
		klog.V(nfdparams.LogLevel).Infof("failed retrieving node list")

		return err
	}

	for _, node := range nodes {
		labels := node.Definition.Labels
		updated := false

		for label := range labels {
			for _, searchLabel := range labelsRemove {
				if strings.Contains(label, searchLabel) {
					delete(labels, label)

					updated = true
				}
			}
		}

		if updated {
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				node.Definition.Labels = labels
				_, updateErr := node.Update()

				return updateErr
			})
			if retryErr != nil {
				klog.V(nfdparams.LogLevel).Infof("Failed to update node %s: %v\n", node.Definition.Name, retryErr)

				return retryErr
			}
		}
	}

	return nil
}
