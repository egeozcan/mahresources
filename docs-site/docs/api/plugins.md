---
sidebar_position: 6
title: Plugins API
---

# Plugins API

Manage plugins, execute actions, and monitor jobs through the REST API.

## Plugin Management

With `-auth` enabled, every endpoint in this section is admin-only.

### List Plugins

```
GET /v1/plugins/manage
```

Returns all discovered plugins with their current state (enabled/disabled), metadata, and settings.

```bash
curl http://localhost:8181/v1/plugins/manage
```

```json
[
  {
    "name": "image-processor",
    "version": "1.0.0",
    "description": "Processes images using external APIs",
    "enabled": true,
    "legacy": false,
    "api_version": 1,
    "capabilities": ["db:read", "http", "image"],
    "capability_labels": {
      "db:read": "Read your library (resources, notes, groups, tags)",
      "http": "Make outbound network requests",
      "image": "Transform images"
    },
    "network": ["api.example.com"],
    "allow_private_hosts": false,
    "dependencies": ["shared-utils"],
    "min_app_version": "1.2.0",
    "settings": [
      { "name": "api_key", "type": "password", "label": "API Key", "required": true }
    ],
    "values": { "api_key": "sk-abc123" }
  }
]
```

`capabilities` is the effective set (`db:write` implies `db:read`), and `capability_labels` gives the human sentence for each one. `legacy` is true for a plugin that declares no manifest at all, which keeps the full `mah` surface. An empty `network` means any public host, the broadest policy rather than the absence of network access. `values` holds the saved setting values.

### Enable Plugin

```
POST /v1/plugin/enable
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name to enable |

```bash
curl -X POST http://localhost:8181/v1/plugin/enable \
  -d "name=image-processor"
```

Required settings must be saved before enabling. Returns an error if required settings are missing. Enabling also records the plugin's declared capabilities as consent, so a plugin that later widens its manifest will not load until an operator enables it again. See [Plugin Permissions](../features/plugin-permissions.md).

### Disable Plugin

```
POST /v1/plugin/disable
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name to disable |

```bash
curl -X POST http://localhost:8181/v1/plugin/disable \
  -d "name=image-processor"
```

Disabling removes all hooks, injections, pages, menus, and actions. In-flight async actions are awaited before the Lua VM is closed. Disable is refused when another enabled plugin depends on this one; the error names the dependents, which must be disabled first.

### Save Plugin Settings

```
POST /v1/plugin/settings?name={pluginName}
Content-Type: application/json
```

| Parameter | Location | Type | Description |
|-----------|----------|------|-------------|
| `name` | query or form | string | Plugin name |
| (body) | JSON body | object | Setting key-value pairs |

```bash
curl -X POST "http://localhost:8181/v1/plugin/settings?name=image-processor" \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "sk-abc123",
    "model": "quality",
    "max_size": 2048
  }'
```

Settings are validated against the plugin's declared setting definitions. Unknown keys are ignored. Boolean settings must be native JSON booleans (`true`/`false`, not strings). Number settings must be native JSON numbers (`2048`, not `"2048"`). A validation failure answers `422 Unprocessable Entity` with `{"errors": [...]}`.

The body replaces the whole settings object rather than merging into it, so send every setting on each save: a declared setting omitted from the request is cleared. The request body is limited to 64 KB.

### Purge Plugin Data

Delete all key-value store data for a plugin. The plugin must be disabled before purging.

```
POST /v1/plugin/purge-data
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name to purge data for |

```bash
curl -X POST http://localhost:8181/v1/plugin/purge-data \
  -d "name=image-processor"
```

**Response:**

```json
{
  "ok": true,
  "name": "image-processor"
}
```

:::warning
Purging deletes all KV store entries for the plugin. This action is irreversible. The plugin must be disabled first; attempting to purge an enabled plugin returns an error.
:::

### Set Scoped Access

Open one plugin to group-limited accounts, or close it again.

```
POST /v1/plugin/scopedAccess
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name |
| `allowed` | string | `1`, `true`, `on` or `yes` opens the plugin; anything else, including an absent value, closes it |

```bash
curl -X POST http://localhost:8181/v1/plugin/scopedAccess \
  -d "name=image-processor" \
  -d "allowed=true"
```

**Response:**

```json
{
  "ok": true,
  "name": "image-processor",
  "allow_scoped_principals": true
}
```

Group-limited users and guests are refused a plugin's pages, JSON endpoints, block and display rendering, and action runs until this is turned on for that plugin. See [Plugin Permissions](../features/plugin-permissions.md).

### List Schedules

```
GET /v1/plugin/schedules?name={pluginName}
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name |

```bash
curl "http://localhost:8181/v1/plugin/schedules?name=image-processor"
```

```json
[
  {
    "scheduleId": "nightly-rollup",
    "pluginName": "image-processor",
    "everySeconds": 3600,
    "overlap": "skip",
    "nextDueAt": "2025-03-01T11:00:00Z",
    "runs": 12,
    "lastStatus": "completed",
    "lastError": "",
    "lastRunAt": "2025-03-01T10:00:04Z",
    "owned": true,
    "registered": true
  }
]
```

`lastRunAt` is present once the schedule has run. `lastStatus` is `completed` or `failed`. `registered` is false when the row exists but the plugin no longer declares that id, which is what a disabled plugin, a renamed schedule and a removed `mah.schedule` call all look like. `owned` is false when the row carries no creator, at which point the schedule has stopped rather than merely lost its label.

### Run a Schedule

```
POST /v1/plugin/schedule/run
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name |
| `scheduleId` | string | The id the plugin passed to `mah.schedule` |

```bash
curl -X POST http://localhost:8181/v1/plugin/schedule/run \
  -d "name=image-processor" \
  -d "scheduleId=nightly-rollup"
```

**Response:**

```json
{
  "ok": true,
  "name": "image-processor",
  "scheduleId": "nightly-rollup",
  "started": true
}
```

The response is sent once the run has *started*, not when it has finished; the run then reports itself through the `action_*` events on the SSE stream. `404` means there is no such row. `409` means the plugin no longer declares that id, the row has no owner, or the claim is already held by a run in progress.

### List Scheduled Downloads

```
GET /v1/plugin/scheduled-downloads?name={pluginName}
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Plugin name |

```bash
curl "http://localhost:8181/v1/plugin/scheduled-downloads?name=image-processor"
```

```json
[
  {
    "id": 17,
    "pluginName": "image-processor",
    "url": "https://example.com/archive.zip",
    "dueAt": "2025-03-01T12:00:00Z",
    "status": "pending",
    "jobId": "",
    "lastError": "",
    "attempts": 0,
    "owned": true,
    "createdAt": "2025-03-01T10:00:00Z",
    "updatedAt": "2025-03-01T10:00:00Z"
  }
]
```

Rows are one-shot deferred host downloads created by `mah.download.submit` with
`delay` or `start_at`. `status` is `pending`, `submitted`, `failed` or
`cancelled`. A `submitted` row carries the queue `jobId`; `claimedAt` appears
briefly while a scheduler tick holds the submit claim. `owned: false` on a
pending row means the submitting user was deleted and the row has stopped
rather than firing as an administrator. Naming a plugin that has no rows returns
an empty array.

### Cancel a Scheduled Download

```
POST /v1/plugin/scheduled-downloads/cancel
Content-Type: application/x-www-form-urlencoded
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Scheduled download row id |

```bash
curl -X POST http://localhost:8181/v1/plugin/scheduled-downloads/cancel \
  -d "id=17"
```

**Response:**

```json
{
  "ok": true,
  "id": 17,
  "status": "cancelled"
}
```

Only pending rows can be cancelled. A row that has already been submitted,
failed or cancelled answers `409 Conflict`.

## Plugin Actions

### List Available Actions

```
GET /v1/plugin/actions
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `entity` | string | Yes | `"resource"`, `"note"`, or `"group"` |
| `content_type` | string | No | Filter by Resource content type |
| `category_id` | uint | No | Filter by Group Category ID |
| `note_type_id` | uint | No | Filter by Note Type ID |

```bash
curl "http://localhost:8181/v1/plugin/actions?entity=resource&content_type=image/jpeg"
```

```json
[
  {
    "plugin_name": "image-processor",
    "id": "colorize",
    "label": "Colorize Image",
    "entity": "resource",
    "placement": ["detail", "card"],
    "async": true,
    "params": [
      { "name": "style", "type": "select", "label": "Style", "options": ["realistic", "artistic"] }
    ]
  }
]
```

### Run an Action

```
POST /v1/jobs/action/run
Content-Type: application/json
```

```json
{
    "plugin": "image-processor",
    "action": "colorize",
    "entity_ids": [42],
    "params": {
        "style": "realistic"
    }
}
```

**Sync actions** return `200 OK`:

```json
{
    "success": true,
    "message": "Image colorized",
    "redirect": "/resource?id=42"
}
```

**Async actions** return `202 Accepted`:

```json
{
    "job_id": "a1b2c3d4e5f6g7h8"
}
```

**Bulk execution** (multiple `entity_ids`) returns wrapped results:

- Sync actions: `{ "results": [...] }` -- one entry per submitted entity, in the
  order they were submitted. A bulk run is not atomic, so an entity whose
  handler failed carries `success: false` and a message in its own slot and the
  response is still `200`; a single-entity run keeps its error status instead.
- Async actions: `{ "job_ids": [...] }`

The `bulk_max` limit on the action registration is enforced, as is the
deployment-wide `-max-action-entities` cap (default 1000). Exceeding either is a
`400`.

Parameter validation, `entity_ref` resolution and the action's own entity filters
each answer `400` with `{"errors": [...]}` rather than the generic error shape, and
a filter mismatch on any one entity vetoes the whole batch. A group-limited caller
gets `403` when the plugin has not been opened to scoped accounts, or when any
named entity lies outside its subtree. See
[Plugin Permissions](../features/plugin-permissions.md).

### Get Action Job Status

```
GET /v1/jobs/action/job
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Job ID |

```bash
curl "http://localhost:8181/v1/jobs/action/job?id=a1b2c3d4e5f6g7h8"
```

```json
{
    "id": "a1b2c3d4e5f6g7h8",
    "source": "plugin",
    "pluginName": "image-processor",
    "actionId": "colorize",
    "label": "Colorize Image",
    "entityId": 42,
    "entityType": "resource",
    "status": "running",
    "progress": 65,
    "message": "Applying color model...",
    "createdAt": "2025-03-01T10:30:00Z"
}
```

## Plugin Block Rendering

```
GET /v1/plugins/{pluginName}/block/render
```

Renders a plugin-defined block type as an HTML fragment. The block editor's frontend calls this endpoint to display plugin blocks.

| Parameter | Location | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `pluginName` | path | string | Yes | The plugin that registered the block type |
| `blockId` | query | integer | Yes | The block to render |
| `mode` | query | string | Yes | `"view"` or `"edit"` |

```bash
curl "http://localhost:8181/v1/plugins/my-plugin/block/render?blockId=42&mode=view"
```

Returns `text/html` content. The block's type must start with `plugin:<pluginName>:`, otherwise a `400` error is returned.

See [Custom Block Types](../features/custom-block-types.md#plugin-block-render-endpoint) for details on how plugin block rendering works.

## Plugin Display Rendering

```
POST /v1/plugins/{pluginName}/display/render
```

Renders a plugin-defined display type as an HTML fragment. The schema-driven metadata display component calls this endpoint when a schema property has `x-display: "plugin:<pluginName>:<type>"`.

**Request body** (JSON):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | The display type name (without the `plugin:<name>:` prefix) |
| `value` | object | Yes | The metadata value to render |
| `schema` | object | No | The JSON Schema of the property |
| `field_path` | string | No | Dot-notation path of the field |
| `field_label` | string | No | Display label of the field |
| `entity_type` | string | No | Entity the metadata belongs to, passed to the renderer as `ctx.entity_type` |
| `entity_id` | integer | No | Id of that entity, passed as `ctx.entity_id`; a value above 2^53-1 is rejected with `400` |

```bash
curl -X POST "http://localhost:8181/v1/plugins/my-plugin/display/render" \
  -H "Content-Type: application/json" \
  -d '{"type":"color-swatch","value":{"hex":"#4f46e5","name":"Indigo"}}'
```

Returns `text/html` content. The plugin must have registered the display type via `mah.display_type()`, otherwise a `500` error is returned. Render timeout is 5 seconds.

See [Plugin Lua API](../features/plugin-lua-api.md#mahdisplay_type----custom-display-renderers) for how to register display types.

## Plugin Pages

```
GET|POST /plugins/{pluginName}/{path}
```

Plugin-registered pages are served at this path. The response is HTML generated by the plugin's page handler.

Two first segments under `/plugins/` are reserved and cannot be a plugin page path: `manage` is the plugin management page, and `public` serves a plugin's own static assets while that plugin is enabled. See [Static Assets](../features/plugin-hooks.md#static-assets).

```bash
curl http://localhost:8181/plugins/image-processor/dashboard
```

## Plugin JSON API Endpoints

```
GET|POST|PUT|DELETE /v1/plugins/{pluginName}/{path}
```

Plugin-registered JSON API endpoints. Unlike plugin pages (which return HTML), these return `application/json` responses.

```bash
# GET endpoint
curl http://localhost:8181/v1/plugins/my-plugin/stats

# POST with JSON body
curl -X POST http://localhost:8181/v1/plugins/my-plugin/webhook \
  -H "Content-Type: application/json" \
  -d '{"event": "test"}'
```

**Success Response:**

```json
{
  "total_notes": 42,
  "query": { "page": "1" }
}
```

**Error Responses:**

| Status | Condition | Body |
|--------|-----------|------|
| 204 | Handler set no response body | (no content) |
| 400 | Handler called `mah.abort()`, unless it had already called `ctx.status()`, in which case that code is used | `{"error": "reason"}` |
| 404 | Plugin not found or path not registered | `{"error": "plugin not found"}` or `{"error": "endpoint not found"}` |
| 405 | Path exists but method not registered | `{"error": "method not allowed"}` |
| 413 | Request body exceeds 1 MB | `{"error": "request body too large"}` |
| 500 | Handler runtime error | `{"error": "internal plugin error"}` |
| 504 | Handler exceeded timeout | `{"error": "handler timed out after <duration>"}` (for example `handler timed out after 5s`) |

See [Plugin Lua API Reference](../features/plugin-lua-api.md) for the `mah.api()` registration function.

## Unified Job Endpoints

The queue endpoint returns every job the download manager runs: downloads, group exports and imports, and admin similarity recomputes. Plugin action jobs are not among them; they appear in the SSE event stream alongside download events.

### List Jobs

```
GET /v1/jobs/queue
```

```bash
curl http://localhost:8181/v1/jobs/queue
```

Returns the retained jobs currently held in the queue manager, including completed, failed, cancelled, or paused jobs until their retention window expires. Plugin action jobs are not included here; they are available only via the SSE event stream below.

### SSE Event Stream

```
GET /v1/jobs/events
```

Server-Sent Events stream for all job types. The stream uses SSE event names to distinguish job types.

**Download events** use event names `added`, `updated`, `removed`:

```
event: updated
data: {"type":"updated","job":{"id":"abcd1234","status":"downloading","progress":45}}
```

**Plugin action events** use event names `action_added`, `action_updated`, `action_removed`:

```
event: action_updated
data: {"job":{"id":"a1b2c3d4e5f6g7h8","source":"plugin","status":"running","progress":65}}
```

**Initialization**: On connect, an `init` event is sent with all current jobs:

```
event: init
data: {"jobs":[...],"actionJobs":[...]}
```
