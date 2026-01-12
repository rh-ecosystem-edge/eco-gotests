package events

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"slices"

	"github.com/redhat-cne/sdk-go/pkg/event"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

var (
	// containsEventRegexp is a regular expression that matches lines in the logs that contain events.
	containsEventRegexp         = regexp.MustCompile(`msg="(received event|event sent|Got CurrentState:)`)
	containsEventNotStateRegexp = regexp.MustCompile(`msg="(received event|event sent)`)
	// extractEventRegexp is a regular expression that extracts the event JSON from the log line. The event JSON
	// will still have superfluous backslashes after being extracted, however.
	extractEventRegexp = regexp.MustCompile(`\{.*\}`)
)

// waitForEventOptions is a struct that holds options for the WaitForEvent function. Options will update this struct and
// the final result is used to configure the WaitForEvent function.
type waitForEventOptions struct {
	container          string
	ignoreCurrentState bool
}

// WaitForEventOption is a function that modifies the waitForEventOptions struct. It is used to set options for the
// WaitForEvent function. The options are applied in the order they are provided.
type WaitForEventOption func(*waitForEventOptions)

// WithContainer is an option for the WaitForEvent function that specifies the container to check for events. If not
// specified, the default container is used.
func WithContainer(container string) WaitForEventOption {
	return func(options *waitForEventOptions) {
		options.container = container
	}
}

// WithoutCurrentState is an option for the WaitForEvent function that specifies whether to ignore messages about the
// current state of events. This allows for checking only events that are received as a subscription.
func WithoutCurrentState(ignoreCurrentState bool) WaitForEventOption {
	return func(options *waitForEventOptions) {
		options.ignoreCurrentState = ignoreCurrentState
	}
}

// WaitForEvent waits up to the specified timeout for an event to be received by the cloud event consumer. It returns an
// error if no event matches the provided filter within the timeout period.
//
// The startTime is the beginning of the time window to check for events and does not count towards the timeout. All
// logs between startTime and the current time plus the timeout are checked for events.
func WaitForEvent(
	eventPod *pod.Builder,
	startTime time.Time,
	timeout time.Duration,
	filter EventFilter,
	options ...WaitForEventOption) error {
	combinedOptions := waitForEventOptions{}
	for _, option := range options {
		option(&combinedOptions)
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			// Each loop we save the previous start time and set the new start time to the current time.
			// Because the new start time is before the logs are fetched, we can guarantee that no logs are
			// missed. The potential duplicates will not affect the result.
			previousStartTime := startTime
			startTime = time.Now()

			logs, err := eventPod.GetLogsWithOptions(&corev1.PodLogOptions{
				SinceTime: &metav1.Time{Time: previousStartTime},
				Container: combinedOptions.container,
			})
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to get logs starting at %s for pod: %v", previousStartTime, err)

				return false, nil
			}

			klog.V(tsparams.LogLevel).Infof("Logs: %s", string(logs))

			extractedEvents := extractEventsFromLogs(logs, combinedOptions.ignoreCurrentState)

			klog.V(tsparams.LogLevel).Infof("Extracted events: %#v\nFilter: %#v", extractedEvents, filter)

			return slices.ContainsFunc(extractedEvents, filter.Filter), nil
		})
}

// extractEventsFromLogs extracts events from the logs of either the cloud event consumer or the cloud event proxy
// containers. Rather than return errors, this function logs them and ignores the line. All lines that were able to be
// parsed into events are returned.
func extractEventsFromLogs(logs []byte, ignoreCurrentState bool) []event.Event {
	var extractedEvents []event.Event

	for line := range bytes.Lines(logs) {
		matcher := containsEventRegexp
		if ignoreCurrentState {
			matcher = containsEventNotStateRegexp
		}

		if !matcher.Match(line) {
			continue
		}

		eventJSON := extractEventRegexp.Find(line)
		if len(eventJSON) == 0 {
			continue
		}

		// The entire log message is formatted as a quoted string, but the extracted JSON does not include the
		// double quotes. They must be added before calling Unquote.
		unquotedEventJSON, err := strconv.Unquote(`"` + string(eventJSON) + `"`)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to unquote event JSON: %v", err)

			continue
		}

		// Event provides a custom function for unmarshalling JSON that handles the different field names
		// between API versions.
		var extractedEvent event.Event

		err = json.Unmarshal([]byte(unquotedEventJSON), &extractedEvent)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to unmarshal event: %v", err)

			continue
		}

		extractedEvents = append(extractedEvents, extractedEvent)
	}

	return extractedEvents
}
