//go:build unit_test

package ptpleap

import (
	"testing"
	"time"
)

// TestParseAnnouncementDate verifies ParseAnnouncementDate on a valid announcement line.
func TestParseAnnouncementDate(t *testing.T) {
	t.Parallel()

	got, err := ParseAnnouncementDate("3692217600     37    # 1 Jan 2017")
	if err != nil {
		t.Fatalf("ParseAnnouncementDate: %v", err)
	}

	want := time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestGetLastAnnouncementBeforeHashLine verifies the last announcement is found when #h follows immediately.
func TestGetLastAnnouncementBeforeHashLine(t *testing.T) {
	t.Parallel()

	data := "# Do not edit\n3644697600     36    # 1 Jul 2015\n3692217600     37    # 1 Jan 2017\n#h\te65754d4"

	got, err := GetLastAnnouncement(data)
	if err != nil {
		t.Fatalf("GetLastAnnouncement: %v", err)
	}

	want := "3692217600     37    # 1 Jan 2017"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestGetLastAnnouncementWithBlankLineBeforeHash verifies the last announcement when a blank line precedes #h.
func TestGetLastAnnouncementWithBlankLineBeforeHash(t *testing.T) {
	t.Parallel()

	data := "3644697600     36    # 1 Jul 2015\n3692217600     37    # 1 Jan 2017\n\n#h\te65754d4"

	got, err := GetLastAnnouncement(data)
	if err != nil {
		t.Fatalf("GetLastAnnouncement: %v", err)
	}

	want := "3692217600     37    # 1 Jan 2017"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestParseAnnouncementDateInvalid verifies ParseAnnouncementDate rejects malformed input.
func TestParseAnnouncementDateInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseAnnouncementDate("not an announcement")
	if err == nil {
		t.Fatal("expected error for invalid announcement")
	}
}
