package block_types

import (
	"encoding/json"
	"testing"
)

// B7: an event whose end precedes its start (well-formed RFC3339) must be
// rejected rather than silently saved and then dropped on render.
func TestCalendar_ValidateState_RejectsInvertedTimedEvent(t *testing.T) {
	state := json.RawMessage(`{"view":"month","customEvents":[{"id":"e1","title":"x","start":"2026-06-20T10:00:00Z","end":"2026-06-20T09:00:00Z","allDay":false,"calendarId":"custom"}]}`)
	if err := (CalendarBlockType{}).ValidateState(state); err == nil {
		t.Fatal("expected inverted timed event (end before start) to be rejected")
	}
}

func TestCalendar_ValidateState_AcceptsValidEvent(t *testing.T) {
	state := json.RawMessage(`{"view":"month","customEvents":[{"id":"e1","title":"x","start":"2026-06-20T09:00:00Z","end":"2026-06-20T10:00:00Z","allDay":false,"calendarId":"custom"}]}`)
	if err := (CalendarBlockType{}).ValidateState(state); err != nil {
		t.Fatalf("valid event must be accepted, got %v", err)
	}
}

// D2: a calendar URL source must use http(s); other schemes are rejected so the
// server-side ICS fetch cannot be pointed at non-http(s) targets.
func TestCalendar_ValidateContent_RejectsNonHttpScheme(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "gopher://x/", "ftp://x/cal.ics"} {
		content := json.RawMessage(`{"calendars":[{"id":"c1","source":{"type":"url","url":"` + bad + `"}}]}`)
		if err := (CalendarBlockType{}).ValidateContent(content); err == nil {
			t.Fatalf("expected scheme %q to be rejected", bad)
		}
	}
	ok := json.RawMessage(`{"calendars":[{"id":"c1","source":{"type":"url","url":"https://example.com/cal.ics"}}]}`)
	if err := (CalendarBlockType{}).ValidateContent(ok); err != nil {
		t.Fatalf("https URL must be accepted, got %v", err)
	}
}
