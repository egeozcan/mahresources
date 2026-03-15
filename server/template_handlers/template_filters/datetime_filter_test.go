package template_filters

import (
	"testing"
	"time"

	"github.com/flosch/pongo2/v4"
)

func TestFilterDateTimeUses24HourFormat(t *testing.T) {
	// 3:30 PM = 15:30 in 24-hour format
	ts := time.Date(2025, 6, 15, 15, 30, 0, 0, time.UTC)

	result, err := filterDateTime(pongo2.AsValue(&ts), pongo2.AsValue(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.String()
	expected := "2025-06-15T15:30"

	if got != expected {
		t.Errorf("filterDateTime(15:30 PM) = %q, want %q (24-hour format for datetime-local input)", got, expected)
	}
}

func TestFilterDateTimePreservesMorningTime(t *testing.T) {
	// 8:45 AM should stay "08:45" in both 12h and 24h, but let's confirm
	ts := time.Date(2025, 1, 5, 8, 45, 0, 0, time.UTC)

	result, _ := filterDateTime(pongo2.AsValue(&ts), pongo2.AsValue(""))
	got := result.String()
	expected := "2025-01-05T08:45"

	if got != expected {
		t.Errorf("filterDateTime(8:45 AM) = %q, want %q", got, expected)
	}
}

func TestFilterDateTimeNilReturnsEmpty(t *testing.T) {
	result, _ := filterDateTime(pongo2.AsValue(nil), pongo2.AsValue(""))
	if result.String() != "" {
		t.Errorf("filterDateTime(nil) = %q, want empty string", result.String())
	}
}
