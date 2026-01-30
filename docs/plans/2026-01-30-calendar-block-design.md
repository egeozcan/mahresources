# Calendar Block Design

## Overview

Add a calendar block type to the note block system that displays events from one or more ICS calendar sources. Supports both URL-based calendars (with stale-while-revalidate caching) and local ICS files stored as Resources.

## Data Model

### Content Schema

Stored in block's `content` JSON field:

```json
{
  "calendars": [
    {
      "id": "uuid-string",
      "name": "Work Calendar",
      "color": "#3b82f6",
      "source": {
        "type": "url",
        "url": "https://calendar.google.com/calendar/ical/..."
      }
    },
    {
      "id": "uuid-string",
      "name": "Holidays",
      "color": "#10b981",
      "source": {
        "type": "resource",
        "resourceId": 123
      }
    }
  ]
}
```

### State Schema

Stored in block's `state` JSON field:

```json
{
  "view": "month",
  "currentDate": "2026-01-01"
}
```

### Source Types

- `url`: External ICS calendar URL (Google Calendar, Outlook, etc.)
- `resource`: Local ICS file stored as a Resource in the system

## Backend Architecture

### New API Endpoint

`GET /v1/note/block/calendar/events`

**Query parameters:**
- `blockId` (required): Block ID to fetch events for
- `start` (required): ISO date string for range start
- `end` (required): ISO date string for range end

**Response:**

```json
{
  "events": [
    {
      "id": "event-uid-from-ics",
      "calendarId": "calendar-uuid",
      "title": "Team Meeting",
      "start": "2026-01-30T10:00:00Z",
      "end": "2026-01-30T11:00:00Z",
      "allDay": false,
      "location": "Room 101",
      "description": "Weekly sync"
    }
  ],
  "calendars": [
    { "id": "uuid", "name": "Work", "color": "#3b82f6" }
  ],
  "cachedAt": "2026-01-30T09:55:00Z"
}
```

### ICS Fetching Flow

1. Read block content to get calendar sources
2. For each calendar:
   - **URL source**: Check backend cache
     - Fresh (<5min): Use cached ICS content
     - Stale (≥5min): Return cached, spawn goroutine to refresh
     - Missing/expired: Fetch synchronously
   - **Resource source**: Read ICS file directly from storage
3. Parse all ICS files using `arran4/golang-ical` library
4. Expand recurring events within requested date range
5. Filter events to date range
6. Return merged events with calendar metadata

### Backend Caching

- In-memory cache keyed by URL (shared across blocks using same calendar)
- Cache entry contains: raw ICS content, fetched timestamp, ETag/Last-Modified headers
- On fetch: Send conditional headers (`If-None-Match`, `If-Modified-Since`) when available
- LRU eviction at ~100 entries
- Max 1MB per ICS file

## Frontend Component

### File: `src/components/blocks/blockCalendar.js`

Pattern matches `blockTable.js`:
- Module-level cache with 5-minute staleness threshold
- `fetchEvents()` implements stale-while-revalidate
- Reactive properties: `events`, `calendars`, `view`, `currentDate`, `isRefreshing`

### View Mode UI

**Header bar:**
- Month/year label (e.g., "January 2026")
- Previous/next navigation arrows
- View toggle: month | agenda

**Month view:**
- CSS grid with 7 columns (Sun-Sat or Mon-Sun based on locale)
- Day cells show date number and colored event indicators
- Events shown as small colored bars or dots
- Click day to see full event list in popover

**Agenda view:**
- Scrollable list grouped by date
- Each event shows: colored left border, time, title
- Empty dates omitted
- Shows next ~30 days of events

**Hover tooltip (both views):**
- Full event title
- Start/end time
- Location (if present)
- Description (if present)
- Calendar name with color badge

### Edit Mode UI

**Configured calendars list:**
- Color swatch (clickable to change)
- Editable display name
- Source indicator (URL icon or Resource icon)
- Remove button

**Add calendar section:**
- Text input for URL (paste and enter)
- "Upload ICS" button (creates Resource, adds to block)
- "Select Resource" button (opens entity picker filtered to .ics content type)

**Color picker:**
- Preset palette of 8-10 colors
- Optional custom hex input

## Block Type Registration

### File: `models/block_types/calendar.go`

```go
type calendarContent struct {
    Calendars []CalendarSource `json:"calendars"`
}

type CalendarSource struct {
    ID     string       `json:"id"`
    Name   string       `json:"name"`
    Color  string       `json:"color"`
    Source SourceConfig `json:"source"`
}

type SourceConfig struct {
    Type       string `json:"type"`
    URL        string `json:"url,omitempty"`
    ResourceID uint   `json:"resourceId,omitempty"`
}
```

**Validation:**
- Color must be valid hex format (#rgb or #rrggbb)
- Source type must be "url" or "resource"
- URL must be well-formed if type is "url"
- ResourceID must be positive if type is "resource"

**Defaults:**
- Empty calendars array
- Month view, current date

### Block Editor Integration

- Icon: `📅`
- Label: "Calendar"
- Added to `blockTypes` array in `blockEditor.js`
- Added to `_getIconForType()` mapping

## Error Handling

### Per-Calendar Errors

- Invalid/unreachable URL: Show error badge in edit mode, skip in view mode
- Malformed ICS content: Same as above
- Resource deleted: Show "Resource not found" warning

Graceful degradation: Other calendars continue to display even if one fails.

### Edge Cases

**Recurring events:**
- Parse RRULE from ICS
- Expand occurrences within requested date range
- Limit expansion to 1 year ahead maximum

**All-day events:**
- Display at top of day cell in month view
- Show without specific time in agenda view

**Multi-day events:**
- Span across multiple day cells in month view
- Show as single entry in agenda with date range

**Timezones:**
- Parse VTIMEZONE from ICS when present
- Convert all times to browser local timezone for display

### Empty States

- No calendars: "Add a calendar to get started" with prominent add buttons
- No events in range: "No events this month" / "No upcoming events"

## Files to Create/Modify

### New Files

- `models/block_types/calendar.go` - Block type definition and validation
- `src/components/blocks/blockCalendar.js` - Frontend Alpine.js component
- `e2e/tests/19-block-calendar.spec.ts` - E2E tests

### Modified Files

- `templates/partials/blockEditor.tpl` - Add calendar block template section
- `server/api_handlers/block_api_handlers.go` - Add events endpoint handler
- `application_context/block_context.go` - Add ICS fetching/caching logic
- `src/components/blockEditor.js` - Add calendar to blockTypes and icon mapping
- `src/main.js` - Import blockCalendar component

## Testing

### E2E Tests

- Add URL-based calendar to block
- Add Resource-based calendar to block
- Switch between month and agenda views
- Navigate between months
- Hover to see event details
- Remove calendar from block
- Error handling for invalid URL

### Unit Tests

- ICS parsing with various event types
- Recurring event expansion
- Timezone conversion
- Date range filtering
