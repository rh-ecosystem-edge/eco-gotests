package ptpdaemon

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// ListPtpDaemonNodes lists the nodes that the PTP daemon is deployed on. It uses the PtpOperatorConfig
// spec.daemonNodeSelector.
func ListPtpDaemonNodes(client *clients.Settings) ([]*nodes.Builder, error) {
	ptpOperatorConfig, err := ptp.PullPtpOperatorConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PtpOperatorConfig: %w", err)
	}

	// Selector is a set of labels where matching nodes will have the PTP daemon deployed. A nil or empty selector
	// will match all nodes.
	selector := ptpOperatorConfig.Definition.Spec.DaemonNodeSelector

	nodeList, err := nodes.List(client, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes matching PtpOperatorConfig spec.daemonNodeSelector: %w", err)
	}

	if len(nodeList) == 0 {
		return nil, fmt.Errorf("no nodes match PtpOperatorConfig spec.daemonNodeSelector %v", selector)
	}

	return nodeList, nil
}

// GetPtpDaemonPodOnNode retrieves the PTP daemon pod running on the specified node. It returns an error if it cannot
// find exactly one PTP daemon pod on the node.
func GetPtpDaemonPodOnNode(client *clients.Settings, nodeName string) (*pod.Builder, error) {
	daemonPods, err := pod.List(client, ranparam.PtpOperatorNamespace, metav1.ListOptions{
		LabelSelector: ranparam.PtpDaemonsetLabelSelector,
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
