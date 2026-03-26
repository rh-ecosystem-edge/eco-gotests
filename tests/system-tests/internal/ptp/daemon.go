package ptp

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// GetLinuxptpDaemonPodOnNode returns the openshift-ptp linuxptp-daemon pod scheduled on nodeName.
func GetLinuxptpDaemonPodOnNode(apiClient *clients.Settings, nodeName string) (*pod.Builder, error) {
	daemonPods, err := pod.List(apiClient, Namespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			DaemonPodLabelKey: DaemonPodLabelValueLinuxpt,
		}).String(),
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list PTP daemon pods on node %s: %w", nodeName, err)
	}

	if len(daemonPods) != 1 {
		return nil, fmt.Errorf("expected exactly one PTP daemon pod on node %s, found %d", nodeName, len(daemonPods))
	}

	return daemonPods[0], nil
}
