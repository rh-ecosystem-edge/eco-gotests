package events

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/redhat-cne/sdk-go/pkg/event"
	"k8s.io/apimachinery/pkg/util/wait"
)

// TransitionRun is a run of consecutive events sharing the same observed value, collapsed for display.
type TransitionRun struct {
	Value   string
	Count   int
	FirstAt time.Time
	LastAt  time.Time
}

// writeRunRow writes one tab-delimited row for run to w, under label.
func writeRunRow(w io.Writer, label string, run TransitionRun) {
	if run.Count > 1 {
		fmt.Fprintf(w, "\t%s\t%s\t%dx\tfirst %s, last %s\n",
			label, run.Value, run.Count, run.FirstAt.Format(time.RFC3339), run.LastAt.Format(time.RFC3339))

		return
	}

	fmt.Fprintf(w, "\t%s\t%s\t\tat %s\n", label, run.Value, run.FirstAt.Format(time.RFC3339))
}

// renderTransitionRuns writes one tab-delimited row per run to w, labeling the first run firstLabel and
// every other run restLabel.
func renderTransitionRuns(w io.Writer, runs []TransitionRun, firstLabel, restLabel string) {
	if len(runs) == 0 {
		fmt.Fprintf(w, "\t(no matching events observed)\n")

		return
	}

	for i, run := range runs {
		label := restLabel
		if i == 0 {
			label = firstLabel
		}

		writeRunRow(w, label, run)
	}
}

// newTransitionWriter returns a tabwriter over b, 2 spaces of column padding.
func newTransitionWriter(b *strings.Builder) *tabwriter.Writer {
	return tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
}

// UnexpectedTransitionError reports WaitForEventTransitioned's own failure: `from` was observed, but a
// value other than `to` was observed before `to` was reached.
type UnexpectedTransitionError struct {
	From   ValueFilter
	FromAt time.Time
	To     ValueFilter
	Runs   []TransitionRun
}

// Error renders a diff-style report as tab-aligned columns: the desired from->to bracket, then every
// chronological run of observed values since `from`, with a repeat count.
func (err *UnexpectedTransitionError) Error() string {
	var b strings.Builder

	w := newTransitionWriter(&b)

	fmt.Fprintln(w, "unexpected transition")
	fmt.Fprintln(w, "Expected:")
	writeRunRow(w, "event initial", TransitionRun{Value: err.From.String(), FirstAt: err.FromAt})
	fmt.Fprintln(w, "\t....")
	fmt.Fprintf(w, "\tevent desired\t%s\n", err.To)
	fmt.Fprintln(w, "Got:")
	renderTransitionRuns(w, err.Runs, "event initial", "event unexpected")

	w.Flush()

	return b.String()
}

// InitialValueNotFoundError reports WaitForEventTransitioned's own failure: `from` was never observed
// anywhere in the window before the deadline.
type InitialValueNotFoundError struct {
	From    ValueFilter
	To      ValueFilter
	Start   time.Time
	Timeout time.Duration
	Runs    []TransitionRun
}

// Error renders a diff-style report as tab-aligned columns: the desired from->to bracket, then every
// chronological run actually observed in the window.
func (err *InitialValueNotFoundError) Error() string {
	var b strings.Builder

	w := newTransitionWriter(&b)

	fmt.Fprintln(w, "initial value never observed")
	fmt.Fprintf(w, "Expected:\tevent initial\t%s\t\twithin %s of %s\n",
		err.From, err.Timeout, err.Start.Format(time.RFC3339))
	fmt.Fprintln(w, "\t....")
	fmt.Fprintf(w, "\tevent desired\t%s\n", err.To)
	fmt.Fprintln(w, "Got:")
	renderTransitionRuns(w, err.Runs, "event", "event")

	w.Flush()

	return b.String()
}

// TimeoutError reports WaitForEventTransitioned's own failure: `from` was observed, but `to` was never
// reached before the deadline.
type TimeoutError struct {
	From    ValueFilter
	To      ValueFilter
	FromAt  time.Time
	Timeout time.Duration
	Runs    []TransitionRun
}

// Error renders a diff-style report as tab-aligned columns: the desired from->to bracket, then every
// chronological run observed since `from`.
func (err *TimeoutError) Error() string {
	var b strings.Builder

	w := newTransitionWriter(&b)

	fmt.Fprintln(w, "timed out waiting for transition")
	fmt.Fprintln(w, "Expected:")
	writeRunRow(w, "event initial", TransitionRun{Value: err.From.String(), FirstAt: err.FromAt})
	fmt.Fprintln(w, "\t....")
	fmt.Fprintf(w, "\tevent desired\t%s\twithin %s\n", err.To, err.Timeout)
	fmt.Fprintln(w, "Got:")
	renderTransitionRuns(w, err.Runs, "event initial", "event")

	w.Flush()

	return b.String()
}

// describeEvent renders an event's own data values for diagnostic display, independent of any filter.
func describeEvent(extractedEvent event.Event) string {
	if extractedEvent.Data == nil || len(extractedEvent.Data.Values) == 0 {
		return extractedEvent.Type
	}

	values := make([]string, 0, len(extractedEvent.Data.Values))
	for _, value := range extractedEvent.Data.Values {
		values = append(values, fmt.Sprintf("%v", value.Value))
	}

	return strings.Join(values, ", ")
}

// buildTransitionRuns collapses consecutive events sharing the same observed value into runs, in
// chronological order.
func buildTransitionRuns(events []event.Event) []TransitionRun {
	var runs []TransitionRun

	for _, extractedEvent := range events {
		value := describeEvent(extractedEvent)
		eventTime := extractedEvent.Time.Time

		if len(runs) > 0 && runs[len(runs)-1].Value == value {
			runs[len(runs)-1].Count++
			runs[len(runs)-1].LastAt = eventTime

			continue
		}

		runs = append(runs, TransitionRun{Value: value, Count: 1, FirstAt: eventTime, LastAt: eventTime})
	}

	return runs
}

// WaitForEventTransitioned watches events matching filter starting at start, and returns the timestamp
// the transition completed at.
//
// The first `from` event's own timestamp becomes fromObservedAt; everything before it is ignored. From
// fromObservedAt onward, only `to` is accepted -- any other value fails immediately with
// *UnexpectedTransitionError. Never observing `from` fails with *InitialValueNotFoundError. Observing
// `from` but never `to` before timeout fails with *TimeoutError.
func WaitForEventTransitioned(
	eventPod LogFetcher, start time.Time, timeout time.Duration, filter EventFilter, from, to ValueFilter,
) (time.Time, error) {
	collector := NewCollector(eventPod, start, filter)

	var fromObservedAt, toObservedAt *time.Time

	var result error

	var lastCollected []event.Event

	processedCount := 0

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			collected, err := collector.Poll()
			if err != nil {
				return false, nil
			}

			lastCollected = collected

			// Poll returns the full accumulated list every call; only the events new since the last
			// iteration are examined here.
			newlyCollected := collected[processedCount:]
			processedCount = len(collected)

			for _, extractedEvent := range newlyCollected {
				eventTime := extractedEvent.Time.Time

				switch {
				case fromObservedAt == nil && HasValue(from).Filter(extractedEvent):
					fromObservedAt = &eventTime
				case fromObservedAt != nil && HasValue(to).Filter(extractedEvent):
					toObservedAt = &eventTime

					return true, nil
				case fromObservedAt != nil:
					// Runs covers everything already fetched in this poll, not further polls.
					result = &UnexpectedTransitionError{
						From:   from,
						FromAt: *fromObservedAt,
						To:     to,
						Runs:   buildTransitionRuns(windowSince(collected, *fromObservedAt)),
					}

					return true, nil
				}
			}

			return false, nil
		})

	if result != nil {
		return time.Time{}, result
	}

	if pollErr != nil {
		if fromObservedAt == nil {
			return time.Time{}, &InitialValueNotFoundError{
				From:    from,
				To:      to,
				Start:   start,
				Timeout: timeout,
				Runs:    buildTransitionRuns(lastCollected),
			}
		}

		return time.Time{}, &TimeoutError{
			From:    from,
			To:      to,
			FromAt:  *fromObservedAt,
			Timeout: timeout,
			Runs:    buildTransitionRuns(windowSince(lastCollected, *fromObservedAt)),
		}
	}

	return *toObservedAt, nil
}

// windowSince returns every event at or after fromAt, in the same chronological order as events.
func windowSince(events []event.Event, fromAt time.Time) []event.Event {
	var windowed []event.Event

	for _, extractedEvent := range events {
		if !extractedEvent.Time.Time.Before(fromAt) {
			windowed = append(windowed, extractedEvent)
		}
	}

	return windowed
}
