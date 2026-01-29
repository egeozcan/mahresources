---
sidebar_position: 3
---

# Notes API

Notes are text-based content items that can be associated with resources, groups, and tags. Each note can have a type that defines its display and behavior.

## List Notes

Retrieve a paginated list of notes with optional filtering.

```
GET /v1/notes
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description/content (partial match) |
| `OwnerId` | integer | Filter by owner group ID |
| `Groups` | integer[] | Filter by associated group IDs |
| `Tags` | integer[] | Filter by tag IDs |
| `Ids` | integer[] | Filter by specific note IDs |
| `NoteTypeId` | integer | Filter by note type ID |
| `CreatedBefore` | string | Filter by creation date (ISO 8601) |
| `CreatedAfter` | string | Filter by creation date (ISO 8601) |
| `StartDateBefore` | string | Notes starting before this date |
| `StartDateAfter` | string | Notes starting after this date |
| `EndDateBefore` | string | Notes ending before this date |
| `EndDateAfter` | string | Notes ending after this date |
| `SortBy` | string[] | Sort order |

### Example

```bash
# List all notes
curl http://localhost:8181/v1/notes.json

# Filter by note type
curl "http://localhost:8181/v1/notes.json?NoteTypeId=1"

# Filter by owner group
curl "http://localhost:8181/v1/notes.json?OwnerId=5"

# Filter by date range
curl "http://localhost:8181/v1/notes.json?StartDateAfter=2024-01-01&StartDateBefore=2024-12-31"
```

### Response

```json
[
  {
    "ID": 1,
    "Name": "Meeting Notes",
    "Description": "Notes from the project kickoff meeting...",
    "StartDate": "2024-01-15T10:00:00Z",
    "EndDate": "2024-01-15T11:30:00Z",
    "OwnerId": 5,
    "NoteTypeId": 1,
    "Meta": {"attendees": ["Alice", "Bob"]},
    "CreatedAt": "2024-01-15T12:00:00Z",
    "UpdatedAt": "2024-01-15T12:00:00Z",
    "Tags": [...],
    "Groups": [...],
    "Resources": [...],
    "NoteType": {...}
  }
]
```

## Get Single Note

Retrieve details for a specific note.

```
GET /v1/note?id={id}
```

### Example

```bash
curl http://localhost:8181/v1/note.json?id=123
```

## Create or Update Note

Create a new note or update an existing one.

```
POST /v1/note
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Note ID (include to update, omit to create) |
| `Name` | string | Note title |
| `Description` | string | Note content/body |
| `OwnerId` | integer | Owner group ID |
| `NoteTypeId` | integer | Note type ID |
| `Groups` | integer[] | Associated group IDs |
| `Tags` | integer[] | Tag IDs |
| `Resources` | integer[] | Associated resource IDs |
| `Meta` | string | JSON metadata object |
| `StartDate` | string | Start date (ISO 8601) |
| `EndDate` | string | End date (ISO 8601) |

### Example - Create

```bash
curl -X POST http://localhost:8181/v1/note \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Project Notes",
    "Description": "This is the note content...",
    "OwnerId": 5,
    "NoteTypeId": 1,
    "Tags": [1, 2],
    "StartDate": "2024-01-15T10:00:00Z"
  }'
```

### Example - Update

```bash
curl -X POST http://localhost:8181/v1/note \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "ID": 123,
    "Name": "Updated Title",
    "Description": "Updated content..."
  }'
```

### Response

```json
{
  "ID": 123,
  "Name": "Project Notes",
  "Description": "This is the note content...",
  ...
}
```

## Delete Note

Delete a note.

```
POST /v1/note/delete?Id={id}
```

### Example

```bash
curl -X POST "http://localhost:8181/v1/note/delete?Id=123" \
  -H "Accept: application/json"
```

## Get Note Meta Keys

Get all unique metadata keys used across notes.

```
GET /v1/notes/meta/keys
```

### Example

```bash
curl http://localhost:8181/v1/notes/meta/keys.json
```

### Response

```json
["attendees", "location", "priority", "status"]
```

## Inline Editing

Edit note name or description with minimal payload.

### Edit Name

```
POST /v1/note/editName?id={id}
```

### Edit Description

```
POST /v1/note/editDescription?id={id}
```

---

# Note Blocks API

Note blocks provide a block-based editing system for note content. Each block has a type, position, content (edited in edit mode), and state (updated while viewing). Blocks are ordered by their position string, which uses fractional indexing for efficient reordering.

## Block Types

The following block types are available:

| Type | Description |
|------|-------------|
| `text` | Plain text content |
| `heading` | Heading with level 1-6 |
| `divider` | Visual separator |
| `gallery` | Collection of resource images |
| `references` | Links to groups |
| `todos` | Checklist with items |
| `table` | Data table (manual data or query-based) |

## List Blocks for a Note

Retrieve all blocks for a specific note, ordered by position.

```
GET /v1/note/blocks?noteId={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `noteId` | integer | **Required.** The note ID |

### Example

```bash
curl "http://localhost:8181/v1/note/blocks?noteId=123"
```

### Response

```json
[
  {
    "id": 1,
    "createdAt": "2024-01-15T10:00:00Z",
    "updatedAt": "2024-01-15T10:30:00Z",
    "noteId": 123,
    "type": "heading",
    "position": "a0",
    "content": {"text": "Introduction", "level": 2},
    "state": {}
  },
  {
    "id": 2,
    "createdAt": "2024-01-15T10:00:00Z",
    "updatedAt": "2024-01-15T10:30:00Z",
    "noteId": 123,
    "type": "text",
    "position": "a1",
    "content": {"text": "This is the introduction paragraph..."},
    "state": {}
  }
]
```

## Get Single Block

Retrieve a specific block by ID.

```
GET /v1/note/block?id={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | **Required.** The block ID |

### Example

```bash
curl "http://localhost:8181/v1/note/block?id=1"
```

### Response

```json
{
  "id": 1,
  "createdAt": "2024-01-15T10:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z",
  "noteId": 123,
  "type": "text",
  "position": "a0",
  "content": {"text": "Hello world"},
  "state": {}
}
```

## Create Block

Create a new block for a note.

```
POST /v1/note/block
```

### Request Body (JSON)

| Field | Type | Description |
|-------|------|-------------|
| `noteId` | integer | **Required.** The note ID |
| `type` | string | **Required.** Block type (text, heading, etc.) |
| `position` | string | **Required.** Position string for ordering |
| `content` | object | Initial content (defaults to type's default content) |

### Example

```bash
curl -X POST http://localhost:8181/v1/note/block \
  -H "Content-Type: application/json" \
  -d '{
    "noteId": 123,
    "type": "text",
    "position": "a0",
    "content": {"text": "My new paragraph"}
  }'
```

### Response

Returns the created block with HTTP status 201.

```json
{
  "id": 5,
  "createdAt": "2024-01-15T12:00:00Z",
  "updatedAt": "2024-01-15T12:00:00Z",
  "noteId": 123,
  "type": "text",
  "position": "a0",
  "content": {"text": "My new paragraph"},
  "state": {}
}
```

## Update Block Content

Update the content of an existing block. Use this in edit mode.

```
PUT /v1/note/block?id={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | **Required.** The block ID |

### Request Body (JSON)

| Field | Type | Description |
|-------|------|-------------|
| `content` | object | **Required.** New content for the block |

### Example

```bash
curl -X PUT "http://localhost:8181/v1/note/block?id=5" \
  -H "Content-Type: application/json" \
  -d '{
    "content": {"text": "Updated paragraph text"}
  }'
```

### Response

Returns the updated block.

```json
{
  "id": 5,
  "createdAt": "2024-01-15T12:00:00Z",
  "updatedAt": "2024-01-15T12:05:00Z",
  "noteId": 123,
  "type": "text",
  "position": "a0",
  "content": {"text": "Updated paragraph text"},
  "state": {}
}
```

## Update Block State

Update the state of a block. Use this while viewing (e.g., checking a todo item).

```
PATCH /v1/note/block/state?id={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | **Required.** The block ID |

### Request Body (JSON)

| Field | Type | Description |
|-------|------|-------------|
| `state` | object | **Required.** New state for the block |

### Example

```bash
# Mark a todo item as checked
curl -X PATCH "http://localhost:8181/v1/note/block/state?id=10" \
  -H "Content-Type: application/json" \
  -d '{
    "state": {"checked": ["item-1", "item-2"]}
  }'
```

### Response

Returns the updated block.

```json
{
  "id": 10,
  "createdAt": "2024-01-15T12:00:00Z",
  "updatedAt": "2024-01-15T12:10:00Z",
  "noteId": 123,
  "type": "todos",
  "position": "a2",
  "content": {"items": [{"id": "item-1", "label": "Task 1"}, {"id": "item-2", "label": "Task 2"}]},
  "state": {"checked": ["item-1", "item-2"]}
}
```

## Delete Block

Delete a block.

```
DELETE /v1/note/block?id={id}
```

Or using POST (for form compatibility):

```
POST /v1/note/block/delete?id={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | **Required.** The block ID |

### Example

```bash
curl -X DELETE "http://localhost:8181/v1/note/block?id=5"
```

### Response

Returns HTTP status 204 (No Content) on success.

## Reorder Blocks

Update positions for multiple blocks in a single request.

```
POST /v1/note/blocks/reorder
```

### Request Body (JSON)

| Field | Type | Description |
|-------|------|-------------|
| `noteId` | integer | **Required.** The note ID |
| `positions` | object | **Required.** Map of block ID to new position string |

### Example

```bash
curl -X POST http://localhost:8181/v1/note/blocks/reorder \
  -H "Content-Type: application/json" \
  -d '{
    "noteId": 123,
    "positions": {
      "1": "a0",
      "2": "a1",
      "3": "a2"
    }
  }'
```

### Response

Returns HTTP status 204 (No Content) on success.

## Block Type Schemas

Each block type has specific content and state schemas.

### Text Block

**Content:**
```json
{
  "text": "The text content"
}
```

**State:** Empty object `{}`

### Heading Block

**Content:**
```json
{
  "text": "Heading text",
  "level": 2
}
```

- `level`: Integer 1-6 (corresponds to h1-h6)

**State:** Empty object `{}`

### Divider Block

**Content:** Empty object `{}`

**State:** Empty object `{}`

### Gallery Block

**Content:**
```json
{
  "resourceIds": [1, 2, 3]
}
```

- `resourceIds`: Array of resource IDs to display

**State:**
```json
{
  "layout": "grid"
}
```

- `layout`: Either `"grid"` or `"list"`

### References Block

**Content:**
```json
{
  "groupIds": [10, 20, 30]
}
```

- `groupIds`: Array of group IDs to reference

**State:** Empty object `{}`

### Todos Block

**Content:**
```json
{
  "items": [
    {"id": "item-1", "label": "First task"},
    {"id": "item-2", "label": "Second task"}
  ]
}
```

- `items`: Array of todo items, each with unique `id` and `label`

**State:**
```json
{
  "checked": ["item-1"]
}
```

- `checked`: Array of item IDs that are checked

### Table Block

**Content (manual data):**
```json
{
  "columns": ["Name", "Value", "Description"],
  "rows": [
    ["Row 1", 100, "First row"],
    ["Row 2", 200, "Second row"]
  ]
}
```

**Content (query-based):**
```json
{
  "queryId": 5
}
```

- Either provide `columns`/`rows` OR `queryId`, not both

**State:**
```json
{
  "sortColumn": "Name",
  "sortDir": "asc"
}
```

- `sortDir`: Either `"asc"` or `"desc"`

---

# Note Types API

Note types define templates and display customizations for notes.

## List Note Types

Retrieve all note types.

```
GET /v1/note/noteTypes
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Description` | string | Filter by description |

### Example

```bash
curl http://localhost:8181/v1/note/noteTypes.json
```

### Response

```json
[
  {
    "ID": 1,
    "Name": "Meeting",
    "Description": "Meeting notes template",
    "CustomHeader": "<h2>{{.Name}}</h2>",
    "CustomSidebar": "...",
    "CustomSummary": "...",
    "CustomAvatar": "..."
  }
]
```

## Create Note Type

Create a new note type.

```
POST /v1/note/noteType
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | Note type name |
| `Description` | string | Description |
| `CustomHeader` | string | Custom header template |
| `CustomSidebar` | string | Custom sidebar template |
| `CustomSummary` | string | Custom summary template |
| `CustomAvatar` | string | Custom avatar template |

### Example

```bash
curl -X POST http://localhost:8181/v1/note/noteType \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Task",
    "Description": "Task tracking notes",
    "CustomHeader": "<div class=\"task-header\">{{.Name}}</div>"
  }'
```

## Edit Note Type

Update an existing note type.

```
POST /v1/note/noteType/edit
```

### Parameters

Same as create, but include `ID` to identify the note type to update.

### Example

```bash
curl -X POST http://localhost:8181/v1/note/noteType/edit \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "ID": 1,
    "Name": "Meeting Notes",
    "Description": "Updated description"
  }'
```

## Delete Note Type

Delete a note type.

```
POST /v1/note/noteType/delete?Id={id}
```

### Example

```bash
curl -X POST "http://localhost:8181/v1/note/noteType/delete?Id=1" \
  -H "Accept: application/json"
```

## Inline Editing for Note Types

### Edit Name

```
POST /v1/noteType/editName?id={id}
```

### Edit Description

```
POST /v1/noteType/editDescription?id={id}
```
