---
sidebar_position: 5
---

# Tags, Categories, Queries, MRQL & More

This page covers Tags, Categories, Resource Categories, Template Partials, Queries, MRQL, Search, Logs, Download Queue, and Users, Accounts & Authentication endpoints.

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
curl http://localhost:8181/v1/tags

# Search for tags
curl "http://localhost:8181/v1/tags?Name=project"
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

### Suggest Tags

Typeahead listing. Accepts the same filters as `GET /v1/tags`, but skips the pagination `COUNT`, so it returns no `X-Total-Count` header.

```
GET /v1/tags/suggest
```

#### Example

```bash
curl "http://localhost:8181/v1/tags/suggest?Name=proj"
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

### Bulk Delete Tags

Delete multiple tags at once.

```
POST /v1/tags/delete
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Tag IDs to delete |

### Merge Tags

Merge multiple tags into one, transferring all associations.

```
POST /v1/tags/merge
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Winner` | integer | Tag ID to keep |
| `Losers` | integer[] | Tag IDs to merge and delete |

### Inline Editing

```
POST /v1/tag/editName?id={id}
POST /v1/tag/editDescription?id={id}
```

### Timeline

```
GET /v1/tags/timeline
```

See [Timeline Endpoints](#timeline-endpoints) below for details.

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
curl http://localhost:8181/v1/categories
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
| `CustomListHeader` | string | Custom list-page header template |
| `CustomDetailFooter` | string | Template rendered at the bottom of the detail page |
| `CustomListFooter` | string | Custom list-page footer template (carrier-bound, like `CustomListHeader`) |
| `CustomHoverCard` | string | Hover-card template; falls back to `CustomSummary` when empty |
| `CustomOwnEntities` | string | Replaces the body of the group detail page's Own Entities section |
| `CustomMRQLResult` | string | Custom MRQL result-card template |
| `CustomCSS` | string | Custom CSS injected on category pages |
| `SectionConfig` | string | JSON section layout configuration |
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

## Resource Categories API

Resource categories define types for resources with optional display customizations and metadata schemas.

### List Resource Categories

```
GET /v1/resourceCategories
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Description` | string | Filter by description |

#### Example

```bash
curl http://localhost:8181/v1/resourceCategories
```

#### Response

```json
[
  {
    "ID": 1,
    "Name": "Photo",
    "Description": "Photograph files",
    "CustomHeader": "...",
    "CustomSidebar": "...",
    "CustomSummary": "...",
    "CustomAvatar": "...",
    "MetaSchema": "..."
  }
]
```

### Create or Update Resource Category

```
POST /v1/resourceCategory
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Resource category ID (include to update) |
| `Name` | string | Resource category name |
| `Description` | string | Description |
| `CustomHeader` | string | Custom header template |
| `CustomSidebar` | string | Custom sidebar template |
| `CustomSummary` | string | Custom summary template |
| `CustomAvatar` | string | Custom avatar template |
| `CustomListHeader` | string | Custom list-page header template |
| `CustomDetailFooter` | string | Template rendered at the bottom of the detail page |
| `CustomListFooter` | string | Custom list-page footer template (carrier-bound, like `CustomListHeader`) |
| `CustomHoverCard` | string | Hover-card template; falls back to `CustomSummary` when empty |
| `CustomPreview` | string | Template rendered above the built-in preview image |
| `CustomLightbox` | string | Lightbox details-panel template; falls back to `CustomSidebar` when empty |
| `CustomCell` | string | Extra table-cell template for the resources details table |
| `CustomMRQLResult` | string | Custom MRQL result-card template |
| `CustomCSS` | string | Custom CSS injected on resource category pages |
| `SectionConfig` | string | JSON section layout configuration |
| `AutoDetectRules` | string | JSON rules for auto-assigning this category to resources |
| `MetaSchema` | string | JSON schema for metadata validation |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resourceCategory \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Photo",
    "Description": "Photograph files"
  }'
```

### Delete Resource Category

```
POST /v1/resourceCategory/delete?Id={id}
```

### Inline Editing

```
POST /v1/resourceCategory/editName?id={id}
POST /v1/resourceCategory/editDescription?id={id}
```

---

## Template Partials API

Template partials are named, reusable template fragments that any carrier's `Custom*` slot can include. See [Custom Templates](../features/custom-templates.md).

With `-auth` enabled, create, edit and delete require the admin role. Reads are open to any authenticated principal.

### List Template Partials

```
GET /v1/templatePartials
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Description` | string | Filter by description |

#### Example

```bash
curl http://localhost:8181/v1/templatePartials
```

### Create or Update Template Partial

```
POST /v1/templatePartial
POST /v1/templatePartial/edit
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Partial ID (include to update) |
| `Name` | string | Partial name (kebab-case) |
| `Description` | string | Description |
| `Content` | string | Template body |

#### Example

```bash
curl -X POST http://localhost:8181/v1/templatePartial \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"Name": "person-card", "Description": "Shared card markup", "Content": "<div class=\"card\"></div>"}'
```

### Delete Template Partial

```
POST /v1/templatePartial/delete?Id={id}
```

### Inline Editing

```
POST /v1/templatePartial/editDescription?id={id}
```

There is no `editName` route: a partial's name must stay kebab-case, so renames go through the create/update path, which validates it.

### List Template Presets

Return the built-in starter template bundles. Read-only and open.

```
GET /v1/templatePresets
```

---

## Queries API

Queries are saved SQL queries that can be executed to generate custom reports. For database-level write protection, configure `DB_READONLY_DSN` as a truly read-only connection; otherwise query execution is not database-enforced read-only.

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
curl http://localhost:8181/v1/queries
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
| `Description` | string | Query description |

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

The response is `{"columns": [...], "rows": [[...], ...]}` — the `SELECT` list in
its own order, and one array of values per row, index-aligned with it. See
[Saved Queries](../features/saved-queries.md#run-a-query) for the repeated-column
and column-type rules, and for what changed from the previous array-of-objects
shape.

### Get Database Schema

Return the database table and column schema. Useful for writing saved queries.

```
GET /v1/query/schema
```

#### Example

```bash
curl http://localhost:8181/v1/query/schema
```

The response carries a `Cache-Control: max-age=300` header, so HTTP clients and proxies may cache it for 5 minutes. This is an HTTP caching hint, not a server-side cache.

### Inline Editing

```
POST /v1/query/editName?id={id}
POST /v1/query/editDescription?id={id}
```

---

## Global Search API

Search across entity types: resources, notes, groups, tags, categories, queries, relation types, note types, resource categories, and saved MRQL queries.

### Search

```
GET /v1/search
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `q` | string | **Required.** Search query |
| `limit` | integer | Maximum results (default: 20, effective max: 50 — larger values are clamped) |
| `types` | string | Entity types to search (comma-separated: `resource`, `note`, `group`, `tag`, `category`, `query`, `relationType`, `noteType`, `resourceCategory`, `mrqlQuery`) |

#### Example

```bash
# Search everything
curl "http://localhost:8181/v1/search?q=project"

# Search with limit
curl "http://localhost:8181/v1/search?q=project&limit=100"

# Search specific types
curl "http://localhost:8181/v1/search?q=project&types=resource,note"
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

The search stops collecting at a ceiling that is never below `limit` and never above 50, and `total` is counted after that trim, so it saturates at the ceiling. When it does, the response also carries `"totalCapped": true`, meaning `total` is a floor rather than an exact count; the field is omitted otherwise.

---

## Logs API

Query the audit log of system events and entity changes. With `-auth` enabled these endpoints are admin-only.

### List Log Entries

```
GET /v1/logs
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Level` | string | Filter by level (info, warning, error) |
| `Action` | string | Filter by action (create, update, delete, system, progress, plugin, reset) |
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
curl http://localhost:8181/v1/logs

# Filter by action
curl "http://localhost:8181/v1/logs?Action=create"

# Filter by entity type
curl "http://localhost:8181/v1/logs?EntityType=resource"

# Filter errors only
curl "http://localhost:8181/v1/logs?Level=error"
```

#### Response

```json
{
  "logs": [
    {
      "id": 100,
      "createdAt": "2024-01-15T10:30:00Z",
      "level": "info",
      "action": "create",
      "entityType": "resource",
      "entityId": 456,
      "entityName": "photo.jpg",
      "message": "Resource created: photo.jpg",
      "requestPath": "/v1/resource"
    }
  ],
  "totalCount": 1500,
  "page": 1,
  "perPage": 50
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
curl "http://localhost:8181/v1/logs/entity?entityType=resource&entityId=123"
```

---

## Download Queue API

Queue background downloads for remote resources. The queue holds up to 100 jobs and runs as many concurrently as `-max-job-concurrency` allows (default 6), a budget shared with group export and import jobs. In the in-memory queue, completed, failed, and cancelled jobs are retained for 1 hour before eviction; paused jobs are retained for 24 hours. A finished download also leaves a durable row behind, which the [Download History](#download-history) endpoints below read.

The canonical route family is `/v1/jobs/*`. Legacy `/v1/download/*` aliases remain available for compatibility.

With `-auth` enabled, the queue listing, `GET /v1/jobs/get`, the job control endpoints and the SSE stream show only the jobs the caller submitted; admins see all of them, and a job belonging to someone else answers `404` rather than `403`.

### Submit Download

Add URLs to the download queue.

```
POST /v1/jobs/download/submit
```

#### Parameters

Same as `POST /v1/resource/remote`, but always queues for background download.

Submit multiple URLs by separating them with newlines in the `URL` field. Each URL becomes a separate job.

#### Example

```bash
curl -X POST http://localhost:8181/v1/jobs/download/submit \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "URL": "https://example.com/file1.zip\nhttps://example.com/file2.zip",
    "OwnerId": 5
  }'
```

#### Response

`202 Accepted`, with one job object per URL, so the caller learns the job ids to poll:

```json
{
  "queued": true,
  "jobs": [
    {
      "id": "job-123",
      "url": "https://example.com/file1.zip",
      "status": "pending",
      "source": "download"
    }
  ]
}
```

The request answers `400` when the `URL` field is empty or no URL in it parses, `403` when a group-limited caller names an owner, group or note outside its subtree, and `503` when the queue is full.

Legacy alias: `POST /v1/download/submit`

### Get Download Queue

Get all download jobs.

```
GET /v1/jobs/queue
```

#### Example

```bash
curl http://localhost:8181/v1/jobs/queue
```

Legacy alias: `GET /v1/download/queue`

#### Response

```json
{
  "jobs": [
    {
      "id": "job-123",
      "url": "https://example.com/file.zip",
      "status": "downloading",
      "progress": 471859,
      "totalSize": 1048576,
      "progressPercent": 45.0,
      "createdAt": "2024-01-15T10:00:00Z",
      "source": "download"
    }
  ]
}
```

`progress` is the number of bytes downloaded and `progressPercent` is `progress / totalSize * 100`, or `-1` when the total size is unknown.

### Job Operations

| Endpoint | Description |
|----------|-------------|
| `POST /v1/jobs/cancel?id={job_id}` | Cancel a job that has not finished — pending, downloading, processing or paused. A finished job answers `409 Conflict`; an unknown id answers `404`. |
| `POST /v1/jobs/pause?id={job_id}` | Pause a download job. A job in a state that cannot be paused answers `409 Conflict`. |
| `POST /v1/jobs/resume?id={job_id}` | Resume a paused download (restarts from the beginning) |
| `POST /v1/jobs/retry?id={job_id}` | Retry a failed or cancelled download |
| `GET /v1/jobs/get?id={job_id}` | Return one job snapshot by id. Answers `404` for an unknown id or a job the caller may not see. No legacy alias. |
| `POST /v1/jobs/clearCompleted` | Dismiss every finished job the caller can see; active and paused jobs are kept. Answers `{"cleared": N, "ids": ["<job id>", ...]}`, naming the jobs that were removed — which rows are finished is decided when the request is handled, so a caller cannot work it out from its own earlier view of the queue. |

Downloads can fail due to network errors, connection timeouts (default 30s), idle timeouts (default 60s), or exceeding the overall timeout (default 30m). Configure these with the `-remote-connect-timeout`, `-remote-idle-timeout`, and `-remote-overall-timeout` flags.

Legacy aliases remain available under `/v1/download/cancel`, `/v1/download/pause`, `/v1/download/resume`, and `/v1/download/retry`.

### Download Events (SSE)

Stream real-time download status updates via Server-Sent Events.

```
GET /v1/jobs/events
```

#### Example

The server emits **named events**, so you must use `addEventListener` (not `onmessage`):

| Event | Description |
|-------|-------------|
| `init` | Full initial state with all current jobs (`{ jobs: [...], actionJobs: [...] }`) |
| `added` | A new download job was queued |
| `updated` | A download job changed status or progress |
| `removed` | A download job was removed from the queue |
| `action_added` | A plugin action job was created |
| `action_updated` | A plugin action job changed status |
| `action_removed` | A plugin action job was removed |

```javascript
const eventSource = new EventSource('http://localhost:8181/v1/jobs/events');

// Receive full initial state (all current jobs)
eventSource.addEventListener('init', (event) => {
  const { jobs, actionJobs } = JSON.parse(event.data);
  console.log('Initial download jobs:', jobs);
  console.log('Initial action jobs:', actionJobs);
});

// Download job updates
eventSource.addEventListener('added', (event) => {
  const { type, job } = JSON.parse(event.data);
  console.log('New download job:', job);
});

eventSource.addEventListener('updated', (event) => {
  const { type, job } = JSON.parse(event.data);
  console.log('Download job updated:', job.id, job.status, job.progressPercent + '%');
});

eventSource.addEventListener('removed', (event) => {
  const { type, job } = JSON.parse(event.data);
  console.log('Download job removed:', job.id);
});

// Plugin action job updates (prefixed with "action_")
eventSource.addEventListener('action_updated', (event) => {
  const { job } = JSON.parse(event.data);
  console.log('Action job updated:', job);
});
```

Legacy alias: `GET /v1/download/events`

### Download History

The durable record behind the `/downloads` page. Every job whose `source` is `download` is written here when it reaches a terminal state, so it outlives the in-memory queue's 100-job cap, its eviction sweep and a restart. Group export, import and plugin action jobs are not recorded. Rows are scoped to the caller unless the caller is an admin.

```
GET /v1/downloads
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Status` | string[] | Filter by status (`completed`, `failed`, `cancelled`); empty matches all |
| `URL` | string | Partial match over URL and resource name |
| `CreatedBefore` | string | Filter by date |
| `CreatedAfter` | string | Filter by date |
| `SortBy` | string[] | Sort order |

#### Response

```json
{
  "downloads": [],
  "count": 0,
  "page": 1,
  "pageSize": 50
}
```

```
POST /v1/downloads/retry
POST /v1/downloads/delete
```

Both take a list of history row ids and report an outcome per id:

```bash
curl -X POST http://localhost:8181/v1/downloads/retry \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"ids": [12, 13]}'
```

A retry re-runs the row in place when its job is still in the queue and otherwise resubmits it from the stored payload, which is re-validated against the retrying principal's own scope. It is refused for a completed row, for a row whose retry is already claiming or running, and while any queued or running job is already fetching the same URL. Delete removes the queue entry along with the row. When no id could be acted on, the response is `409` with the per-id outcomes still attached.

---

## Series API

A series groups related resources into an ordered sequence (e.g., pages of a scanned document, frames of an animation).

### List Series

```
GET /v1/seriesList
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number for pagination |
| `Name` | string | Filter by name (partial match) |
| `Slug` | string | Filter by slug |
| `CreatedBefore` | string | Filter by creation date (ISO 8601) |
| `CreatedAfter` | string | Filter by creation date (ISO 8601) |
| `SortBy` | string[] | Sort order |

### Get Single Series

```
GET /v1/series?id={id}
```

### Create Series

```
POST /v1/series/create
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | **Required.** Series name |
| `Slug` | string | Optional slug; defaults to the trimmed name |

### Update Series

```
POST /v1/series
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | **Required.** Series ID |
| `Name` | string | New name |
| `Meta` | string | JSON metadata |

### Delete Series

```
POST /v1/series/delete?Id={id}
```

### Inline Editing

```
POST /v1/series/editName?id={id}
```

### Remove Resource from Series

```
POST /v1/resource/removeSeries?id={resourceId}
```

Removes a resource from its series without deleting the series itself.

---

## Meta Keys Endpoints

Get all unique metadata keys used across entities.

| Entity | Endpoint |
|--------|----------|
| Resources | `GET /v1/resources/meta/keys` |
| Notes | `GET /v1/notes/meta/keys` |
| Groups | `GET /v1/groups/meta/keys` |

Each returns an array of objects, one per metadata key, with the key under a `key` field:

```json
[{"key": "author"}, {"key": "source"}, {"key": "date_created"}, {"key": "location"}]
```

---

## MRQL API

Execute, validate, and manage [MRQL](/features/mrql) queries. See the [MRQL feature page](/features/mrql) for the full query language reference.

### Execute Query

```
POST /v1/mrql
```

#### Request Body

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | MRQL query expression (required) |
| `limit` | integer | Max results (flat) or items per bucket (grouped) |
| `page` | integer | Page number |
| `buckets` | integer | Buckets per page (grouped mode only) |
| `offset` | integer | Direct offset for cursor-based bucket paging |
| `params` | object | Bindings for the query's `$name` placeholders; JSON body only |

The same placeholders can also be bound with `param.<name>` form fields or query-string parameters, which win over `params` on a key collision.

#### Query Parameters

| Parameter | Description |
|-----------|-------------|
| `render=1` | Process `CustomMRQLResult` templates server-side and populate `renderedHTML` on each entity |

#### Example

```bash
curl -X POST http://localhost:8181/v1/mrql \
  -H "Content-Type: application/json" \
  -d '{"query": "type = resource AND tags = \"photo\" ORDER BY created DESC", "limit": 20}'
```

Returns an `MRQLResult` for flat queries (with `resources`, `notes`, `groups` arrays) or an `MRQLGroupedResult` for GROUP BY queries (with `mode`, `buckets`, and `rows` fields).

### Validate Query

```
POST /v1/mrql/validate
```

#### Request Body

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | MRQL query expression |

#### Response

```json
{
  "valid": true,
  "errors": []
}
```

When invalid, `errors` contains objects with position and message details.

### Autocomplete

```
POST /v1/mrql/complete
```

#### Request Body

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | Partial MRQL query |
| `cursor` | integer | Cursor position in the query string |

Returns context-aware completion suggestions for the cursor position.

### Explain Query

Return the SQL a query would run, without executing it. See [MRQL](/features/mrql).

```
POST /v1/mrql/explain
```

#### Request Body

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | MRQL query expression |
| `id` | integer | Saved query ID (alternative to `query`) |
| `name` | string | Saved query name (alternative to `query`) |
| `params` | object | Bindings for the query's `$name` placeholders |
| `nativePlan` | boolean | Also run the database's own query planner on the generated SQL. Admin-only under `-auth`; a non-admin request answers `403` |

### Export Results

Stream query results as a downloadable file. See [MRQL](/features/mrql).

```
GET|POST /v1/mrql/export
```

| Parameter | Description |
|-----------|-------------|
| `query` | MRQL query expression |
| `id` / `name` | Saved query to export instead of inline text |
| `format` | `csv` (default) or `json`; a `?format=` query parameter wins over the body |

### Generate Query

Draft an MRQL query from a natural-language prompt. Requires `DEEPSEEK_API_KEY`.

```
POST /v1/mrql/generate
```

#### Request Body

| Field | Type | Description |
|-------|------|-------------|
| `prompt` | string | **Required.** Natural-language description of the query |

### List Saved Queries

```
GET /v1/mrql/saved
GET /v1/mrql/saved?id=N
```

Without `id`, returns a paginated list of all saved MRQL queries. With `id`, returns a single saved query.

| Parameter | Description |
|-----------|-------------|
| `id` | Saved query ID (returns single query) |
| `all=1` | Return all saved queries without pagination |
| `page` | Page number for paginated listing |

### Create Saved Query

```
POST /v1/mrql/saved
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Query name |
| `query` | string | Yes | MRQL query expression |
| `description` | string | No | Query description |

Returns 201 with the created saved query object.

### Update Saved Query

```
PUT /v1/mrql/saved?id=N
```

Same body fields as create. Returns the updated saved query.

### Delete Saved Query

```
POST /v1/mrql/saved/delete?id=N
```

Returns `{"id": N}` on success.

### Run Saved Query

```
POST /v1/mrql/saved/run
```

| Parameter | Description |
|-----------|-------------|
| `id` | Saved query ID |
| `name` | Saved query name (fallback if `id` not found) |
| `limit` | Max results |
| `page` | Page number |
| `buckets` | Buckets per page (grouped mode) |
| `offset` | Direct offset |
| `params` | Bindings for the query's `$name` placeholders; JSON body only (`param.<name>` fields and query-string parameters bind the same placeholders and win on collision) |
| `render=1` | Process `CustomMRQLResult` templates server-side |

Looks up the saved query by ID first, then by name. Revalidates the saved query before execution (schema changes may have invalidated it since save time). Returns the same response format as the execute endpoint.

---

## Admin Stats API

Server and data statistics for monitoring. With `-auth` enabled these endpoints are admin-only.

### Server Stats

```
GET /v1/admin/server-stats
```

Returns server runtime information (memory usage, goroutines, uptime, etc.).

### Data Stats

```
GET /v1/admin/data-stats
```

Returns entity counts and storage statistics.

### Expensive Data Stats

```
GET /v1/admin/data-stats/expensive
```

Returns statistics that require heavier queries (e.g., orphan counts, duplicate detection). Separated from the main stats endpoint to avoid blocking.

### Similarity Maintenance

Rebuild image-similarity data. Both are admin-only under `-auth`.

```
POST /v1/admin/similarity/recompute
POST /v1/admin/similarity/retry-failed
```

`recompute` submits a background job that rebuilds the stored similarity pairs and answers `{"jobId": "..."}`, or `409 Conflict` when one is already running. `retry-failed` resets failed hash rows so the backfill worker retries them, and answers `{"reset": N}`.

### Runtime Settings

`GET /v1/admin/settings` and `PUT|DELETE /v1/admin/settings/{key}` read and change the settings that can be edited without a restart. See [Runtime Settings](../configuration/runtime-settings.md).

---

## Users, Accounts & Authentication API

Session and token authentication, admin user management, and self-service account endpoints. These are the whole programmatic surface of the [authentication feature](../features/authentication.md).

### Log In

```
POST /v1/auth/login
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `username` | string | **Required.** Account username |
| `password` | string | **Required.** Account password |

Accepts a JSON body or a form body. On success, sets the session cookie and returns the user. Answers `401` for a bad credential or a disabled account, `429` when login rate-limiting is enabled and the limit is hit, and `503` with `Retry-After` when the session could not be minted because of contention.

#### Example

```bash
curl -X POST http://localhost:8181/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "hunter2hunter2"}'
```

### Log Out

```
POST /v1/auth/logout
```

Revokes the current session and clears the cookie. Returns `{"ok": true}`.

### Current Principal

```
GET /v1/auth/me
```

#### Response

```json
{
  "authEnabled": true,
  "userId": 3,
  "username": "alice",
  "role": "editor",
  "scopeGroupId": null,
  "isAdmin": false,
  "canWrite": true,
  "superUser": false,
  "csrfToken": "..."
}
```

`csrfToken` is empty for Bearer-authenticated requests, which are exempt from the CSRF check.

### User Administration

Admin-only under `-auth`.

```
GET  /v1/users
POST /v1/users
GET  /v1/user?id={id}
POST /v1/user
POST /v1/user/delete
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | User ID (required on update and delete) |
| `username` | string | Account username |
| `displayName` | string | Display name |
| `password` | string | Password (at least 8 characters, at most 72 bytes) |
| `role` | string | `admin`, `editor`, `user` or `guest` |
| `scopeGroupId` | integer | Group subtree the account is confined to |
| `disabled` | boolean | Lock the account |

`GET /v1/users` accepts `offset` and `limit`; a `limit` of `0` returns every account. Deleting, demoting or disabling the last enabled administrator is refused with `409 Conflict`.

#### Example

```bash
curl -X POST http://localhost:8181/v1/users \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"username": "bob", "password": "correcthorse", "role": "user"}'
```

### Self-Service Account

These act on the caller's own record and are open to any authenticated principal, including guests.

| Endpoint | Description |
|----------|-------------|
| `POST /v1/account/password` | Change own password. Takes `currentPassword` and `newPassword`; revokes the caller's other sessions. |
| `GET /v1/account/tokens` | List the caller's API tokens. |
| `POST /v1/account/tokens` | Mint an API token. Takes `name` and an optional `expiresIn` Go duration; the raw token is returned exactly once. |
| `POST /v1/account/tokens/delete` | Revoke one of the caller's tokens by `id`. |
| `GET /v1/account/settings` | Read the caller's UI settings. |
| `PUT /v1/account/settings/{key}` | Upsert one setting. |
| `DELETE /v1/account/settings/{key}` | Remove one setting. |

#### Example

```bash
curl -X POST http://localhost:8181/v1/account/tokens \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"name": "laptop", "expiresIn": "720h"}'
```

---

## Timeline Endpoints

Each major entity type has a timeline endpoint that returns entities grouped by time period, suitable for timeline/calendar views.

| Entity | Endpoint |
|--------|----------|
| Resources | `GET /v1/resources/timeline` |
| Notes | `GET /v1/notes/timeline` |
| Groups | `GET /v1/groups/timeline` |
| Tags | `GET /v1/tags/timeline` |
| Categories | `GET /v1/categories/timeline` |
| Queries | `GET /v1/queries/timeline` |

Each accepts the same query parameters as the corresponding list endpoint, plus three timeline-specific ones: `granularity` (`yearly`, `monthly` or `weekly`, default `monthly`), `anchor` (`YYYY-MM-DD`, default today) and `columns` (default 15; a value outside 1 to 60 is ignored rather than clamped). See [Timeline Parameters](../features/timeline-view.md#timeline-parameters).
