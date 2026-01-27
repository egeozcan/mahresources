---
sidebar_position: 5
---

# Other API Endpoints

This page covers Tags, Categories, Queries, Search, Logs, and Download Queue endpoints.

---

## Tags API

Tags are labels that can be applied to resources, notes, and groups for organization.

### List Tags

```
GET /v1/tags
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description |
| `CreatedBefore` | string | Filter by creation date |
| `CreatedAfter` | string | Filter by creation date |
| `SortBy` | string[] | Sort order |

#### Example

```bash
curl http://localhost:8181/v1/tags.json

# Search for tags
curl "http://localhost:8181/v1/tags.json?Name=project"
```

#### Response

```json
[
  {
    "ID": 1,
    "Name": "important",
    "Description": "High priority items",
    "CreatedAt": "2024-01-01T00:00:00Z"
  }
]
```

### Create or Update Tag

```
POST /v1/tag
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Tag ID (include to update) |
| `Name` | string | Tag name |
| `Description` | string | Description |

#### Example

```bash
# Create
curl -X POST http://localhost:8181/v1/tag \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"Name": "urgent", "Description": "Requires immediate attention"}'

# Update
curl -X POST http://localhost:8181/v1/tag \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"ID": 1, "Name": "critical", "Description": "Updated description"}'
```

### Delete Tag

```
POST /v1/tag/delete?Id={id}
```

### Inline Editing

```
POST /v1/tag/editName?id={id}
POST /v1/tag/editDescription?id={id}
```

---

## Categories API

Categories define types for groups with optional display customizations.

### List Categories

```
GET /v1/categories
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Description` | string | Filter by description |

#### Example

```bash
curl http://localhost:8181/v1/categories.json
```

#### Response

```json
[
  {
    "ID": 1,
    "Name": "Person",
    "Description": "Individual people",
    "CustomHeader": "...",
    "CustomSidebar": "...",
    "CustomSummary": "...",
    "CustomAvatar": "...",
    "MetaSchema": "..."
  }
]
```

### Create or Update Category

```
POST /v1/category
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Category ID (include to update) |
| `Name` | string | Category name |
| `Description` | string | Description |
| `CustomHeader` | string | Custom header template |
| `CustomSidebar` | string | Custom sidebar template |
| `CustomSummary` | string | Custom summary template |
| `CustomAvatar` | string | Custom avatar template |
| `MetaSchema` | string | JSON schema for metadata validation |

#### Example

```bash
curl -X POST http://localhost:8181/v1/category \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Company",
    "Description": "Business organizations"
  }'
```

### Delete Category

```
POST /v1/category/delete?Id={id}
```

### Inline Editing

```
POST /v1/category/editName?id={id}
POST /v1/category/editDescription?id={id}
```

---

## Queries API

Queries are saved SQL queries that can be executed to generate custom reports.

### List Queries

```
GET /v1/queries
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Text` | string | Filter by query text |

#### Example

```bash
curl http://localhost:8181/v1/queries.json
```

### Get Single Query

```
GET /v1/query?id={id}
```

### Create or Update Query

```
POST /v1/query
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Query ID (include to update) |
| `Name` | string | Query name |
| `Text` | string | SQL query text |
| `Template` | string | Display template |

#### Example

```bash
curl -X POST http://localhost:8181/v1/query \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Recent Resources",
    "Text": "SELECT * FROM resources ORDER BY created_at DESC LIMIT 10"
  }'
```

### Delete Query

```
POST /v1/query/delete?Id={id}
```

### Run Query

Execute a saved query and get results.

```
POST /v1/query/run
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Query ID to run |
| `name` | string | Query name to run (alternative to id) |

#### Example

```bash
# Run by ID
curl -X POST "http://localhost:8181/v1/query/run?id=1" \
  -H "Accept: application/json"

# Run by name
curl -X POST "http://localhost:8181/v1/query/run?name=Recent%20Resources" \
  -H "Accept: application/json"
```

### Inline Editing

```
POST /v1/query/editName?id={id}
POST /v1/query/editDescription?id={id}
```

---

## Global Search API

Search across all entity types (resources, notes, groups, tags, categories).

### Search

```
GET /v1/search
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `q` | string | **Required.** Search query |
| `limit` | integer | Maximum results (default: 20) |
| `types` | string | Comma-separated entity types to search |

#### Example

```bash
# Search everything
curl "http://localhost:8181/v1/search.json?q=project"

# Search with limit
curl "http://localhost:8181/v1/search.json?q=project&limit=50"

# Search specific types
curl "http://localhost:8181/v1/search.json?q=project&types=resources,notes"
```

#### Response

```json
{
  "query": "project",
  "total": 15,
  "results": [
    {
      "id": 1,
      "type": "group",
      "name": "Project Alpha",
      "description": "Main project group",
      "score": 100,
      "url": "/group?id=1",
      "extra": {"category": "Project"}
    },
    {
      "id": 5,
      "type": "resource",
      "name": "project-plan.pdf",
      "description": "Project planning document",
      "score": 85,
      "url": "/resource?id=5"
    }
  ]
}
```

---

## Logs API

Access the audit log of system events and entity changes.

### List Log Entries

```
GET /v1/logs
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Level` | string | Filter by level (info, warning, error) |
| `Action` | string | Filter by action (create, update, delete, system) |
| `EntityType` | string | Filter by entity type |
| `EntityID` | integer | Filter by entity ID |
| `Message` | string | Search in message (partial match) |
| `RequestPath` | string | Search in request path |
| `CreatedBefore` | string | Filter by date |
| `CreatedAfter` | string | Filter by date |
| `SortBy` | string[] | Sort order |

#### Example

```bash
# List recent logs
curl http://localhost:8181/v1/logs.json

# Filter by action
curl "http://localhost:8181/v1/logs.json?Action=create"

# Filter by entity type
curl "http://localhost:8181/v1/logs.json?EntityType=resource"

# Filter errors only
curl "http://localhost:8181/v1/logs.json?Level=error"
```

#### Response

```json
{
  "logs": [
    {
      "ID": 100,
      "Level": "info",
      "Action": "create",
      "EntityType": "resource",
      "EntityID": 456,
      "Message": "Resource created: photo.jpg",
      "RequestPath": "/v1/resource",
      "CreatedAt": "2024-01-15T10:30:00Z"
    }
  ],
  "totalCount": 1500,
  "page": 1,
  "perPage": 30
}
```

### Get Single Log Entry

```
GET /v1/log?id={id}
```

### Get Entity History

Get all log entries for a specific entity.

```
GET /v1/logs/entity
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `entityType` | string | **Required.** Entity type (tag, note, resource, group) |
| `entityId` | integer | **Required.** Entity ID |
| `page` | integer | Page number (default: 1) |

#### Example

```bash
# Get history for a specific resource
curl "http://localhost:8181/v1/logs/entity.json?entityType=resource&entityId=123"
```

---

## Download Queue API

Manage background downloads for remote resources.

### Submit Download

Add URLs to the download queue.

```
POST /v1/download/submit
```

#### Parameters

Same as `POST /v1/resource/remote`, but always queues for background download.

Multiple URLs can be submitted by separating them with newlines in the `URL` field.

#### Example

```bash
curl -X POST http://localhost:8181/v1/download/submit \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "URL": "https://example.com/file1.zip\nhttps://example.com/file2.zip",
    "OwnerId": 5
  }'
```

### Get Download Queue

Get all download jobs.

```
GET /v1/download/queue
```

#### Example

```bash
curl http://localhost:8181/v1/download/queue.json
```

#### Response

```json
[
  {
    "id": "job-123",
    "url": "https://example.com/file.zip",
    "status": "downloading",
    "progress": 45,
    "createdAt": "2024-01-15T10:00:00Z"
  }
]
```

### Cancel Download

Cancel a pending or in-progress download.

```
POST /v1/download/cancel?id={job_id}
```

### Pause Download

Pause a download job.

```
POST /v1/download/pause?id={job_id}
```

### Resume Download

Resume a paused download (restarts from beginning).

```
POST /v1/download/resume?id={job_id}
```

### Retry Download

Retry a failed or cancelled download.

```
POST /v1/download/retry?id={job_id}
```

### Download Events (SSE)

Subscribe to real-time download status updates via Server-Sent Events.

```
GET /v1/download/events
```

#### Example

```javascript
const eventSource = new EventSource('http://localhost:8181/v1/download/events');

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Download update:', data);
};
```

---

## Meta Keys Endpoints

Get all unique metadata keys used across entities.

| Entity | Endpoint |
|--------|----------|
| Resources | `GET /v1/resources/meta/keys` |
| Notes | `GET /v1/notes/meta/keys` |
| Groups | `GET /v1/groups/meta/keys` |

These endpoints return arrays of strings representing all metadata keys in use:

```json
["author", "source", "date_created", "location"]
```
