//go:build unit_test

package ptpleap

import (
	"testing"
	"time"
)

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

func TestParseAnnouncementDateInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseAnnouncementDate("not an announcement")
	if err == nil {
		t.Fatal("expected error for invalid announcement")
	}
}
