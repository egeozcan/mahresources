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
