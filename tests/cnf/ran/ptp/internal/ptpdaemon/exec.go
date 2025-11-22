// Package ptpdaemon provides functions for executing commands in the PTP daemon pod.
package ptpdaemon

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
)

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

// execCommandOptions is a struct that contains the options for the execCommand function. It should not be used directly
// since the ExecCommandOption type is used to set the options.
type execCommandOptions struct {
	attempts           uint
	retryOnError       bool
	retryOnEmptyOutput bool
	retryDelay         time.Duration
}

// ExecCommandOption is a function type that can be used to set the options for the execCommand function. It should not
// be implemented outside of the functions provided by this package.
type ExecCommandOption func(*execCommandOptions)

// WithRetries sets the number of attempts to execute the command. The number of attempts is the number of retries plus
// one, since the first attempt is not a retry. It defaults to 1 attempt and 0 retries.
func WithRetries(retries uint) ExecCommandOption {
	return func(o *execCommandOptions) {
		o.attempts = retries + 1
	}
}

// WithRetryOnError sets whether to retry the command if it returns an error. It defaults to false.
func WithRetryOnError(retryOnError bool) ExecCommandOption {
	return func(o *execCommandOptions) {
		o.retryOnError = retryOnError
	}
}

// WithRetryOnEmptyOutput sets whether to retry the command if it returns an empty output. It defaults to false.
func WithRetryOnEmptyOutput(retryOnEmptyOutput bool) ExecCommandOption {
	return func(o *execCommandOptions) {
		o.retryOnEmptyOutput = retryOnEmptyOutput
	}
}

// WithRetryDelay sets the delay between retries. It defaults to 1 second.
func WithRetryDelay(retryDelay time.Duration) ExecCommandOption {
	return func(o *execCommandOptions) {
		o.retryDelay = retryDelay
	}
}

// ExecuteCommandInPtpDaemonPod executes a command in the PTP daemon pod running on the specified node, optionally with
// the provided options. Note that retries will lookup the PTP daemon pod on each retry to account for the pod being
// deleted and recreated.
func ExecuteCommandInPtpDaemonPod(
	client *clients.Settings, nodeName string, command string, options ...ExecCommandOption) (string, error) {
	execOptions := &execCommandOptions{
		attempts:           1,
		retryOnError:       false,
		retryOnEmptyOutput: false,
		retryDelay:         1 * time.Second,
	}

	for _, option := range options {
		option(execOptions)
	}

	// This loop handles all the retries. After the first attempt, we wait for the retry delay before retrying.
	// Failed attempts to get the PTP daemon pod will always be retried when there are remaining attempts.
	for retry := range execOptions.attempts {
		if retry > 0 {
			time.Sleep(execOptions.retryDelay)
		}

		// We always handle the error by attempting to retry here since there is no option for whether to retry
		// the daemon pod lookup or not.
		daemonPod, err := GetPtpDaemonPodOnNode(client, nodeName)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to get PTP daemon pod on node %s: %v", nodeName, err)

			continue
		}

		// If there is the option to retry on error, we only log the error before continuing to retry.
		// Otherwise, we return the error immediately since we do not retry on it.
		output, err := daemonPod.ExecCommand([]string{"sh", "-c", command}, ranparam.PtpContainerName)
		if execOptions.retryOnError && err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to execute command %q in PTP daemon pod on node %s: %v",
				command, nodeName, err)

			continue
		} else if err != nil {
			return "", fmt.Errorf("failed to execute command %q in PTP daemon pod on node %s: %w", command, nodeName, err)
		}

		// Empty output is only considered an error if retrying on empty output, so there is no else if for this
		// check.
		if execOptions.retryOnEmptyOutput && output.Len() == 0 {
			klog.V(tsparams.LogLevel).Infof("Failed to execute command %q in PTP daemon pod on node %s: no output returned",
				command, nodeName)

			continue
		}

		// In the success case, we do not need to retry and we can return the output.
		return output.String(), nil
	}

	return "", fmt.Errorf(
		"failed to execute command %q in PTP daemon pod on node %s after %d attempts: ran out of attempts",
		command, nodeName, execOptions.attempts)
}
