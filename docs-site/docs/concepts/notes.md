---
sidebar_position: 3
---

# Notes

A Note holds text content: meeting minutes, journal entries, research, or any written material. Notes render as Markdown, support optional date ranges, and can have Resources (files) attached.

## Note Properties

| Property | Description |
|----------|-------------|
| `name` | Title of the note |
| `description` | Main content (supports Markdown) |
| `meta` | Arbitrary JSON metadata |
| `startDate` | Optional start date |
| `endDate` | Optional end date |
| `noteTypeId` | Optional type for categorization |
| `shareToken` | Token for public sharing (see [Note Sharing](../features/note-sharing.md)) |
| `blocks` | Block-based content units (see [Block-Based Content](#block-based-content)) |

## Content Format

The `description` field contains the main note content:

- Plain text is always supported
- Markdown rendering is available in the UI
- HTML is preserved if entered directly
- No character limit (stored as TEXT)

### Markdown Support

Notes support standard Markdown formatting:

```markdown
# Heading 1
## Heading 2

**Bold** and *italic* text

- Bullet lists
- With multiple items

1. Numbered lists
2. Work too

[Links](https://example.com)

> Blockquotes for citations

`inline code` and code blocks
```

## Block-Based Content

Notes also support an optional block-based content structure for rich, interactive content.

### What Are Blocks?

Blocks are structured content units within a note. Each block has a specific type and stores its data as JSON -- enabling interactive elements like to-do lists, image galleries, and sortable tables that maintain state between sessions.

### Block Properties

| Property | Description |
|----------|-------------|
| `type` | Block type (text, heading, divider, gallery, references, todos, table, calendar) |
| `content` | JSON data edited in edit mode |
| `state` | JSON data modified while viewing |
| `position` | Lexicographic string for ordering |

### Content vs State

Blocks separate **content** (what you edit) from **state** (runtime changes):

- **Content**: Data changed in edit mode. Examples: todo item labels, heading text, table columns
- **State**: Data modified while viewing. Examples: which todos are checked, table sort order

Users can interact with blocks (checking items, sorting tables) without entering edit mode.

### Block Types

#### Text Block

Basic text content, supports Markdown.

```json
{
  "type": "text",
  "content": { "text": "This is a paragraph of text." }
}
```

#### Heading Block

Section headings with configurable level.

```json
{
  "type": "heading",
  "content": { "text": "Section Title", "level": 2 }
}
```

Supported levels: 1 through 6.

#### Divider Block

Visual separator between content sections.

```json
{
  "type": "divider",
  "content": {}
}
```

#### Gallery Block

Displays attached resources as an image gallery.

```json
{
  "type": "gallery",
  "content": { "resourceIds": [101, 102, 103] }
}
```

#### References Block

Links to related groups.

```json
{
  "type": "references",
  "content": { "groupIds": [5, 12, 27] }
}
```

#### Todos Block

Interactive to-do list with checkable items.

```json
{
  "type": "todos",
  "content": {
    "items": [
      { "id": "a1b2", "label": "First task" },
      { "id": "c3d4", "label": "Second task" }
    ]
  },
  "state": {
    "checked": ["a1b2"]
  }
}
```

- `content.items`: The to-do items (edited in edit mode)
- `state.checked`: IDs of checked items (toggled while viewing)

#### Table Block

Sortable data table.

```json
{
  "type": "table",
  "content": {
    "columns": [
      { "id": "name", "label": "Name" },
      { "id": "status", "label": "Status" }
    ],
    "rows": [
      { "id": "r1", "name": "Item A", "status": "Active" },
      { "id": "r2", "name": "Item B", "status": "Pending" }
    ]
  },
  "state": {
    "sortColumn": "name",
    "sortDir": "asc"
  }
}
```

- `content.columns` and `content.rows`: Table structure (edited in edit mode)
- `state.sortColumn` and `state.sortDir`: Current sort settings (changed by clicking headers)

#### Calendar Block

Displays calendar events from iCal sources or custom entries.

```json
{
  "type": "calendar",
  "content": {
    "calendars": [
      {
        "id": "work",
        "name": "Work Calendar",
        "color": "#3b82f6",
        "source": { "type": "url", "url": "https://example.com/cal.ics" }
      },
      {
        "id": "local",
        "name": "Local File",
        "color": "#10b981",
        "source": { "type": "resource", "resourceId": 42 }
      }
    ]
  },
  "state": {
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
}
```

- `content.calendars`: Calendar sources -- each with an `id`, `name`, optional hex `color`, and a `source` (either `url` or `resource` with a `resourceId` pointing to an iCal file)
- `state.view`: Current view mode (`month`, `week`, or `agenda`)
- `state.customEvents`: User-created events (max 500 per block). Each event must have `calendarId` set to `"custom"`

### Position Ordering

Blocks use lexicographic position strings (e.g., "a", "b", "c" or "aaa", "aab") for ordering. Blocks can be inserted between existing ones without renumbering:

| Position | Block |
|----------|-------|
| `a` | First block |
| `b` | Second block |
| `am` | Inserted between first and second |

### Backward Compatibility

Blocks are backward compatible with existing notes:

- A note's `description` field syncs bidirectionally with its first text block
- Notes without blocks render the `description` field as before
- Adding blocks to an existing note preserves the description as the first text block
- Editing the first text block updates the description field

Older clients and integrations continue to work while new features use the block system.

## Date Ranges

Notes can have optional date fields for temporal organization:

### Start Date
- When the note's subject began
- Useful for events, projects, or time-bound topics

### End Date
- When the note's subject ended
- Can be left empty for ongoing items

### Use Cases

| Scenario | Start Date | End Date |
|----------|------------|----------|
| Single event | Event date | Same as start |
| Date range | Begin date | End date |
| Ongoing | Begin date | Empty |
| Point in time | Date | Empty |

Dates enable filtering and sorting notes chronologically.

## Note Types

Note Types group similar notes and apply consistent styling, enabling type-specific filtering and custom presentation.

### Note Type Properties

| Property | Description |
|----------|-------------|
| `name` | Type name (e.g., "Meeting Notes") |
| `description` | Explanation of the type |
| `customHeader` | HTML template for note headers |
| `customSidebar` | HTML template for note sidebars |
| `customSummary` | HTML template for list views |
| `customAvatar` | HTML template for note avatars |

### Custom Templates

Note Types can include custom HTML templates rendered with Pongo2 (Django-like) syntax:

```html
<!-- customHeader example -->
<div class="meeting-header">
  <span class="date">{{ note.startDate }}</span>
  <span class="type-badge">Meeting</span>
</div>
```

Templates have access to:
- `note` - The current note object
- `meta` - The note's metadata
- Standard template functions

## Attachments

Mahresources links Resources to Notes through many-to-many relationships. A resource can be attached to multiple notes, and each note can have multiple attachments -- reference documents for meeting minutes, images to illustrate content, or supporting files for research.

## Relationships

Notes connect to other entities:

### Ownership
- A Note can be **owned by** one Group
- Appears in the owner's "Owned Notes" section
- Deleting the owner cascades to owned notes

### Related Groups
- A Note can be **related to** multiple Groups
- Many-to-many relationship
- Appears in each group's "Related Notes" section

### Attached Resources
- A Note can have multiple Resources attached
- Many-to-many relationship
- Resources appear as attachments

### Tags
- A Note can have multiple Tags
- Enables topic-based organization
- Many-to-many relationship

## Searching Notes

Notes are included in global search:

- Searches `name` and `description` fields
- Full-text search when FTS is enabled
- Filter by Note Type in advanced search

### Query Parameters

Filter notes with these parameters:

| Parameter | Description |
|-----------|-------------|
| `name` | Filter by name (partial match) |
| `noteTypeId` | Filter by Note Type |
| `ownerId` | Filter by owner Group |
| `tags` | Filter by tag IDs |
| `startDateAfter` | Notes starting after date |
| `startDateBefore` | Notes starting before date |
| `endDateAfter` | Notes ending after date |
| `endDateBefore` | Notes ending before date |

## API Operations

For full API details -- creating, querying, and bulk operations on Notes -- see [API: Notes](../api/notes.md).
