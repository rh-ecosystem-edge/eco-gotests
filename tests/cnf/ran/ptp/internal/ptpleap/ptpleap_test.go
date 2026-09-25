package ptpleap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseAnnouncementDate verifies ParseAnnouncementDate on a valid announcement line.
func TestParseAnnouncementDate(t *testing.T) {
	t.Parallel()

	got, err := ParseAnnouncementDate("3692217600     37    # 1 Jan 2017")
	require.NoError(t, err)

	want := time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, got)
}

// TestGetLastAnnouncementBeforeHashLine verifies the last announcement is found when #h follows immediately.
func TestGetLastAnnouncementBeforeHashLine(t *testing.T) {
	t.Parallel()

	data := "# Do not edit\n3644697600     36    # 1 Jul 2015\n3692217600     37    # 1 Jan 2017\n#h\te65754d4"

	got, err := GetLastAnnouncement(data)
	require.NoError(t, err)
	assert.Equal(t, "3692217600     37    # 1 Jan 2017", got)
}

// TestGetLastAnnouncementWithBlankLineBeforeHash verifies the last announcement when a blank line precedes #h.
func TestGetLastAnnouncementWithBlankLineBeforeHash(t *testing.T) {
	t.Parallel()

	data := "3644697600     36    # 1 Jul 2015\n3692217600     37    # 1 Jan 2017\n\n#h\te65754d4"

	got, err := GetLastAnnouncement(data)
	require.NoError(t, err)
	assert.Equal(t, "3692217600     37    # 1 Jan 2017", got)
}

// TestParseAnnouncementDateInvalid verifies ParseAnnouncementDate rejects malformed input.
func TestParseAnnouncementDateInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseAnnouncementDate("not an announcement")
	require.Error(t, err)
}
