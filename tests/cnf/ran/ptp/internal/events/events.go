package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/redhat-cne/sdk-go/pkg/event"
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

// LogFetcher is the minimal capability Collector needs from a pod -- fetching its own logs. *pod.Builder
// already satisfies this; a fake implementation needs no Kubernetes plumbing at all.
type LogFetcher interface {
	GetLogsWithOptions(options *corev1.PodLogOptions) ([]byte, error)
}

// WaitForEvent waits up to the specified timeout for an event to be received by the cloud event consumer. It returns an
// error if no event matches the provided filter within the timeout period.
//
// The startTime is the beginning of the time window to check for events and does not count towards the timeout. All
// logs between startTime and the current time plus the timeout are checked for events.
func WaitForEvent(
	eventPod LogFetcher,
	startTime time.Time,
	timeout time.Duration,
	filter EventFilter,
	options ...WaitForEventOption) error {
	collector := NewCollector(eventPod, startTime, filter, options...)

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			matched, err := collector.Poll()
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to poll for events: %v", err)

				return false, nil
			}

			return len(matched) > 0, nil
		})
}

// Collector incrementally accumulates events matching its own filter from eventPod's own logs. Each Poll call
// fetches only what's new since the previous Poll (or since the collector's own start time, for the first call) and
// appends the matches to its own growing, append-only list -- it never re-fetches or re-parses logs already scraped.
type Collector struct {
	eventPod LogFetcher
	filter   EventFilter
	options  waitForEventOptions
	lastPoll time.Time
	events   []event.Event
}

// NewCollector creates a collector whose first Poll call fetches events starting from start.
func NewCollector(
	eventPod LogFetcher, start time.Time, filter EventFilter, options ...WaitForEventOption) *Collector {
	combinedOptions := waitForEventOptions{}
	for _, option := range options {
		option(&combinedOptions)
	}

	return &Collector{eventPod: eventPod, filter: filter, options: combinedOptions, lastPoll: start}
}

// Poll fetches any events new since the last Poll call, appends the ones matching the collector's own filter to its
// accumulated list, and returns everything accumulated so far.
func (collector *Collector) Poll() ([]event.Event, error) {
	previousPoll := collector.lastPoll
	collector.lastPoll = time.Now()

	logs, err := collector.eventPod.GetLogsWithOptions(&corev1.PodLogOptions{
		SinceTime: &metav1.Time{Time: previousPoll},
		Container: collector.options.container,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get logs starting at %s: %w", previousPoll, err)
	}

	klog.V(tsparams.LogLevel).Infof("Logs: %s", string(logs))

	for _, extractedEvent := range extractEventsFromLogs(logs, collector.options.ignoreCurrentState) {
		if collector.filter.Filter(extractedEvent) {
			collector.events = append(collector.events, extractedEvent)
		}
	}

	klog.V(tsparams.LogLevel).Infof("Accumulated events: %#v", collector.events)

	return collector.events, nil
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
