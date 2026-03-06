---
sidebar_position: 6
title: Plugins API
---

# Plugins API

Manage plugins, execute actions, and monitor jobs through the REST API.

## Plugin Management

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
    "settings": [
      { "name": "api_key", "type": "password", "label": "API Key", "required": true }
    ]
  }
]
```

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

Required settings must be saved before enabling. Returns an error if required settings are missing.

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

Disabling removes all hooks, injections, pages, menus, and actions. In-flight async actions are awaited before the Lua VM is closed.

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

Settings are validated against the plugin's declared setting definitions. Unknown keys are ignored. Boolean settings accept `"true"` and `"false"` strings. Number settings must be valid numeric strings.

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

**Bulk execution** (multiple `entity_ids`) returns an array of results or job IDs. The `bulk_max` limit on the action registration is enforced.

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

## Plugin Pages

```
GET|POST /plugins/{pluginName}/{path}
```

Plugin-registered pages are served at this path. The response is HTML generated by the plugin's page handler.

```bash
curl http://localhost:8181/plugins/image-processor/dashboard
```

## Unified Job Endpoints

These endpoints combine download queue jobs and plugin action jobs.

### List All Jobs

```
GET /v1/jobs/queue
```

```bash
curl http://localhost:8181/v1/jobs/queue
```

Returns all active jobs from both the download queue and async plugin actions.

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
