package application_context

import (
	"testing"
	"time"
)

func TestICSCache_GetSet(t *testing.T) {
	cache := NewICSCache(10, 5*time.Minute)

	// Cache miss
	_, ok := cache.Get("https://example.com/cal.ics")
	if ok {
		t.Error("expected cache miss for new URL")
	}

	// Set and get
	cache.Set("https://example.com/cal.ics", []byte("VCALENDAR"), "", "")
	entry, ok := cache.Get("https://example.com/cal.ics")
	if !ok {
		t.Error("expected cache hit after Set")
	}
	if string(entry.Content) != "VCALENDAR" {
		t.Errorf("expected content 'VCALENDAR', got '%s'", string(entry.Content))
	}
}

func TestICSCache_IsFresh(t *testing.T) {
	cache := NewICSCache(10, 100*time.Millisecond)
	cache.Set("https://example.com/cal.ics", []byte("VCALENDAR"), "", "")

	entry, _ := cache.Get("https://example.com/cal.ics")
	if !entry.IsFresh(100 * time.Millisecond) {
		t.Error("entry should be fresh immediately after set")
	}

	time.Sleep(150 * time.Millisecond)
	entry, _ = cache.Get("https://example.com/cal.ics")
	if entry.IsFresh(100 * time.Millisecond) {
		t.Error("entry should be stale after threshold")
	}
}

func TestICSCache_LRUEviction(t *testing.T) {
	cache := NewICSCache(2, 5*time.Minute)
	cache.Set("url1", []byte("cal1"), "", "")
	cache.Set("url2", []byte("cal2"), "", "")
	cache.Set("url3", []byte("cal3"), "", "")

	_, ok := cache.Get("url1")
	if ok {
		t.Error("url1 should have been evicted")
	}
	_, ok = cache.Get("url2")
	if !ok {
		t.Error("url2 should still be in cache")
	}
	_, ok = cache.Get("url3")
	if !ok {
		t.Error("url3 should be in cache")
	}
}

func TestICSCache_ETagAndLastModified(t *testing.T) {
	cache := NewICSCache(10, 5*time.Minute)
	cache.Set("https://example.com/cal.ics", []byte("VCALENDAR"), "abc123", "Mon, 01 Jan 2026 00:00:00 GMT")

	entry, ok := cache.Get("https://example.com/cal.ics")
	if !ok {
		t.Error("expected cache hit")
	}
	if entry.ETag != "abc123" {
		t.Errorf("expected ETag 'abc123', got '%s'", entry.ETag)
	}
	if entry.LastModified != "Mon, 01 Jan 2026 00:00:00 GMT" {
		t.Errorf("expected LastModified 'Mon, 01 Jan 2026 00:00:00 GMT', got '%s'", entry.LastModified)
	}
}

func TestICSCache_LRUAccessRefresh(t *testing.T) {
	cache := NewICSCache(2, 5*time.Minute)
	cache.Set("url1", []byte("cal1"), "", "")
	cache.Set("url2", []byte("cal2"), "", "")

	// Access url1 to make it recently used
	cache.Get("url1")

	// Add url3 - should evict url2 (least recently used)
	cache.Set("url3", []byte("cal3"), "", "")

	_, ok := cache.Get("url1")
	if !ok {
		t.Error("url1 should still be in cache after being accessed")
	}
	_, ok = cache.Get("url2")
	if ok {
		t.Error("url2 should have been evicted")
	}
	_, ok = cache.Get("url3")
	if !ok {
		t.Error("url3 should be in cache")
	}
}

func TestParseICSEvents(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:test-event-1
DTSTART:20260115T100000Z
DTEND:20260115T110000Z
SUMMARY:Team Meeting
LOCATION:Room 101
DESCRIPTION:Weekly sync
END:VEVENT
BEGIN:VEVENT
UID:test-event-2
DTSTART;VALUE=DATE:20260120
DTEND;VALUE=DATE:20260121
SUMMARY:Holiday
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Check first event
	if events[0].Title != "Team Meeting" {
		t.Errorf("expected title 'Team Meeting', got '%s'", events[0].Title)
	}
	if events[0].AllDay {
		t.Error("first event should not be all-day")
	}
	if events[0].Location != "Room 101" {
		t.Errorf("expected location 'Room 101', got '%s'", events[0].Location)
	}
	if events[0].Description != "Weekly sync" {
		t.Errorf("expected description 'Weekly sync', got '%s'", events[0].Description)
	}

	// Check second event (all-day)
	if events[1].Title != "Holiday" {
		t.Errorf("expected title 'Holiday', got '%s'", events[1].Title)
	}
	if !events[1].AllDay {
		t.Error("second event should be all-day")
	}
}

func TestParseICSEvents_DateFiltering(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:jan-event
DTSTART:20260115T100000Z
DTEND:20260115T110000Z
SUMMARY:January Event
END:VEVENT
BEGIN:VEVENT
UID:feb-event
DTSTART:20260215T100000Z
DTEND:20260215T110000Z
SUMMARY:February Event
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event in January, got %d", len(events))
	}
	if events[0].Title != "January Event" {
		t.Errorf("expected 'January Event', got '%s'", events[0].Title)
	}
}

func TestParseICSEvents_EmptyCalendar(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestParseICSEvents_InvalidContent(t *testing.T) {
	icsContent := `not valid ics content`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	// The library is lenient, so it might not error on invalid content
	// but it should return empty events
	if err == nil && len(events) != 0 {
		t.Errorf("expected 0 events for invalid content, got %d", len(events))
	}
}

func TestParseICSEvents_CalendarID(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:test-event
DTSTART:20260115T100000Z
DTEND:20260115T110000Z
SUMMARY:Test Event
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "my-calendar", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].CalendarID != "my-calendar" {
		t.Errorf("expected CalendarID 'my-calendar', got '%s'", events[0].CalendarID)
	}
}

func TestParseICSEvents_TimezoneHandling(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VTIMEZONE
TZID:America/New_York
BEGIN:STANDARD
DTSTART:20071104T020000
RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
TZNAME:EST
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:20070311T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
TZNAME:EDT
END:DAYLIGHT
END:VTIMEZONE
BEGIN:VEVENT
UID:tz-event
DTSTART;TZID=America/New_York:20260115T100000
DTEND;TZID=America/New_York:20260115T110000
SUMMARY:Timezone Event
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// The event should be parsed with proper timezone handling
	// 10:00 EST = 15:00 UTC (January is EST, -5 hours)
	expectedStart := time.Date(2026, 1, 15, 15, 0, 0, 0, time.UTC)
	if !events[0].Start.Equal(expectedStart) {
		t.Errorf("expected start time %v, got %v", expectedStart, events[0].Start)
	}
}

func TestParseICSEvents_SpanningDateRange(t *testing.T) {
	// Event that spans the end of the date range
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:spanning-event
DTSTART:20260130T100000Z
DTEND:20260202T110000Z
SUMMARY:Spanning Event
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	// Event starts within range, so it should be included
	if len(events) != 1 {
		t.Errorf("expected 1 event (starts in range), got %d", len(events))
	}
}

func TestParseICSEvents_EventBeforeRange(t *testing.T) {
	// Event entirely before the date range
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:early-event
DTSTART:20251215T100000Z
DTEND:20251215T110000Z
SUMMARY:Early Event
END:VEVENT
END:VCALENDAR`

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	events, err := ParseICSEvents([]byte(icsContent), "cal-1", start, end)
	if err != nil {
		t.Fatalf("ParseICSEvents failed: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events (before range), got %d", len(events))
	}
}
