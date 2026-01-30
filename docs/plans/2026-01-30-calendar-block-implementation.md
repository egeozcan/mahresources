# Calendar Block Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a calendar block type that displays events from ICS files (URLs or Resources) with stale-while-revalidate caching.

**Architecture:** Backend Go block type with ICS parsing/caching + Alpine.js frontend component with month/agenda views. The pattern follows the existing table block (query data fetching, frontend caching).

**Tech Stack:** Go (`arran4/golang-ical` for ICS parsing), Alpine.js, Tailwind CSS

---

## Task 1: Add golang-ical dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the ICS parsing library**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go get github.com/arran4/golang-ical
```

**Step 2: Verify it was added**

```bash
grep golang-ical go.mod
```

Expected: Line showing `github.com/arran4/golang-ical`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang-ical dependency for calendar block"
```

---

## Task 2: Create calendar block type with validation

**Files:**
- Create: `models/block_types/calendar.go`
- Test: `models/block_types/calendar_test.go`

**Step 1: Write the failing test**

Create `models/block_types/calendar_test.go`:

```go
package block_types

import (
	"encoding/json"
	"testing"
)

func TestCalendarBlockType_Type(t *testing.T) {
	bt := CalendarBlockType{}
	if bt.Type() != "calendar" {
		t.Errorf("expected type 'calendar', got '%s'", bt.Type())
	}
}

func TestCalendarBlockType_ValidateContent(t *testing.T) {
	bt := CalendarBlockType{}

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "empty calendars is valid",
			content: `{"calendars": []}`,
			wantErr: false,
		},
		{
			name: "valid url calendar",
			content: `{"calendars": [{"id": "abc", "name": "Work", "color": "#3b82f6", "source": {"type": "url", "url": "https://example.com/cal.ics"}}]}`,
			wantErr: false,
		},
		{
			name: "valid resource calendar",
			content: `{"calendars": [{"id": "abc", "name": "Holidays", "color": "#10b981", "source": {"type": "resource", "resourceId": 123}}]}`,
			wantErr: false,
		},
		{
			name: "invalid color format",
			content: `{"calendars": [{"id": "abc", "name": "Work", "color": "red", "source": {"type": "url", "url": "https://example.com/cal.ics"}}]}`,
			wantErr: true,
		},
		{
			name: "invalid source type",
			content: `{"calendars": [{"id": "abc", "name": "Work", "color": "#fff", "source": {"type": "file"}}]}`,
			wantErr: true,
		},
		{
			name: "url source missing url",
			content: `{"calendars": [{"id": "abc", "name": "Work", "color": "#fff", "source": {"type": "url"}}]}`,
			wantErr: true,
		},
		{
			name: "resource source missing resourceId",
			content: `{"calendars": [{"id": "abc", "name": "Work", "color": "#fff", "source": {"type": "resource"}}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bt.ValidateContent(json.RawMessage(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalendarBlockType_ValidateState(t *testing.T) {
	bt := CalendarBlockType{}

	tests := []struct {
		name    string
		state   string
		wantErr bool
	}{
		{
			name:    "empty state is valid",
			state:   `{}`,
			wantErr: false,
		},
		{
			name:    "month view is valid",
			state:   `{"view": "month", "currentDate": "2026-01-01"}`,
			wantErr: false,
		},
		{
			name:    "agenda view is valid",
			state:   `{"view": "agenda"}`,
			wantErr: false,
		},
		{
			name:    "invalid view",
			state:   `{"view": "week"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bt.ValidateState(json.RawMessage(tt.state))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalendarBlockType_Defaults(t *testing.T) {
	bt := CalendarBlockType{}

	content := bt.DefaultContent()
	var c struct {
		Calendars []any `json:"calendars"`
	}
	if err := json.Unmarshal(content, &c); err != nil {
		t.Errorf("DefaultContent() returned invalid JSON: %v", err)
	}
	if len(c.Calendars) != 0 {
		t.Errorf("DefaultContent() should have empty calendars array")
	}

	state := bt.DefaultState()
	var s struct {
		View string `json:"view"`
	}
	if err := json.Unmarshal(state, &s); err != nil {
		t.Errorf("DefaultState() returned invalid JSON: %v", err)
	}
	if s.View != "month" {
		t.Errorf("DefaultState() view should be 'month', got '%s'", s.View)
	}
}

func TestCalendarBlockType_IsRegistered(t *testing.T) {
	bt := GetBlockType("calendar")
	if bt == nil {
		t.Error("calendar block type should be registered")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go test ./models/block_types/... -run TestCalendar -v
```

Expected: FAIL (CalendarBlockType not defined)

**Step 3: Write the implementation**

Create `models/block_types/calendar.go`:

```go
package block_types

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
)

// calendarContent represents the content schema for calendar blocks.
type calendarContent struct {
	Calendars []CalendarSource `json:"calendars"`
}

// CalendarSource represents a single calendar source configuration.
type CalendarSource struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Color  string       `json:"color"`
	Source SourceConfig `json:"source"`
}

// SourceConfig represents the source configuration for a calendar.
type SourceConfig struct {
	Type       string `json:"type"`
	URL        string `json:"url,omitempty"`
	ResourceID uint   `json:"resourceId,omitempty"`
}

// calendarState represents the state schema for calendar blocks.
type calendarState struct {
	View        string `json:"view"`
	CurrentDate string `json:"currentDate"`
}

// hexColorRegex matches #rgb or #rrggbb format
var hexColorRegex = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// CalendarBlockType implements BlockType for calendar content.
type CalendarBlockType struct{}

func (c CalendarBlockType) Type() string {
	return "calendar"
}

func (c CalendarBlockType) ValidateContent(content json.RawMessage) error {
	var cc calendarContent
	if err := json.Unmarshal(content, &cc); err != nil {
		return err
	}

	for i, cal := range cc.Calendars {
		// Validate color format
		if cal.Color != "" && !hexColorRegex.MatchString(cal.Color) {
			return errors.New("calendar color must be in #rgb or #rrggbb format")
		}

		// Validate source type
		if cal.Source.Type != "url" && cal.Source.Type != "resource" {
			return errors.New("calendar source type must be 'url' or 'resource'")
		}

		// Validate URL source
		if cal.Source.Type == "url" {
			if cal.Source.URL == "" {
				return errors.New("calendar url source requires a url")
			}
			if _, err := url.ParseRequestURI(cal.Source.URL); err != nil {
				return errors.New("calendar url is not valid: " + err.Error())
			}
		}

		// Validate resource source
		if cal.Source.Type == "resource" {
			if cal.Source.ResourceID == 0 {
				return errors.New("calendar resource source requires a resourceId")
			}
		}

		_ = i // Avoid unused variable warning
	}

	return nil
}

func (c CalendarBlockType) ValidateState(state json.RawMessage) error {
	var s calendarState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}

	if s.View != "" && s.View != "month" && s.View != "agenda" {
		return errors.New("calendar view must be 'month' or 'agenda'")
	}

	return nil
}

func (c CalendarBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"calendars": []}`)
}

func (c CalendarBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"view": "month"}`)
}

func init() {
	RegisterBlockType(CalendarBlockType{})
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go test ./models/block_types/... -run TestCalendar -v
```

Expected: All tests PASS

**Step 5: Commit**

```bash
git add models/block_types/calendar.go models/block_types/calendar_test.go
git commit -m "feat(blocks): add calendar block type with validation"
```

---

## Task 3: Create ICS cache and parser utilities

**Files:**
- Create: `application_context/calendar_cache.go`
- Test: `application_context/calendar_cache_test.go`

**Step 1: Write the failing test**

Create `application_context/calendar_cache_test.go`:

```go
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

	// Should be fresh immediately
	entry, _ := cache.Get("https://example.com/cal.ics")
	if !entry.IsFresh(100 * time.Millisecond) {
		t.Error("entry should be fresh immediately after set")
	}

	// Wait for staleness
	time.Sleep(150 * time.Millisecond)
	entry, _ = cache.Get("https://example.com/cal.ics")
	if entry.IsFresh(100 * time.Millisecond) {
		t.Error("entry should be stale after threshold")
	}
}

func TestICSCache_LRUEviction(t *testing.T) {
	cache := NewICSCache(2, 5*time.Minute) // Max 2 entries

	cache.Set("url1", []byte("cal1"), "", "")
	cache.Set("url2", []byte("cal2"), "", "")
	cache.Set("url3", []byte("cal3"), "", "") // Should evict url1

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

	// Check first event (timed)
	if events[0].Title != "Team Meeting" {
		t.Errorf("expected title 'Team Meeting', got '%s'", events[0].Title)
	}
	if events[0].AllDay {
		t.Error("first event should not be all-day")
	}
	if events[0].Location != "Room 101" {
		t.Errorf("expected location 'Room 101', got '%s'", events[0].Location)
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
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go test ./application_context/... -run "TestICS|TestParse" -v
```

Expected: FAIL (NewICSCache, ParseICSEvents not defined)

**Step 3: Write the implementation**

Create `application_context/calendar_cache.go`:

```go
package application_context

import (
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
)

// ICSCacheEntry represents a cached ICS calendar.
type ICSCacheEntry struct {
	Content      []byte
	FetchedAt    time.Time
	ETag         string
	LastModified string
}

// IsFresh returns true if the entry is fresher than the given threshold.
func (e *ICSCacheEntry) IsFresh(threshold time.Duration) bool {
	return time.Since(e.FetchedAt) < threshold
}

// ICSCache provides thread-safe caching for ICS calendar content.
type ICSCache struct {
	mu         sync.RWMutex
	entries    map[string]*ICSCacheEntry
	order      []string // For LRU eviction
	maxEntries int
	ttl        time.Duration
}

// NewICSCache creates a new ICS cache with the given max entries and TTL.
func NewICSCache(maxEntries int, ttl time.Duration) *ICSCache {
	return &ICSCache{
		entries:    make(map[string]*ICSCacheEntry),
		order:      make([]string, 0),
		maxEntries: maxEntries,
		ttl:        ttl,
	}
}

// Get retrieves a cache entry by URL. Returns nil, false if not found.
func (c *ICSCache) Get(url string) (*ICSCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[url]
	return entry, ok
}

// Set stores a cache entry for the given URL.
func (c *ICSCache) Set(url string, content []byte, etag, lastModified string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If URL already exists, update it and move to end of order
	if _, exists := c.entries[url]; exists {
		c.entries[url] = &ICSCacheEntry{
			Content:      content,
			FetchedAt:    time.Now(),
			ETag:         etag,
			LastModified: lastModified,
		}
		c.moveToEnd(url)
		return
	}

	// Evict oldest if at capacity
	if len(c.entries) >= c.maxEntries && c.maxEntries > 0 {
		oldest := c.order[0]
		delete(c.entries, oldest)
		c.order = c.order[1:]
	}

	// Add new entry
	c.entries[url] = &ICSCacheEntry{
		Content:      content,
		FetchedAt:    time.Now(),
		ETag:         etag,
		LastModified: lastModified,
	}
	c.order = append(c.order, url)
}

// moveToEnd moves the given URL to the end of the LRU order.
func (c *ICSCache) moveToEnd(url string) {
	for i, u := range c.order {
		if u == url {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, url)
			return
		}
	}
}

// CalendarEvent represents a parsed calendar event.
type CalendarEvent struct {
	ID          string    `json:"id"`
	CalendarID  string    `json:"calendarId"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
}

// ParseICSEvents parses ICS content and returns events within the date range.
func ParseICSEvents(content []byte, calendarID string, start, end time.Time) ([]CalendarEvent, error) {
	cal, err := ics.ParseCalendar(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}

	var events []CalendarEvent

	for _, component := range cal.Components {
		event, ok := component.(*ics.VEvent)
		if !ok {
			continue
		}

		// Get event times
		dtstart := event.GetProperty(ics.ComponentPropertyDtStart)
		dtend := event.GetProperty(ics.ComponentPropertyDtEnd)
		if dtstart == nil {
			continue
		}

		// Parse start time
		eventStart, allDay, err := parseICSDateTime(dtstart)
		if err != nil {
			continue // Skip events with invalid dates
		}

		// Parse end time (default to start if not specified)
		var eventEnd time.Time
		if dtend != nil {
			eventEnd, _, _ = parseICSDateTime(dtend)
		} else {
			if allDay {
				eventEnd = eventStart.AddDate(0, 0, 1)
			} else {
				eventEnd = eventStart.Add(time.Hour)
			}
		}

		// Filter by date range
		if eventEnd.Before(start) || eventStart.After(end) {
			continue
		}

		// Get event properties
		uid := ""
		if prop := event.GetProperty(ics.ComponentPropertyUniqueId); prop != nil {
			uid = prop.Value
		}

		summary := ""
		if prop := event.GetProperty(ics.ComponentPropertySummary); prop != nil {
			summary = prop.Value
		}

		location := ""
		if prop := event.GetProperty(ics.ComponentPropertyLocation); prop != nil {
			location = prop.Value
		}

		description := ""
		if prop := event.GetProperty(ics.ComponentPropertyDescription); prop != nil {
			description = prop.Value
		}

		events = append(events, CalendarEvent{
			ID:          uid,
			CalendarID:  calendarID,
			Title:       summary,
			Start:       eventStart,
			End:         eventEnd,
			AllDay:      allDay,
			Location:    location,
			Description: description,
		})
	}

	return events, nil
}

// parseICSDateTime parses an ICS date/time property.
// Returns the time, whether it's an all-day event, and any error.
func parseICSDateTime(prop *ics.IANAProperty) (time.Time, bool, error) {
	value := prop.Value

	// Check if it's a DATE value (all-day event)
	if prop.ICalParameters["VALUE"] != nil && prop.ICalParameters["VALUE"][0] == "DATE" {
		t, err := time.Parse("20060102", value)
		return t, true, err
	}

	// Try parsing as full datetime
	// Try with Z suffix (UTC)
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t, false, nil
	}

	// Try without timezone (treat as UTC)
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false, nil
	}

	// Try date only (all-day)
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true, nil
	}

	return time.Time{}, false, errors.New("unable to parse date: " + value)
}
```

Add the missing imports at the top:

```go
package application_context

import (
	"errors"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
)
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go test ./application_context/... -run "TestICS|TestParse" -v
```

Expected: All tests PASS

**Step 5: Commit**

```bash
git add application_context/calendar_cache.go application_context/calendar_cache_test.go
git commit -m "feat(blocks): add ICS cache and parser for calendar block"
```

---

## Task 4: Add calendar events API endpoint

**Files:**
- Modify: `server/interfaces/block_interfaces.go`
- Modify: `server/api_handlers/block_api_handlers.go`
- Modify: `server/routes.go`
- Modify: `application_context/block_context.go`

**Step 1: Add the interface**

Add to `server/interfaces/block_interfaces.go`:

```go
// CalendarBlockEventFetcher combines block reading and resource access for calendar blocks.
type CalendarBlockEventFetcher interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	GetResource(id uint) (*models.Resource, error)
	GetCalendarEvents(blockID uint, start, end time.Time) (*CalendarEventsResponse, error)
}

// CalendarEventsResponse is the response for the calendar events API.
type CalendarEventsResponse struct {
	Events    []CalendarEvent   `json:"events"`
	Calendars []CalendarInfo    `json:"calendars"`
	CachedAt  string            `json:"cachedAt"`
}

// CalendarEvent represents a single calendar event.
type CalendarEvent struct {
	ID          string    `json:"id"`
	CalendarID  string    `json:"calendarId"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
}

// CalendarInfo represents calendar metadata.
type CalendarInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
```

Note: You'll need to add `"time"` to the imports.

**Step 2: Add handler to block_api_handlers.go**

Add at the end of `server/api_handlers/block_api_handlers.go`:

```go
// GetCalendarBlockEventsHandler returns events for a calendar block.
// Route: GET /v1/note/block/calendar/events?blockId=X&start=Y&end=Z
func GetCalendarBlockEventsHandler(ctx interfaces.CalendarBlockEventFetcher) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		blockID := uint(http_utils.GetIntQueryParameter(request, "blockId", 0))
		if blockID == 0 {
			http_utils.HandleError(errors.New("blockId is required"), writer, request, http.StatusBadRequest)
			return
		}

		startStr := request.URL.Query().Get("start")
		endStr := request.URL.Query().Get("end")
		if startStr == "" || endStr == "" {
			http_utils.HandleError(errors.New("start and end dates are required"), writer, request, http.StatusBadRequest)
			return
		}

		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			http_utils.HandleError(errors.New("invalid start date format, use YYYY-MM-DD"), writer, request, http.StatusBadRequest)
			return
		}

		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			http_utils.HandleError(errors.New("invalid end date format, use YYYY-MM-DD"), writer, request, http.StatusBadRequest)
			return
		}
		// Include the entire end day
		end = end.Add(24*time.Hour - time.Second)

		response, err := ctx.GetCalendarEvents(blockID, start, end)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}
```

**Step 3: Add route to routes.go**

Add after the table/query route (around line 135):

```go
router.Methods(http.MethodGet).Path("/v1/note/block/calendar/events").HandlerFunc(api_handlers.GetCalendarBlockEventsHandler(appContext))
```

**Step 4: Implement GetCalendarEvents in block_context.go**

Add to `application_context/block_context.go`:

```go
// Global ICS cache (shared across all calendar blocks)
var icsCache = NewICSCache(100, 30*time.Minute)

// CalendarEventsResponse is the response for the calendar events API.
type CalendarEventsResponse struct {
	Events    []CalendarEvent `json:"events"`
	Calendars []CalendarInfo  `json:"calendars"`
	CachedAt  string          `json:"cachedAt"`
}

// CalendarInfo represents calendar metadata.
type CalendarInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// calendarBlockContent represents the content schema for calendar blocks.
type calendarBlockContent struct {
	Calendars []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Color  string `json:"color"`
		Source struct {
			Type       string `json:"type"`
			URL        string `json:"url,omitempty"`
			ResourceID uint   `json:"resourceId,omitempty"`
		} `json:"source"`
	} `json:"calendars"`
}

// GetCalendarEvents fetches and parses calendar events for a calendar block.
func (ctx *MahresourcesContext) GetCalendarEvents(blockID uint, start, end time.Time) (*CalendarEventsResponse, error) {
	// Get the block
	block, err := ctx.GetBlock(blockID)
	if err != nil {
		return nil, err
	}

	if block.Type != "calendar" {
		return nil, errors.New("block is not a calendar type")
	}

	// Parse block content
	var content calendarBlockContent
	if err := json.Unmarshal(block.Content, &content); err != nil {
		return nil, err
	}

	var allEvents []CalendarEvent
	var calendars []CalendarInfo
	staleThreshold := 5 * time.Minute

	for _, cal := range content.Calendars {
		calendars = append(calendars, CalendarInfo{
			ID:    cal.ID,
			Name:  cal.Name,
			Color: cal.Color,
		})

		var icsContent []byte
		var fetchErr error

		switch cal.Source.Type {
		case "url":
			icsContent, fetchErr = ctx.fetchICSFromURL(cal.Source.URL, staleThreshold)
		case "resource":
			icsContent, fetchErr = ctx.fetchICSFromResource(cal.Source.ResourceID)
		}

		if fetchErr != nil {
			log.Printf("Warning: failed to fetch calendar %s: %v", cal.ID, fetchErr)
			continue // Skip this calendar, continue with others
		}

		events, parseErr := ParseICSEvents(icsContent, cal.ID, start, end)
		if parseErr != nil {
			log.Printf("Warning: failed to parse calendar %s: %v", cal.ID, parseErr)
			continue
		}

		allEvents = append(allEvents, events...)
	}

	// Sort events by start time
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Start.Before(allEvents[j].Start)
	})

	return &CalendarEventsResponse{
		Events:    allEvents,
		Calendars: calendars,
		CachedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// fetchICSFromURL fetches ICS content from a URL with caching.
func (ctx *MahresourcesContext) fetchICSFromURL(url string, staleThreshold time.Duration) ([]byte, error) {
	// Check cache
	if entry, ok := icsCache.Get(url); ok {
		if entry.IsFresh(staleThreshold) {
			return entry.Content, nil
		}
		// Stale but usable - trigger background refresh
		go ctx.refreshICSCache(url, entry.ETag, entry.LastModified)
		return entry.Content, nil
	}

	// Cache miss - fetch synchronously
	return ctx.fetchAndCacheICS(url, "", "")
}

// fetchAndCacheICS fetches ICS content from URL and caches it.
func (ctx *MahresourcesContext) fetchAndCacheICS(url, etag, lastModified string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add conditional headers if we have them
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Not modified - keep using cached version
	if resp.StatusCode == http.StatusNotModified {
		if entry, ok := icsCache.Get(url); ok {
			// Update timestamp to mark as fresh
			icsCache.Set(url, entry.Content, entry.ETag, entry.LastModified)
			return entry.Content, nil
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch ICS: " + resp.Status)
	}

	// Read body with size limit (1MB)
	content, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	// Cache the result
	newETag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")
	icsCache.Set(url, content, newETag, newLastModified)

	return content, nil
}

// refreshICSCache refreshes a cached ICS URL in the background.
func (ctx *MahresourcesContext) refreshICSCache(url, etag, lastModified string) {
	_, _ = ctx.fetchAndCacheICS(url, etag, lastModified)
}

// fetchICSFromResource reads ICS content from a Resource.
func (ctx *MahresourcesContext) fetchICSFromResource(resourceID uint) ([]byte, error) {
	resource, err := ctx.GetResource(resourceID)
	if err != nil {
		return nil, err
	}

	file, err := ctx.fs.Open(resource.GetCleanLocation())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}
```

Add to imports at top of file:

```go
import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"time"
	// ... existing imports
)
```

**Step 5: Run tests**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && go build --tags 'json1 fts5' && go test ./... -v
```

Expected: Build succeeds, all tests pass

**Step 6: Commit**

```bash
git add server/interfaces/block_interfaces.go server/api_handlers/block_api_handlers.go server/routes.go application_context/block_context.go
git commit -m "feat(api): add calendar block events endpoint"
```

---

## Task 5: Create frontend calendar component (basic structure)

**Files:**
- Create: `src/components/blocks/blockCalendar.js`
- Modify: `src/main.js`
- Modify: `src/components/blockEditor.js`

**Step 1: Create the component file**

Create `src/components/blocks/blockCalendar.js`:

```javascript
// src/components/blocks/blockCalendar.js
// Calendar block component with month/agenda views and stale-while-revalidate caching

// Module-level cache for calendar events
const eventCache = new Map();
const CACHE_TTL = 30000;       // 30s - data considered expired
const STALE_THRESHOLD = 5 * 60 * 1000; // 5 minutes

function getCacheKey(blockId, start, end) {
  return `${blockId}:${start}:${end}`;
}

function getCacheEntry(key) {
  return eventCache.get(key);
}

function setCacheEntry(key, data) {
  if (eventCache.size >= 50) {
    const oldestKey = eventCache.keys().next().value;
    eventCache.delete(oldestKey);
  }
  eventCache.set(key, {
    data,
    timestamp: Date.now()
  });
}

function isCacheFresh(entry) {
  return entry && (Date.now() - entry.timestamp) < STALE_THRESHOLD;
}

function isCacheStale(entry) {
  return entry && (Date.now() - entry.timestamp) >= STALE_THRESHOLD;
}

// Color palette for auto-assigning calendar colors
const COLOR_PALETTE = [
  '#3b82f6', // blue
  '#10b981', // green
  '#f59e0b', // amber
  '#ef4444', // red
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#f97316', // orange
];

export function blockCalendar(block, saveContentFn, saveStateFn, getEditMode, noteId) {
  return {
    block,
    saveContentFn,
    saveStateFn,
    getEditMode,
    noteId,

    // Calendar sources from content
    calendars: JSON.parse(JSON.stringify(block?.content?.calendars || [])),

    // View state
    view: block?.state?.view || 'month',
    currentDate: block?.state?.currentDate ? new Date(block.state.currentDate) : new Date(),

    // Events data
    events: [],
    calendarMeta: {}, // id -> {name, color}
    loading: false,
    error: null,
    isRefreshing: false,
    lastFetchTime: null,

    // Edit mode state
    newUrl: '',
    showColorPicker: null, // calendar ID being edited

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    // Current month/year for display
    get currentMonth() {
      return this.currentDate.toLocaleString('default', { month: 'long' });
    },
    get currentYear() {
      return this.currentDate.getFullYear();
    },

    // Date range for current view
    get dateRange() {
      const d = new Date(this.currentDate);
      if (this.view === 'month') {
        const start = new Date(d.getFullYear(), d.getMonth(), 1);
        const end = new Date(d.getFullYear(), d.getMonth() + 1, 0);
        return { start, end };
      } else {
        // Agenda: next 30 days
        const start = new Date();
        start.setHours(0, 0, 0, 0);
        const end = new Date(start);
        end.setDate(end.getDate() + 30);
        return { start, end };
      }
    },

    // Format date for API
    formatDate(date) {
      return date.toISOString().split('T')[0];
    },

    async init() {
      // Build calendar metadata map
      this.calendars.forEach(cal => {
        this.calendarMeta[cal.id] = { name: cal.name, color: cal.color };
      });

      if (this.calendars.length > 0) {
        await this.fetchEvents();
      }
    },

    async fetchEvents(forceRefresh = false) {
      if (this.calendars.length === 0) {
        this.events = [];
        return;
      }

      const { start, end } = this.dateRange;
      const cacheKey = getCacheKey(this.block.id, this.formatDate(start), this.formatDate(end));
      const cacheEntry = getCacheEntry(cacheKey);

      if (!forceRefresh) {
        if (isCacheFresh(cacheEntry)) {
          this.applyEventData(cacheEntry.data);
          return;
        }
        if (isCacheStale(cacheEntry)) {
          this.applyEventData(cacheEntry.data);
          this.backgroundRefresh(cacheKey, start, end);
          return;
        }
      }

      // Cache miss or force refresh
      this.loading = true;
      this.error = null;

      try {
        const data = await this.fetchFromServer(start, end);
        setCacheEntry(cacheKey, data);
        this.applyEventData(data);
      } catch (err) {
        this.error = err.message || 'Failed to load events';
        console.error('Calendar fetch error:', err);
      } finally {
        this.loading = false;
      }
    },

    async backgroundRefresh(cacheKey, start, end) {
      if (this.isRefreshing) return;
      this.isRefreshing = true;
      try {
        const data = await this.fetchFromServer(start, end);
        setCacheEntry(cacheKey, data);
        this.applyEventData(data);
      } catch (err) {
        console.error('Background refresh failed:', err);
      } finally {
        this.isRefreshing = false;
      }
    },

    async fetchFromServer(start, end) {
      const params = new URLSearchParams({
        blockId: this.block.id,
        start: this.formatDate(start),
        end: this.formatDate(end)
      });
      const response = await fetch(`/v1/note/block/calendar/events?${params}`);
      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || `HTTP ${response.status}`);
      }
      return response.json();
    },

    applyEventData(data) {
      this.events = data.events || [];
      this.lastFetchTime = data.cachedAt ? new Date(data.cachedAt) : new Date();
      // Update calendar metadata
      (data.calendars || []).forEach(cal => {
        this.calendarMeta[cal.id] = { name: cal.name, color: cal.color };
      });
    },

    // Navigation
    prevMonth() {
      const d = new Date(this.currentDate);
      d.setMonth(d.getMonth() - 1);
      this.currentDate = d;
      this.saveState();
      this.fetchEvents();
    },

    nextMonth() {
      const d = new Date(this.currentDate);
      d.setMonth(d.getMonth() + 1);
      this.currentDate = d;
      this.saveState();
      this.fetchEvents();
    },

    setView(v) {
      this.view = v;
      this.saveState();
      this.fetchEvents();
    },

    saveState() {
      this.saveStateFn(this.block.id, {
        view: this.view,
        currentDate: this.currentDate.toISOString().split('T')[0]
      });
    },

    saveContent() {
      this.saveContentFn(this.block.id, { calendars: this.calendars });
    },

    // Calendar management
    addCalendarFromUrl() {
      if (!this.newUrl.trim()) return;
      const id = crypto.randomUUID();
      const colorIndex = this.calendars.length % COLOR_PALETTE.length;
      const newCal = {
        id,
        name: 'Calendar ' + (this.calendars.length + 1),
        color: COLOR_PALETTE[colorIndex],
        source: { type: 'url', url: this.newUrl.trim() }
      };
      this.calendars.push(newCal);
      this.calendarMeta[id] = { name: newCal.name, color: newCal.color };
      this.newUrl = '';
      this.saveContent();
      this.fetchEvents(true);
    },

    addCalendarFromResource(resourceId, resourceName) {
      const id = crypto.randomUUID();
      const colorIndex = this.calendars.length % COLOR_PALETTE.length;
      const newCal = {
        id,
        name: resourceName || 'Calendar ' + (this.calendars.length + 1),
        color: COLOR_PALETTE[colorIndex],
        source: { type: 'resource', resourceId }
      };
      this.calendars.push(newCal);
      this.calendarMeta[id] = { name: newCal.name, color: newCal.color };
      this.saveContent();
      this.fetchEvents(true);
    },

    removeCalendar(calId) {
      this.calendars = this.calendars.filter(c => c.id !== calId);
      delete this.calendarMeta[calId];
      this.saveContent();
      this.fetchEvents(true);
    },

    updateCalendarName(calId, name) {
      const cal = this.calendars.find(c => c.id === calId);
      if (cal) {
        cal.name = name;
        this.calendarMeta[calId].name = name;
        this.saveContent();
      }
    },

    updateCalendarColor(calId, color) {
      const cal = this.calendars.find(c => c.id === calId);
      if (cal) {
        cal.color = color;
        this.calendarMeta[calId].color = color;
        this.saveContent();
      }
      this.showColorPicker = null;
    },

    openResourcePicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'resource',
        noteId: this.noteId,
        existingIds: [],
        contentTypeFilter: 'text/calendar',
        onConfirm: (selectedIds) => {
          // Fetch resource info and add
          selectedIds.forEach(async (id) => {
            try {
              const res = await fetch(`/v1/resource?id=${id}`);
              if (res.ok) {
                const resource = await res.json();
                this.addCalendarFromResource(id, resource.Name);
              }
            } catch (err) {
              console.error('Failed to fetch resource:', err);
            }
          });
        }
      });
    },

    // Month view helpers
    get monthDays() {
      const { start, end } = this.dateRange;
      const days = [];
      const firstDay = start.getDay(); // 0-6

      // Pad with previous month days
      for (let i = 0; i < firstDay; i++) {
        const d = new Date(start);
        d.setDate(d.getDate() - (firstDay - i));
        days.push({ date: d, isCurrentMonth: false });
      }

      // Current month days
      for (let d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
        days.push({ date: new Date(d), isCurrentMonth: true });
      }

      // Pad to complete weeks
      while (days.length % 7 !== 0) {
        const lastDate = days[days.length - 1].date;
        const d = new Date(lastDate);
        d.setDate(d.getDate() + 1);
        days.push({ date: d, isCurrentMonth: false });
      }

      return days;
    },

    getEventsForDay(date) {
      const dayStart = new Date(date);
      dayStart.setHours(0, 0, 0, 0);
      const dayEnd = new Date(date);
      dayEnd.setHours(23, 59, 59, 999);

      return this.events.filter(e => {
        const eventStart = new Date(e.start);
        const eventEnd = new Date(e.end);
        return eventStart <= dayEnd && eventEnd >= dayStart;
      });
    },

    isToday(date) {
      const today = new Date();
      return date.getDate() === today.getDate() &&
             date.getMonth() === today.getMonth() &&
             date.getFullYear() === today.getFullYear();
    },

    // Agenda view helpers
    get agendaEvents() {
      // Group events by date
      const groups = {};
      this.events.forEach(e => {
        const dateKey = new Date(e.start).toDateString();
        if (!groups[dateKey]) {
          groups[dateKey] = { date: new Date(e.start), events: [] };
        }
        groups[dateKey].events.push(e);
      });
      return Object.values(groups).sort((a, b) => a.date - b.date);
    },

    formatEventTime(event) {
      if (event.allDay) return 'All day';
      const start = new Date(event.start);
      return start.toLocaleTimeString('default', { hour: 'numeric', minute: '2-digit' });
    },

    formatAgendaDate(date) {
      return date.toLocaleDateString('default', { weekday: 'short', month: 'short', day: 'numeric' });
    },

    getCalendarColor(calId) {
      return this.calendarMeta[calId]?.color || '#6b7280';
    },

    getCalendarName(calId) {
      return this.calendarMeta[calId]?.name || 'Unknown';
    }
  };
}
```

**Step 2: Update main.js to import the component**

Add to imports section in `src/main.js`:

```javascript
import { blockCalendar } from './components/blocks/blockCalendar.js';
```

Add to the Alpine.data registrations:

```javascript
Alpine.data('blockCalendar', blockCalendar);
```

**Step 3: Update blockEditor.js icon mapping**

In `src/components/blockEditor.js`, update `_getIconForType`:

```javascript
_getIconForType(type) {
  const icons = {
    text: '📝',
    heading: '🔤',
    divider: '──',
    gallery: '🖼️',
    references: '📁',
    todos: '☑️',
    table: '📊',
    calendar: '📅'
  };
  return icons[type] || '📦';
},
```

**Step 4: Build and verify**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && npm run build-js
```

Expected: Build succeeds without errors

**Step 5: Commit**

```bash
git add src/components/blocks/blockCalendar.js src/main.js src/components/blockEditor.js
git commit -m "feat(frontend): add calendar block Alpine.js component"
```

---

## Task 6: Add calendar block template

**Files:**
- Modify: `templates/partials/blockEditor.tpl`

**Step 1: Add the calendar block template section**

Add after the table block template (around line 490, before the empty state div):

```html
{# Calendar block #}
<template x-if="block.type === 'calendar'">
    <div x-data="blockCalendar(block, (id, content) => updateBlockContent(id, content), (id, state) => updateBlockState(id, state), () => editMode, noteId)" x-init="init()">
        {# View mode #}
        <template x-if="!editMode">
            <div class="calendar-block">
                {# Header #}
                <div class="flex items-center justify-between mb-4">
                    <div class="flex items-center gap-2">
                        <button @click="prevMonth()" class="p-1 hover:bg-gray-100 rounded" title="Previous">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
                            </svg>
                        </button>
                        <span class="text-lg font-semibold" x-text="currentMonth + ' ' + currentYear"></span>
                        <button @click="nextMonth()" class="p-1 hover:bg-gray-100 rounded" title="Next">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                            </svg>
                        </button>
                    </div>
                    <div class="flex items-center gap-2">
                        <template x-if="isRefreshing">
                            <span class="text-xs text-gray-400 flex items-center">
                                <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                                </svg>
                            </span>
                        </template>
                        <div class="flex border border-gray-200 rounded overflow-hidden text-sm">
                            <button @click="setView('month')" class="px-3 py-1" :class="view === 'month' ? 'bg-blue-500 text-white' : 'bg-white hover:bg-gray-50'">Month</button>
                            <button @click="setView('agenda')" class="px-3 py-1" :class="view === 'agenda' ? 'bg-blue-500 text-white' : 'bg-white hover:bg-gray-50'">Agenda</button>
                        </div>
                    </div>
                </div>

                {# Loading #}
                <template x-if="loading && events.length === 0">
                    <div class="text-center py-8 text-gray-500">Loading events...</div>
                </template>

                {# Error #}
                <template x-if="error">
                    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm mb-4">
                        <span x-text="error"></span>
                        <button @click="fetchEvents(true)" class="ml-2 underline">Retry</button>
                    </div>
                </template>

                {# Month view #}
                <template x-if="view === 'month' && !loading">
                    <div>
                        <div class="grid grid-cols-7 gap-px bg-gray-200 rounded overflow-hidden">
                            <template x-for="day in ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']">
                                <div class="bg-gray-50 py-2 text-center text-xs font-medium text-gray-500" x-text="day"></div>
                            </template>
                            <template x-for="day in monthDays" :key="day.date.toISOString()">
                                <div class="bg-white min-h-[80px] p-1 relative"
                                     :class="{ 'bg-gray-50': !day.isCurrentMonth, 'ring-2 ring-blue-500 ring-inset': isToday(day.date) }">
                                    <span class="text-xs" :class="day.isCurrentMonth ? 'text-gray-700' : 'text-gray-400'" x-text="day.date.getDate()"></span>
                                    <div class="mt-1 space-y-0.5">
                                        <template x-for="event in getEventsForDay(day.date).slice(0, 3)" :key="event.id">
                                            <div class="text-xs px-1 py-0.5 rounded truncate cursor-pointer hover:opacity-80"
                                                 :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                                 :title="event.title + (event.location ? ' @ ' + event.location : '')"
                                                 x-text="event.allDay ? event.title : formatEventTime(event) + ' ' + event.title">
                                            </div>
                                        </template>
                                        <template x-if="getEventsForDay(day.date).length > 3">
                                            <div class="text-xs text-gray-400 px-1" x-text="'+' + (getEventsForDay(day.date).length - 3) + ' more'"></div>
                                        </template>
                                    </div>
                                </div>
                            </template>
                        </div>
                    </div>
                </template>

                {# Agenda view #}
                <template x-if="view === 'agenda' && !loading">
                    <div class="space-y-4">
                        <template x-if="agendaEvents.length === 0">
                            <div class="text-center py-8 text-gray-400">No upcoming events</div>
                        </template>
                        <template x-for="group in agendaEvents" :key="group.date.toISOString()">
                            <div>
                                <div class="text-sm font-medium text-gray-600 mb-2" x-text="formatAgendaDate(group.date)"></div>
                                <div class="space-y-2">
                                    <template x-for="event in group.events" :key="event.id">
                                        <div class="flex items-start gap-3 p-2 rounded hover:bg-gray-50">
                                            <div class="w-1 h-full min-h-[40px] rounded" :style="'background-color: ' + getCalendarColor(event.calendarId)"></div>
                                            <div class="flex-1 min-w-0">
                                                <div class="font-medium text-sm" x-text="event.title"></div>
                                                <div class="text-xs text-gray-500">
                                                    <span x-text="formatEventTime(event)"></span>
                                                    <span x-show="event.location" class="ml-2">📍 <span x-text="event.location"></span></span>
                                                </div>
                                                <div x-show="event.description" class="text-xs text-gray-400 mt-1 line-clamp-2" x-text="event.description"></div>
                                            </div>
                                            <div class="text-xs px-2 py-0.5 rounded"
                                                 :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                                 x-text="getCalendarName(event.calendarId)">
                                            </div>
                                        </div>
                                    </template>
                                </div>
                            </div>
                        </template>
                    </div>
                </template>

                {# Empty state #}
                <template x-if="calendars.length === 0 && !loading">
                    <div class="text-center py-8 text-gray-400">
                        <p>No calendars added yet.</p>
                        <p class="text-sm mt-1">Click "Edit Blocks" to add calendars.</p>
                    </div>
                </template>
            </div>
        </template>

        {# Edit mode #}
        <template x-if="editMode">
            <div class="space-y-4">
                {# Configured calendars #}
                <div>
                    <p class="text-sm font-medium text-gray-700 mb-2">Calendars</p>
                    <template x-if="calendars.length === 0">
                        <p class="text-sm text-gray-400">No calendars configured</p>
                    </template>
                    <div class="space-y-2">
                        <template x-for="cal in calendars" :key="cal.id">
                            <div class="flex items-center gap-2 p-2 bg-gray-50 rounded">
                                <div class="relative">
                                    <button @click="showColorPicker = showColorPicker === cal.id ? null : cal.id"
                                            class="w-6 h-6 rounded border border-gray-300 cursor-pointer"
                                            :style="'background-color: ' + cal.color">
                                    </button>
                                    <template x-if="showColorPicker === cal.id">
                                        <div class="absolute top-8 left-0 z-10 p-2 bg-white border rounded shadow-lg flex flex-wrap gap-1 w-32">
                                            <template x-for="color in ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316']">
                                                <button @click="updateCalendarColor(cal.id, color)"
                                                        class="w-6 h-6 rounded border"
                                                        :style="'background-color: ' + color"
                                                        :class="cal.color === color ? 'ring-2 ring-offset-1 ring-gray-400' : ''">
                                                </button>
                                            </template>
                                        </div>
                                    </template>
                                </div>
                                <input type="text" :value="cal.name"
                                       @blur="updateCalendarName(cal.id, $event.target.value)"
                                       class="flex-1 px-2 py-1 text-sm border border-gray-300 rounded">
                                <span class="text-xs text-gray-400" x-text="cal.source.type === 'url' ? '🔗 URL' : '📄 File'"></span>
                                <button @click="removeCalendar(cal.id)" class="text-red-500 hover:text-red-700 p-1" title="Remove">
                                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                                    </svg>
                                </button>
                            </div>
                        </template>
                    </div>
                </div>

                {# Add calendar #}
                <div class="pt-2 border-t border-gray-200">
                    <p class="text-sm font-medium text-gray-700 mb-2">Add Calendar</p>
                    <div class="space-y-2">
                        <div class="flex gap-2">
                            <input type="url" x-model="newUrl"
                                   @keydown.enter="addCalendarFromUrl()"
                                   placeholder="Paste ICS calendar URL..."
                                   class="flex-1 px-3 py-2 text-sm border border-gray-300 rounded">
                            <button @click="addCalendarFromUrl()"
                                    :disabled="!newUrl.trim()"
                                    class="px-3 py-2 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed">
                                Add URL
                            </button>
                        </div>
                        <button @click="openResourcePicker()"
                                class="w-full py-2 px-4 border-2 border-dashed border-gray-300 rounded-lg text-gray-500 hover:border-blue-400 hover:text-blue-500 transition-colors text-sm">
                            + Select ICS File from Resources
                        </button>
                    </div>
                </div>
            </div>
        </template>
    </div>
</template>
```

**Step 2: Build and verify**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && npm run build
```

Expected: Build succeeds

**Step 3: Commit**

```bash
git add templates/partials/blockEditor.tpl
git commit -m "feat(frontend): add calendar block template"
```

---

## Task 7: Write E2E tests

**Files:**
- Create: `e2e/tests/19-block-calendar.spec.ts`

**Step 1: Create the test file**

Create `e2e/tests/19-block-calendar.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Calendar Block', () => {
  test.beforeEach(async ({ api }) => {
    // Create a test note
    await api.createNote({ name: 'Calendar Test Note' });
  });

  test('can add calendar block', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');

    // Enter edit mode
    await page.click('button:has-text("Edit Blocks")');

    // Add calendar block
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Verify block was added
    await expect(page.locator('text=No calendars configured')).toBeVisible();
  });

  test('can add calendar from URL', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Add a calendar URL
    const testUrl = 'https://calendar.google.com/calendar/ical/en.usa%23holiday%40group.v.calendar.google.com/public/basic.ics';
    await page.fill('input[placeholder*="ICS calendar URL"]', testUrl);
    await page.click('button:has-text("Add URL")');

    // Verify calendar was added
    await expect(page.locator('text=Calendar 1')).toBeVisible();
    await expect(page.locator('text=🔗 URL')).toBeVisible();
  });

  test('can switch between month and agenda views', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Exit edit mode
    await page.click('button:has-text("Done")');

    // Should start in month view
    await expect(page.locator('text=Sun')).toBeVisible();
    await expect(page.locator('text=Mon')).toBeVisible();

    // Switch to agenda view
    await page.click('button:has-text("Agenda")');
    await expect(page.locator('text=No upcoming events')).toBeVisible();

    // Switch back to month view
    await page.click('button:has-text("Month")');
    await expect(page.locator('text=Sun')).toBeVisible();
  });

  test('can navigate between months', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');
    await page.click('button:has-text("Done")');

    // Get current month
    const currentMonth = new Date().toLocaleString('default', { month: 'long' });

    // Navigate to next month
    await page.click('button[title="Next"]');

    // Month should have changed
    const nextMonth = new Date();
    nextMonth.setMonth(nextMonth.getMonth() + 1);
    const nextMonthName = nextMonth.toLocaleString('default', { month: 'long' });
    await expect(page.locator(`text=${nextMonthName}`)).toBeVisible();

    // Navigate back
    await page.click('button[title="Previous"]');
    await expect(page.locator(`text=${currentMonth}`)).toBeVisible();
  });

  test('can remove calendar', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Add a calendar
    await page.fill('input[placeholder*="ICS calendar URL"]', 'https://example.com/cal.ics');
    await page.click('button:has-text("Add URL")');
    await expect(page.locator('text=Calendar 1')).toBeVisible();

    // Remove the calendar
    await page.click('button[title="Remove"]');
    await expect(page.locator('text=No calendars configured')).toBeVisible();
  });

  test('can change calendar color', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Add a calendar
    await page.fill('input[placeholder*="ICS calendar URL"]', 'https://example.com/cal.ics');
    await page.click('button:has-text("Add URL")');

    // Click the color swatch to open picker
    await page.click('.w-6.h-6.rounded.border');

    // Select a different color (red)
    await page.click('button[style*="background-color: rgb(239, 68, 68)"]');

    // Verify color changed
    const colorSwatch = page.locator('.w-6.h-6.rounded.border').first();
    await expect(colorSwatch).toHaveCSS('background-color', 'rgb(239, 68, 68)');
  });

  test('can rename calendar', async ({ page }) => {
    await page.goto('/notes');
    await page.click('text=Calendar Test Note');
    await page.click('button:has-text("Edit Blocks")');
    await page.click('button:has-text("+ Add Block")');
    await page.click('button:has-text("Calendar")');

    // Add a calendar
    await page.fill('input[placeholder*="ICS calendar URL"]', 'https://example.com/cal.ics');
    await page.click('button:has-text("Add URL")');

    // Rename it
    const nameInput = page.locator('input[value="Calendar 1"]');
    await nameInput.fill('My Work Calendar');
    await nameInput.blur();

    // Verify name changed
    await expect(page.locator('input[value="My Work Calendar"]')).toBeVisible();
  });
});
```

**Step 2: Run tests**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block/e2e && npm run test:with-server -- --grep "Calendar Block"
```

Expected: Tests pass (or identify issues to fix)

**Step 3: Commit**

```bash
git add e2e/tests/19-block-calendar.spec.ts
git commit -m "test(e2e): add calendar block tests"
```

---

## Task 8: Final integration testing and cleanup

**Step 1: Build everything**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/calendar-block && npm run build
```

**Step 2: Run all tests**

```bash
go test ./... && cd e2e && npm run test:with-server
```

Expected: All tests pass

**Step 3: Manual testing**

Start the server and test manually:

```bash
./mahresources -ephemeral -bind-address=:8181
```

Test checklist:
- [ ] Create note, add calendar block
- [ ] Add calendar via URL
- [ ] Verify events display in month view
- [ ] Switch to agenda view
- [ ] Navigate between months
- [ ] Change calendar color
- [ ] Rename calendar
- [ ] Remove calendar
- [ ] Add multiple calendars

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat(blocks): complete calendar block implementation"
```

---

## Summary

The implementation adds:
1. **Backend**: Calendar block type with content/state validation
2. **Backend**: ICS cache with stale-while-revalidate pattern
3. **Backend**: Events API endpoint (`/v1/note/block/calendar/events`)
4. **Frontend**: Alpine.js component with month/agenda views
5. **Frontend**: Template with full edit/view mode UI
6. **Tests**: Unit tests for parsing/caching, E2E tests for UI

Total: 8 tasks, each with 4-6 steps.
