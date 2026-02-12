package daemonlogs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// LogMatcher is a function type that matches a log line. It returns true if the log line matches the criteria.
type LogMatcher func(string) bool

// ContainsMatcher returns a LogMatcher that matches a log line if it contains the given message.
func ContainsMatcher(message string) LogMatcher {
	return func(line string) bool {
		return strings.Contains(line, message)
	}
}

// RegexpMatcher returns a LogMatcher that matches a log line if it matches the given regular expression.
func RegexpMatcher(regexp *regexp.Regexp) LogMatcher {
	return func(line string) bool {
		return regexp.MatchString(line)
	}
}

// waitForPodLogOptions is a struct that contains the options for the WaitForPodLog function. It should not be used
// directly since the WaitForPodLogOption type is used to set the options.
type waitForPodLogOptions struct {
	startTime       time.Time
	timeout         time.Duration
	pollingInterval time.Duration
	matcher         LogMatcher
	ignoreTimeout   bool
}

// WaitForPodLogOption is a function type that can be used to set the options for the WaitForPodLog function. It should
// not be implemented outside of the functions provided by this package.
type WaitForPodLogOption func(*waitForPodLogOptions)

// WithStartTime sets the start time from which to fetch logs. Logs before this time will not be considered. No
// validation is performed on this value. It defaults to the current time when WaitForPodLog is called.
func WithStartTime(startTime time.Time) WaitForPodLogOption {
	return func(o *waitForPodLogOptions) {
		o.startTime = startTime
	}
}

// WithTimeout sets the maximum time to wait for the log message to appear. Values less than or equal to zero will
// print a log and this option will be a no-op. It defaults to 30 seconds.
func WithTimeout(timeout time.Duration) WaitForPodLogOption {
	if timeout <= 0 {
		klog.V(tsparams.LogLevel).Infof("Timeout cannot be less than or equal to zero, falling back to the default")

		return func(o *waitForPodLogOptions) {}
	}

	return func(o *waitForPodLogOptions) {
		o.timeout = timeout
	}
}

// WithPollingInterval sets the interval between polling attempts. Values less than or equal to zero will print a log
// and this option will be a no-op. It defaults to 1 second.
func WithPollingInterval(interval time.Duration) WaitForPodLogOption {
	if interval <= 0 {
		klog.V(tsparams.LogLevel).Infof("Polling interval cannot be less than or equal to zero, falling back to the default")

		return func(o *waitForPodLogOptions) {}
	}

	return func(o *waitForPodLogOptions) {
		o.pollingInterval = interval
	}
}

// WithMatcher sets the matcher function to use for matching log lines. If a nil matcher is provided, a log will be
// printed and this option will be a no-op. It defaults to a matcher that always returns false.
func WithMatcher(matcher LogMatcher) WaitForPodLogOption {
	if matcher == nil {
		klog.V(tsparams.LogLevel).Infof("Matcher function cannot be nil, falling back to the default")

		return func(o *waitForPodLogOptions) {}
	}

	return func(o *waitForPodLogOptions) {
		o.matcher = matcher
	}
}

// defaultMatcher is a matcher that always returns false. This will cause the function to always time out.
func defaultMatcher(line string) bool {
	return false
}

// getDefaultWaitForPodLogOptions returns a waitForPodLogOptions struct with default values.
func getDefaultWaitForPodLogOptions() *waitForPodLogOptions {
	return &waitForPodLogOptions{
		startTime:       time.Now(),
		timeout:         30 * time.Second,
		pollingInterval: 1 * time.Second,
		matcher:         defaultMatcher,
		ignoreTimeout:   false,
	}
}

// WaitForPodLog waits for a message to appear in the PTP daemon pod logs on the specified node. It polls the logs at
// the specified interval until either the matcher function returns true for a log line or the timeout is reached. Note
// that the function will lookup the PTP daemon pod on each poll to account for the pod being deleted and recreated.
func WaitForPodLog(client *clients.Settings, nodeName string, options ...WaitForPodLogOption) error {
	logOptions := getDefaultWaitForPodLogOptions()

	for _, option := range options {
		option(logOptions)
	}

	if logOptions.matcher == nil {
		return fmt.Errorf("matcher function must be provided using WithMatcher option")
	}

	// Track the last time we successfully fetched logs to avoid missing log entries between polls.
	lastFetchTime := logOptions.startTime

	return wait.PollUntilContextTimeout(
		context.TODO(), logOptions.pollingInterval, logOptions.timeout, true, func(ctx context.Context) (bool, error) {
			// Get the PTP daemon pod on each poll to handle pod restarts.
			daemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(client, nodeName)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to get PTP daemon pod on node %s: %v", nodeName, err)

				return false, nil
			}

			// We save the time of the next last fetch before getting the logs to avoid missing log entries
			// between polls. Once the logs are fetched, we update the last fetch time to this time.
			localFetchTime := time.Now()

			logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
				SinceTime: &metav1.Time{Time: lastFetchTime},
				Container: ranparam.PtpContainerName,
			})
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to get logs starting at %s for PTP daemon pod on node %s: %v",
					lastFetchTime, nodeName, err)

				return false, nil
			}

			// Update the last fetch time to the current time before processing logs. This could cause
			// duplicates, but this will not affect the result.
			lastFetchTime = localFetchTime

			for line := range strings.SplitSeq(string(logs), "\n") {
				if logOptions.matcher(line) {
					klog.V(tsparams.LogLevel).Infof("Found matching log line in PTP daemon pod on node %s: %q", nodeName, line)

					return true, nil
				}
			}

			return false, nil
		})
}

// profileLoadMessage is the message that appears in the linuxptp-daemon-container logs when the profiles are loaded.
const profileLoadMessage = "load profiles"

// WaitForProfileLoad waits for the profile load message to appear in the PTP daemon pod logs on the specified node. It
// matches for the profile load message and uses the provided options for the WaitForPodLog function.
func WaitForProfileLoad(client *clients.Settings, nodeName string, options ...WaitForPodLogOption) error {
	options = append(options, WithMatcher(ContainsMatcher(profileLoadMessage)))

	err := WaitForPodLog(client, nodeName, options...)
	if err != nil {
		return fmt.Errorf("failed to wait for profile load on node %s: %w", nodeName, err)
	}

	return nil
}

// WaitForProfileLoadOnNodes waits for the profile load message to appear in the PTP daemon pod logs on the specified
// nodes. It will check each node concurrently and return all errors that occur. Similar to [WaitForProfileLoad], it
// uses the provided options for the WaitForPodLog function.
func WaitForProfileLoadOnNodes(
	client *clients.Settings, nodeNames []string, options ...WaitForPodLogOption) error {
	if len(nodeNames) == 0 {
		return nil
	}

	var waitGroup sync.WaitGroup

	errChannel := make(chan error, len(nodeNames))

	for _, nodeName := range nodeNames {
		waitGroup.Go(func() {
			err := WaitForProfileLoad(client, nodeName, options...)
			if err != nil {
				errChannel <- fmt.Errorf("failed to wait for profile load on node %s: %w", nodeName, err)
			}
		})
	}

	waitGroup.Wait()
	close(errChannel)

	var allErrors []error

	for err := range errChannel {
		allErrors = append(allErrors, err)
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("failed to wait for profile load on nodes %v: %w", nodeNames, errors.Join(allErrors...))
	}

	return nil
}

// WaitForProfileLoadOnPTPNodes waits for the profile load message to appear in the PTP daemon pod logs on all PTP
// daemon nodes. It will check each node concurrently and return all errors that occur. Similar to [WaitForProfileLoad],
// it uses the provided options for the WaitForPodLog function.
func WaitForProfileLoadOnPTPNodes(client *clients.Settings, options ...WaitForPodLogOption) error {
	nodes, err := ptpdaemon.ListPtpDaemonNodes(client)
	if err != nil {
		return fmt.Errorf("failed to list PTP daemon nodes: %w", err)
	}

	nodeNames := make([]string, len(nodes))
	for index, node := range nodes {
		nodeNames[index] = node.Definition.Name
	}

	err = WaitForProfileLoadOnNodes(client, nodeNames, options...)
	if err != nil {
		return fmt.Errorf("failed to wait for profile load on PTP daemon nodes: %w", err)
	}

	return nil
}
