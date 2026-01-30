package application_context

import (
	"container/list"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"mahresources/server/interfaces"
)

// ICSCacheEntry represents a cached ICS file with metadata for conditional fetching
type ICSCacheEntry struct {
	Content      []byte
	FetchedAt    time.Time
	ETag         string
	LastModified string
}

// IsFresh returns true if the entry is still within the freshness threshold
func (e *ICSCacheEntry) IsFresh(threshold time.Duration) bool {
	return time.Since(e.FetchedAt) < threshold
}

// ICSCache provides an LRU cache for ICS calendar data with TTL support
type ICSCache struct {
	mu         sync.RWMutex
	entries    map[string]*list.Element
	order      *list.List
	maxEntries int
	ttl        time.Duration
}

type icsCacheItem struct {
	url   string
	entry *ICSCacheEntry
}

// NewICSCache creates a new ICS cache with the specified max entries and TTL
func NewICSCache(maxEntries int, ttl time.Duration) *ICSCache {
	return &ICSCache{
		entries:    make(map[string]*list.Element),
		order:      list.New(),
		maxEntries: maxEntries,
		ttl:        ttl,
	}
}

// Get retrieves a cached entry by URL. Returns the entry and whether it was found.
// Note: Get moves the entry to the front of the LRU list but does NOT check freshness.
// Callers should use IsFresh() to determine if the entry should be refetched.
func (c *ICSCache) Get(url string) (*ICSCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[url]
	if !ok {
		return nil, false
	}

	// Move to front (most recently used)
	c.order.MoveToFront(elem)

	item := elem.Value.(*icsCacheItem)
	return item.entry, true
}

// Set stores an ICS entry in the cache with optional ETag and LastModified headers
func (c *ICSCache) Set(url string, content []byte, etag, lastModified string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &ICSCacheEntry{
		Content:      content,
		FetchedAt:    time.Now(),
		ETag:         etag,
		LastModified: lastModified,
	}

	// Check if URL already exists
	if elem, ok := c.entries[url]; ok {
		// Update existing entry
		c.order.MoveToFront(elem)
		elem.Value.(*icsCacheItem).entry = entry
		return
	}

	// Evict oldest entries if at capacity
	for c.order.Len() >= c.maxEntries {
		c.evictOldest()
	}

	// Add new entry
	item := &icsCacheItem{url: url, entry: entry}
	elem := c.order.PushFront(item)
	c.entries[url] = elem
}

// evictOldest removes the least recently used entry (must be called with lock held)
func (c *ICSCache) evictOldest() {
	elem := c.order.Back()
	if elem != nil {
		item := elem.Value.(*icsCacheItem)
		delete(c.entries, item.url)
		c.order.Remove(elem)
	}
}

// Size returns the current number of entries in the cache
func (c *ICSCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// Clear removes all entries from the cache
func (c *ICSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*list.Element)
	c.order.Init()
}

// ParseICSEvents parses ICS content and returns events within the specified date range.
// Note: This parser does not support recurring events (RRULE). Events with RRULE will
// only show their first occurrence. Full RRULE support would require significant complexity.
func ParseICSEvents(content []byte, calendarID string, rangeStart, rangeEnd time.Time) ([]interfaces.CalendarEvent, error) {
	cal, err := ics.ParseCalendar(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}

	var events []interfaces.CalendarEvent

	for _, component := range cal.Components {
		event, ok := component.(*ics.VEvent)
		if !ok {
			continue
		}

		calEvent := parseVEvent(event, calendarID)
		if calEvent == nil {
			continue
		}

		// Filter by date range
		// Include event if it overlaps with the range at all
		if eventOverlapsRange(calEvent.Start, calEvent.End, rangeStart, rangeEnd) {
			events = append(events, *calEvent)
		}
	}

	return events, nil
}

// parseVEvent converts an ICS VEvent to a CalendarEvent
func parseVEvent(event *ics.VEvent, calendarID string) *interfaces.CalendarEvent {
	uid := event.GetProperty(ics.ComponentPropertyUniqueId)
	if uid == nil {
		return nil
	}

	summary := event.GetProperty(ics.ComponentPropertySummary)
	dtstart := event.GetProperty(ics.ComponentPropertyDtStart)
	dtend := event.GetProperty(ics.ComponentPropertyDtEnd)

	if dtstart == nil {
		return nil
	}

	startTime, allDay := parseICSDateTime(dtstart)
	if startTime.IsZero() {
		return nil
	}

	var endTime time.Time
	if dtend != nil {
		endTime, _ = parseICSDateTime(dtend)
	}
	if endTime.IsZero() {
		// Default end time to start time + 1 hour (or +1 day for all-day events)
		if allDay {
			endTime = startTime.Add(24 * time.Hour)
		} else {
			endTime = startTime.Add(time.Hour)
		}
	}

	calEvent := &interfaces.CalendarEvent{
		ID:         uid.Value,
		CalendarID: calendarID,
		Start:      startTime,
		End:        endTime,
		AllDay:     allDay,
	}

	if summary != nil {
		calEvent.Title = summary.Value
	}

	location := event.GetProperty(ics.ComponentPropertyLocation)
	if location != nil {
		calEvent.Location = location.Value
	}

	description := event.GetProperty(ics.ComponentPropertyDescription)
	if description != nil {
		calEvent.Description = description.Value
	}

	return calEvent
}

// parseICSDateTime parses an ICS date/time property and returns the time and whether it's an all-day event
func parseICSDateTime(prop *ics.IANAProperty) (time.Time, bool) {
	if prop == nil {
		return time.Time{}, false
	}

	value := prop.Value
	params := prop.ICalParameters

	// Check if it's a date-only value (all-day event)
	isAllDay := false
	if valueParam, ok := params["VALUE"]; ok && len(valueParam) > 0 {
		isAllDay = valueParam[0] == "DATE"
	}

	// Check for timezone
	var loc *time.Location = time.UTC
	if tzid, ok := params["TZID"]; ok && len(tzid) > 0 {
		if parsedLoc, err := time.LoadLocation(tzid[0]); err == nil {
			loc = parsedLoc
		}
	}

	// Try parsing various formats
	if isAllDay {
		// DATE format: YYYYMMDD
		t, err := time.ParseInLocation("20060102", value, loc)
		if err == nil {
			return t.UTC(), true
		}
	}

	// Check if it ends with Z (UTC)
	if strings.HasSuffix(value, "Z") {
		// DATETIME format with Z: YYYYMMDDTHHMMSSz
		t, err := time.Parse("20060102T150405Z", value)
		if err == nil {
			return t.UTC(), false
		}
	}

	// DATETIME format without Z: YYYYMMDDTHHMMSS
	t, err := time.ParseInLocation("20060102T150405", value, loc)
	if err == nil {
		return t.UTC(), false
	}

	// Try DATE format as fallback
	t, err = time.ParseInLocation("20060102", value, loc)
	if err == nil {
		return t.UTC(), true
	}

	return time.Time{}, false
}

// eventOverlapsRange checks if an event overlaps with the given date range
func eventOverlapsRange(eventStart, eventEnd, rangeStart, rangeEnd time.Time) bool {
	// Event overlaps if it starts before range ends AND ends after range starts
	return eventStart.Before(rangeEnd) && eventEnd.After(rangeStart)
}
