package mocksmo

import (
	"testing"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserverCallbackURL(t *testing.T) {
	t.Parallel()

	baseURL := "https://smo.example.com"
	observerID := "550e8400-e29b-41d4-a716-446655440000"

	got := ObserverCallbackURL(baseURL, observerID)
	want := "https://smo.example.com/mock_smo/v1/observers/550e8400-e29b-41d4-a716-446655440000"

	assert.Equal(t, want, got)
}

func TestParseNotifications(t *testing.T) {
	t.Parallel()

	// Matches oran-mock-smo slog.NewJSONHandler output on stdout. Only "Observer echo notification" lines are parsed.
	logs := []byte(
		`{"time":"2026-07-28T12:25:09.502177138Z","level":"DEBUG","msg":"Request completed",` +
			`"method":"POST","url":"/mock_smo/v1/o2ims_alarms_observer","status":204,"duration":"18.528612ms"}` + "\n" +
			`{"time":"2026-07-28T12:25:10.000000000Z","level":"INFO","msg":"Observer echo notification",` +
			`"observerId":"sub-1","body":{"notificationEventType":3,"extensions":{"tracker":"abc"},` +
			`"alarmAcknowledged":false}}` + "\n" +
			`{"time":"2026-07-28T12:25:12.000000000Z","level":"INFO","msg":"Observer echo notification",` +
			`"observerId":"sub-2","body":null}` + "\n",
	)

	notifications, err := parseNotifications(logs, "")
	require.NoError(t, err)
	require.Len(t, notifications, 1)

	notification := notifications[0]
	assert.Equal(t, oranapi.AlarmEventNotificationTypeACKNOWLEDGE, notification.NotificationEventType)
	assert.Equal(t, "abc", notification.Extensions["tracker"])
}

func TestParseNotificationsFiltersByObserverID(t *testing.T) {
	t.Parallel()

	logs := []byte(
		`{"time":"2026-07-28T12:25:10.000000000Z","level":"INFO","msg":"Observer echo notification",` +
			`"observerId":"wanted","body":{"notificationEventType":0,"extensions":{"tracker":"one"}}}` + "\n" +
			`{"time":"2026-07-28T12:25:11.000000000Z","level":"INFO","msg":"Observer echo notification",` +
			`"observerId":"other","body":{"notificationEventType":0,"extensions":{"tracker":"two"}}}` + "\n",
	)

	notifications, err := parseNotifications(logs, "wanted")
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	assert.Equal(t, "one", notifications[0].Extensions["tracker"])
}
