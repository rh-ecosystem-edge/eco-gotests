package mocksmo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	deploymentName                  = "oran-smo-server"
	observerPathPrefix              = "/mock_smo/v1/observers"
	observerEchoNotificationMessage = "Observer echo notification"
)

// LogLevel is the default glog verbosity level for this package.
const LogLevel klog.Level = 90

var labelSelector = map[string]string{"app": deploymentName}

// ObserverCallbackURL returns the fully qualified observer echo callback URL for the given observer ID.
func ObserverCallbackURL(smoBaseURL, observerID string) string {
	return strings.TrimSuffix(smoBaseURL, "/") + observerPathPrefix + "/" + observerID
}

// observerEchoLogEntry is the slog JSON shape for observer echo notifications in mock SMO pod logs.
type observerEchoLogEntry struct {
	Msg        string          `json:"msg"`
	ObserverID string          `json:"observerId"`
	Body       json.RawMessage `json:"body"`
}

// waitForNotificationOptions are all of the options for the wait for a matching notification. It is used for an
// options pattern to the WaitForNotification function.
type waitForNotificationOptions struct {
	timeout    time.Duration
	start      time.Time
	observerID string
	matchFunc  func(notification *oranapi.AlarmEventNotification) bool
}

// getDefaultWaitForNotificationOptions returns the default options for the wait for a matching notification. The
// defaults are a 30 second timeout, start time of now (when this function is called) and a match function that returns
// true if any notification is received.
func getDefaultWaitForNotificationOptions() *waitForNotificationOptions {
	return &waitForNotificationOptions{
		timeout:   time.Second * 30,
		start:     time.Now(),
		matchFunc: func(notification *oranapi.AlarmEventNotification) bool { return true },
	}
}

// waitForNotificationOption is a function that can be used to modify the options for the wait for a matching
// notification.
type waitForNotificationOption func(options *waitForNotificationOptions)

// WithTimeout sets the timeout for the wait for a matching notification.
func WithTimeout(timeout time.Duration) waitForNotificationOption {
	return func(options *waitForNotificationOptions) {
		options.timeout = timeout
	}
}

// WithStart sets the start time for the wait for a matching notification.
func WithStart(start time.Time) waitForNotificationOption {
	return func(options *waitForNotificationOptions) {
		options.start = start
	}
}

// WithObserverID limits notifications to those received for the given observer ID.
func WithObserverID(observerID string) waitForNotificationOption {
	return func(options *waitForNotificationOptions) {
		options.observerID = observerID
	}
}

// WithMatchFunc sets the match function for the wait for a matching notification.
func WithMatchFunc(matchFunc func(notification *oranapi.AlarmEventNotification) bool) waitForNotificationOption {
	return func(options *waitForNotificationOptions) {
		options.matchFunc = matchFunc
	}
}

// PullPod pulls the mock SMO server pod from the cluster. It will fail if no pods matching the app label are found. If
// more than one pod is found, it will log a warning and return the first one.
func PullPod(client *clients.Settings, nsname string) (*pod.Builder, error) {
	matchingPods, err := pod.List(client, nsname, metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for the mock SMO server: %w", err)
	}

	if len(matchingPods) == 0 {
		return nil, fmt.Errorf("no pods for the mock SMO server found in namespace %q", nsname)
	}

	if len(matchingPods) > 1 {
		klog.V(LogLevel).Infof("Expected 1 pod for the mock SMO server, but found %d in namespace %q, using the first one",
			len(matchingPods), nsname)
	}

	klog.V(LogLevel).Infof("Successfully pulled pod %q in namespace %q for mock SMO server",
		matchingPods[0].Definition.Name, nsname)

	return matchingPods[0], nil
}

// WaitForNotification waits for an observer echo notification in the mock SMO server logs. Callers may provide options,
// otherwise the defaults of 30 seconds timeout, start time of now, and a match function that returns true if any
// notification is received will be used.
func WaitForNotification(client *clients.Settings, namespace string, options ...waitForNotificationOption) error {
	appliedOptions := getDefaultWaitForNotificationOptions()

	for _, option := range options {
		option(appliedOptions)
	}

	pod, err := PullPod(client, namespace)
	if err != nil {
		return fmt.Errorf("failed to pull mock SMO server pod: %w", err)
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, appliedOptions.timeout, true, func(ctx context.Context) (bool, error) {
			newStart := time.Now()

			notificationsRaw, err := pod.GetLogsWithOptions(&corev1.PodLogOptions{
				SinceTime: &metav1.Time{Time: appliedOptions.start},
			})
			if err != nil {
				return false, fmt.Errorf("failed to get mock SMO server pod logs: %w", err)
			}

			// By setting the new start time, we avoid looking at the same notifications again. This is not
			// perfect, since we may still see duplicates, but it guarantees we do not miss any while being
			// more efficient.
			appliedOptions.start = newStart

			parsedNotifications, err := parseNotifications(notificationsRaw, appliedOptions.observerID)
			if err != nil {
				return false, fmt.Errorf("failed to parse notifications: %w", err)
			}

			return slices.ContainsFunc(parsedNotifications, appliedOptions.matchFunc), nil
		})
}

// ListReceivedNotifications lists observer echo notifications received since the given time. If sinceTime is zero, then
// all notifications will be listed. If observerID is non-empty, only notifications for that observer are returned.
func ListReceivedNotifications(
	client *clients.Settings,
	namespace string,
	sinceTime time.Time,
	observerID string,
) ([]*oranapi.AlarmEventNotification, error) {
	pod, err := PullPod(client, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull mock SMO server pod: %w", err)
	}

	podOptionSinceTime := &metav1.Time{Time: sinceTime}
	if sinceTime.IsZero() {
		podOptionSinceTime = nil
	}

	notificationsRaw, err := pod.GetLogsWithOptions(&corev1.PodLogOptions{
		SinceTime: podOptionSinceTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get mock SMO server pod logs: %w", err)
	}

	parsedNotifications, err := parseNotifications(notificationsRaw, observerID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse notifications: %w", err)
	}

	return parsedNotifications, nil
}

// parseNotifications parses observer echo notifications from mock SMO pod logs. It will return any errors encountered
// in parsing. If an error is returned, the notifications will be empty. Elements of the returned slice are guaranteed
// not to be nil.
//
// Notifications are expected as slog JSON log lines with msg "Observer echo notification". The alarm body is nested
// under the body field. The parsing will look for the opening curly brace to determine the start of the JSON. Anything
// prior to it on the line will be ignored.
func parseNotifications(notificationsRaw []byte, observerID string) ([]*oranapi.AlarmEventNotification, error) {
	var notifications []*oranapi.AlarmEventNotification

	scanner := bufio.NewScanner(bytes.NewReader(notificationsRaw))
	for scanner.Scan() {
		line := scanner.Bytes()

		jsonStart := bytes.IndexByte(line, '{')
		if jsonStart == -1 {
			continue
		}

		var logEntry observerEchoLogEntry

		err := json.Unmarshal(line[jsonStart:], &logEntry)
		if err != nil {
			klog.V(LogLevel).Infof("Skipping malformed log line while parsing observer notifications: %v, line: %q",
				err, string(line))

			continue
		}

		if logEntry.Msg != observerEchoNotificationMessage {
			continue
		}

		if observerID != "" && logEntry.ObserverID != observerID {
			continue
		}

		if len(logEntry.Body) == 0 || bytes.Equal(logEntry.Body, []byte("null")) {
			continue
		}

		var notification oranapi.AlarmEventNotification

		err = json.Unmarshal(logEntry.Body, &notification)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal notification from log body %q: %w", string(logEntry.Body), err)
		}

		notifications = append(notifications, &notification)
	}

	err := scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("error scanning notifications: %w", err)
	}

	return notifications, nil
}
