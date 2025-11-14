package ptpdaemon

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
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

// EnsurePtpDaemonPodExistsOnNode waits until exactly one linuxptp-daemon pod exists on the given node
// and returns it. It retries until the provided timeout elapses.
func EnsurePtpDaemonPodExistsOnNode(
	client *clients.Settings,
	nodeName string,
	timeout time.Duration) (*pod.Builder, error) {
	var daemonPod *pod.Builder

	err := wait.PollUntilContextTimeout(
		context.TODO(), 1*time.Second, timeout, true, func(
			ctx context.Context) (bool, error) {
			var err error

			daemonPod, err = GetPtpDaemonPodOnNode(client, nodeName)
			if err == nil {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("timed out ensuring PTP daemon pod exists on node %s: %w", nodeName, err)
	}

	return daemonPod, nil
}

// ValidatePtpDaemonPodRunning ensures the linuxptp-daemon pod exists on the given node and
// validates it remains in Running state continuously for 45 seconds.
func ValidatePtpDaemonPodRunning(client *clients.Settings, nodeName string) error {
	daemonPod, err := EnsurePtpDaemonPodExistsOnNode(client, nodeName, 5*time.Minute)
	if err != nil {
		return err
	}

	// First, wait until the pod reaches Running.
	if err := daemonPod.WaitUntilRunning(5 * time.Minute); err != nil {
		return fmt.Errorf("PTP daemon pod on node %s did not reach Running: %w", nodeName, err)
	}

	// Then, validate it remains in Running state continuously for 45 seconds.
	err = wait.PollUntilContextTimeout(
		context.TODO(), 1*time.Second, 45*time.Second, true, func(ctx context.Context) (bool, error) {
			err := daemonPod.WaitUntilInStatus(corev1.PodRunning, 1*time.Second)
			if err != nil {
				return false, fmt.Errorf("PTP daemon pod on node %s was not Running continuously for 45s: %w", nodeName, err)
			}

			return false, nil
		})

	if wait.Interrupted(err) {
		return nil
	}

	return err
}
