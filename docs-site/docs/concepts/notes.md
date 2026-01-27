---
sidebar_position: 3
---

# Notes

Notes are text-based entities for storing written content. They support rich text, date ranges, and attachments through resource relationships.

## Note Properties

| Property | Description |
|----------|-------------|
| `name` | Title of the note |
| `description` | Main content (supports Markdown) |
| `meta` | Arbitrary JSON metadata |
| `startDate` | Optional start date |
| `endDate` | Optional end date |
| `noteTypeId` | Optional type for categorization |

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

Note Types provide categorization and custom presentation:

### Purpose
- Group similar notes together
- Apply consistent styling
- Enable type-specific filtering

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

Note Types can include custom HTML templates that are rendered using Pongo2 (Django-like) syntax:

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

Notes can have attached Resources through many-to-many relationships:

### Attaching Resources
- Link existing resources to a note
- Resources appear in the note's attachments section
- One resource can be attached to multiple notes

### Use Cases
- Reference documents for meeting notes
- Images to illustrate content
- Supporting files for research notes

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

### Create Note

```
POST /v1/note
Content-Type: application/json

{
  "name": "Meeting Notes",
  "description": "# Agenda\n\n- Item 1\n- Item 2",
  "ownerId": 123,
  "noteTypeId": 1,
  "startDate": "2024-01-15T10:00:00Z"
}
```

### Query Notes

```
GET /v1/notes?noteTypeId=1&ownerId=123
```

### Bulk Operations

- `POST /v1/notes/addTags` - Add tags to notes
- `POST /v1/notes/removeTags` - Remove tags from notes
- `POST /v1/notes/addMeta` - Merge metadata into notes
- `POST /v1/notes/delete` - Delete multiple notes
