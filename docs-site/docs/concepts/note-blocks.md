---
sidebar_position: 8
title: Note Blocks
---

# Note Blocks

Note Blocks provide a structured content system within Notes. Each block has a type, a string-based position for ordering, JSON content (edited in edit mode), and JSON state (modified while viewing).

## Block Properties

| Property | Type | Description |
|----------|------|-------------|
| `type` | string | Block type identifier |
| `position` | string | Lexicographic ordering key (max 64 chars) |
| `content` | JSON | Data edited in edit mode (defaults to `{}`) |
| `state` | JSON | Runtime/UI state (defaults to `{}`) |
| `noteId` | integer | FK to the parent Note |

## Content vs State

Blocks separate what you author from what changes during use:

- **Content**: Changed in edit mode. Example: a todo item's text, a heading's title, a table's query name
- **State**: Changed in view mode. Example: which todos are checked, the calendar's current view, a table's sort direction

This separation means users can interact with blocks (checking items, sorting tables) without entering edit mode.

## Block Types

Eight built-in block types ship with Mahresources. Plugins can register additional types using `mah.block_type()` -- these appear with the prefix `plugin:<plugin-name>:<type>`.

### Text

Rich text content. The first text block syncs bidirectionally with the Note's `description` field.

```json
{"text": "This is a paragraph of text."}
```

### Heading

Section heading with a configurable level (1-6, corresponding to h1-h6).

**Content:**
```json
{"text": "Section Title", "level": 2}
```

**State:** Empty object `{}`

### Divider

A horizontal separator line. Has no content or state.

**Content:** `{}`

**State:** `{}`

### Gallery

A collection of Resources displayed as thumbnails. State controls the layout.

**Content:**
```json
{"resourceIds": [1, 2, 3]}
```

**State:**
```json
{"layout": "grid"}
```

- `layout`: `"grid"` or `"list"`

### References

A list of linked Groups displayed as cards.

**Content:**
```json
{"groupIds": [10, 20, 30]}
```

**State:** Empty object `{}`

### Todos

Interactive checklist. Content holds the items; state tracks which are checked. Each item needs a unique `id`.

**Content:**
```json
{
  "items": [
    {"id": "a1b2", "label": "First task"},
    {"id": "c3d4", "label": "Second task"}
  ]
}
```

**State:**
```json
{"checked": ["a1b2"]}
```

### Table

Data table with two modes: manual data (columns/rows) or query-driven.

**Manual mode:**
```json
{
  "columns": ["Name", "Size", "Type"],
  "rows": [["photo.jpg", "2.4 MB", "image/jpeg"]]
}
```

**Query mode:**
```json
{
  "queryId": 5,
  "queryParams": {"minSize": "1000000"},
  "isStatic": false
}
```

The `queryId` references a saved Query by ID. The `queryParams` object provides named parameters passed to the Query at execution time. Set `isStatic` to `true` to prevent automatic refresh.

**State:**
```json
{"sortColumn": "Name", "sortDir": "asc"}
```

### Calendar

Calendar view from iCal sources (URL or stored Resource) with optional custom events.

**Content:**
```json
{
  "calendars": [
    {
      "id": "work",
      "name": "Work Calendar",
      "color": "#3b82f6",
      "source": {"type": "url", "url": "https://calendar.example.org/work.ics"}
    },
    {
      "id": "local",
      "name": "Stored Calendar",
      "color": "#10b981",
      "source": {"type": "resource", "resourceId": 42}
    }
  ]
}
```

**State:**
```json
{
  "view": "month",
  "currentDate": "2024-06-15",
  "customEvents": [
    {
      "id": "evt1",
      "title": "Team Meeting",
      "start": "2024-06-20T10:00:00Z",
      "end": "2024-06-20T11:00:00Z",
      "allDay": false,
      "calendarId": "custom"
    }
  ]
}
```

- `state.view`: `month`, `week`, or `agenda`
- `state.customEvents`: User-created events (max 500 per block, each with `calendarId` set to `"custom"`)
- ICS files are capped at 10MB. Recurring events (RRULE) are not supported.

## Position Ordering

Blocks use lexicographic string positions for ordering. Insert between existing blocks without renumbering:

| Position | Block |
|----------|-------|
| `a` | First block |
| `b` | Second block |
| `am` | Inserted between first and second |

When position strings grow too long from repeated insertions, call the rebalance endpoint to reassign evenly distributed positions.

## Description Synchronization

The Note's `description` field and the first text block stay in sync:

- Editing the first text block updates `description`
- Editing `description` updates the first text block
- Deleting the first text block promotes the next text block
- Notes without blocks render `description` directly

## API Operations

### Block CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/note/blocks?noteId={id}` | List blocks for a Note (ordered by position) |
| `GET` | `/v1/note/block?id={id}` | Get single block |
| `GET` | `/v1/note/block/types` | List available block types |
| `POST` | `/v1/note/block` | Create block (JSON body: `noteId`, `type`, `position`, `content`) |
| `PUT` | `/v1/note/block` | Update block content (JSON body: `id`, `content`) |
| `PATCH` | `/v1/note/block/state` | Update block state (JSON body: `id`, `state`) |
| `DELETE` | `/v1/note/block?id={id}` | Delete block |
| `POST` | `/v1/note/block/delete?id={id}` | Delete block (form alternative) |

### Ordering

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/note/blocks/reorder` | Bulk update positions (JSON: `noteId`, `positions` map) |
| `POST` | `/v1/note/blocks/rebalance?noteId={id}` | Redistribute position strings evenly |

### Sub-Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/note/block/table/query?blockId={id}` | Execute table block's saved Query |
| `GET` | `/v1/note/block/calendar/events?blockId={id}&start={date}&end={date}` | Fetch calendar events (YYYY-MM-DD dates) |

For full API examples and response formats, see [API: Notes](../api/notes.md).
