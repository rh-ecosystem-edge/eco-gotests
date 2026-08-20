//go:build unit_test

package events_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/redhat-cne/sdk-go/pkg/event"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/redhat-cne/sdk-go/pkg/types"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"
)

// fakeLogFetcher serves canned pod logs directly, with no Kubernetes plumbing at all -- exactly the
// point of extracting LogFetcher: GetLogsWithOptions never touches a real or fake clientset.
type fakeLogFetcher struct {
	logs []byte
}

func (f *fakeLogFetcher) GetLogsWithOptions(*corev1.PodLogOptions) ([]byte, error) {
	return f.logs, nil
}

// syncStateEvent builds a real event.Event and renders it into the exact log-line shape
// extractEventsFromLogs parses: JSON marshaled via the type's own real marshaling code (so the fixture
// can never drift from what the real struct actually produces), then embedded in a msg="..." line the
// same way the daemon's own log formatter would quote it.
func syncStateEvent(t *testing.T, eventTime time.Time, nodeName string, state eventptp.SyncState) string {
	t.Helper()

	extractedEvent := event.Event{
		ID:              "test",
		Type:            string(eventptp.PtpStateChange),
		DataContentType: event.StringOfApplicationJSON(),
		Time:            &types.Timestamp{Time: eventTime},
		Data: &event.Data{
			Version: "1.0",
			Values: []event.DataValue{{
				Resource:  fmt.Sprintf("/cluster/node/%s/sync/ptp-status/lock-state", nodeName),
				DataType:  event.NOTIFICATION,
				ValueType: event.ENUMERATION,
				Value:     string(state),
			}},
		},
	}

	payload, err := json.Marshal(extractedEvent)
	require.NoError(t, err)

	escaped := strings.ReplaceAll(string(payload), `"`, `\"`)

	return fmt.Sprintf(`time="%s" level=info msg="received event %s"`, eventTime.Format(time.RFC3339), escaped)
}

func TestWaitForEventTransitioned_CleanTransition_ReturnsToObservedAt(t *testing.T) {
	base := time.Now()
	holdoverAt := base
	lockedAt := base.Add(2 * time.Second)

	fetcher := &fakeLogFetcher{logs: []byte(strings.Join([]string{
		syncStateEvent(t, holdoverAt, "master-0", eventptp.HOLDOVER),
		syncStateEvent(t, lockedAt, "master-0", eventptp.LOCKED),
	}, "\n"))}

	filter := events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.OnNode("master-0")))

	observedAt, err := events.WaitForEventTransitioned(fetcher, base, 5*time.Second, filter,
		events.WithSyncState(eventptp.HOLDOVER), events.WithSyncState(eventptp.LOCKED))
	require.NoError(t, err)
	require.WithinDuration(t, lockedAt, observedAt, time.Second)
}

// TestWaitForEventTransitioned_SelfRecoveringUnexpectedValue_StillFails drives the real polling loop (not
// just the pure helpers) through a timeline lifted from a real regression: HOLDOVER sustained, then a
// spurious FREERUN before the daemon self-recovers to LOCKED a few seconds later -- the transition must
// fail the moment FREERUN is observed, not silently succeed once LOCKED eventually shows up.
// Jira: https://redhat.atlassian.net/browse/OCPBUGS-90101
func TestWaitForEventTransitioned_SelfRecoveringUnexpectedValue_StillFails(t *testing.T) {
	base := time.Now()
	holdoverAt := base
	freerunAt := base.Add(43 * time.Second)
	selfRecoveredLockedAt := base.Add(47 * time.Second)

	fetcher := &fakeLogFetcher{logs: []byte(strings.Join([]string{
		syncStateEvent(t, holdoverAt, "master-0", eventptp.HOLDOVER),
		syncStateEvent(t, freerunAt, "master-0", eventptp.FREERUN),
		syncStateEvent(t, selfRecoveredLockedAt, "master-0", eventptp.LOCKED),
	}, "\n"))}

	filter := events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.OnNode("master-0")))

	_, err := events.WaitForEventTransitioned(fetcher, base, 5*time.Second, filter,
		events.WithSyncState(eventptp.HOLDOVER), events.WithSyncState(eventptp.LOCKED))

	// All 3 events arrive in a single fetched batch here (the fake returns the whole log at once), so the
	// diagnostic correctly includes the self-recovery to LOCKED that the real regression's own report
	// describes -- everything already fetched at the point of failure, not just up to the first bad value.
	var unexpected *events.UnexpectedTransitionError
	require.ErrorAs(t, err, &unexpected)
	require.Len(t, unexpected.Runs, 3)
	require.Equal(t,
		"unexpected transition\n"+
			"Expected:\n"+
			"  event initial  HOLDOVER    at "+holdoverAt.UTC().Format(time.RFC3339)+"\n"+
			"  ....\n"+
			"  event desired  LOCKED\n"+
			"Got:\n"+
			"  event initial     HOLDOVER    at "+holdoverAt.UTC().Format(time.RFC3339)+"\n"+
			"  event unexpected  FREERUN     at "+freerunAt.UTC().Format(time.RFC3339)+"\n"+
			"  event unexpected  LOCKED      at "+selfRecoveredLockedAt.UTC().Format(time.RFC3339)+"\n",
		err.Error())
	require.Equal(t, "HOLDOVER", unexpected.Runs[0].Value)
	require.Equal(t, "FREERUN", unexpected.Runs[1].Value)
	require.Equal(t, "LOCKED", unexpected.Runs[2].Value)
}

func TestWaitForEventTransitioned_FromNeverObserved_Errors(t *testing.T) {
	base := time.Now()
	fetcher := &fakeLogFetcher{logs: []byte(syncStateEvent(t, base, "master-0", eventptp.LOCKED))}

	filter := events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.OnNode("master-0")))

	_, err := events.WaitForEventTransitioned(fetcher, base, time.Second, filter,
		events.WithSyncState(eventptp.HOLDOVER), events.WithSyncState(eventptp.LOCKED))

	var notFound *events.InitialValueNotFoundError
	require.ErrorAs(t, err, &notFound)
	require.Len(t, notFound.Runs, 1)
	require.Equal(t, "LOCKED", notFound.Runs[0].Value)
}

func TestWaitForEventTransitioned_ToNeverReached_TimesOut(t *testing.T) {
	base := time.Now()
	fetcher := &fakeLogFetcher{logs: []byte(syncStateEvent(t, base, "master-0", eventptp.HOLDOVER))}

	filter := events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.OnNode("master-0")))

	_, err := events.WaitForEventTransitioned(fetcher, base, time.Second, filter,
		events.WithSyncState(eventptp.HOLDOVER), events.WithSyncState(eventptp.LOCKED))

	var timedOut *events.TimeoutError
	require.ErrorAs(t, err, &timedOut)
	require.Len(t, timedOut.Runs, 1)
	require.Equal(t, "HOLDOVER", timedOut.Runs[0].Value)
}
