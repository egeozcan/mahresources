---
sidebar_position: 11
title: Plugin Actions
---

# Plugin Actions

Actions let plugins contribute operations that appear in the UI alongside Resources, Notes, and Groups. Actions can collect user input through typed parameters, run synchronously or asynchronously, and target specific entity types or content types.

## Registering an Action

Register actions during `init()` using `mah.action(table)`:

```lua
function init()
    mah.action({
        id = "colorize",
        label = "Colorize Image",
        entity = "resource",
        placement = {"detail", "card"},
        filters = {
            content_types = {"image/jpeg", "image/png"}
        },
        params = {
            { name = "style", type = "select", label = "Style", options = {"realistic", "artistic"}, default = "realistic" },
            { name = "intensity", type = "number", label = "Intensity", min = 1, max = 100, default = 50 }
        },
        async = true,
        confirm = "This will process the image. Continue?",
        handler = function(ctx)
            local resource = mah.db.get_resource(ctx.entity_id)
            -- process the resource...
            mah.job_progress(ctx.job_id, 50, "Processing...")
            -- ...
            mah.job_complete(ctx.job_id, { message = "Done" })
        end
    })
end
```

## Registration Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | Yes | -- | Unique ID within the plugin (normalized to lowercase) |
| `label` | string | Yes | -- | Display label in the UI |
| `entity` | string | Yes | -- | Target entity: `"resource"`, `"note"`, or `"group"` |
| `handler` | function | Yes | -- | Lua function called when the action runs |
| `description` | string | No | `""` | Optional description |
| `icon` | string | No | `""` | Optional icon identifier |
| `placement` | table | No | `{"detail"}` | Where to show: `"detail"`, `"card"`, `"bulk"` |
| `filters` | table | No | match all | Content-type, category, or note-type filters |
| `params` | table | No | none | User input parameter definitions |
| `async` | boolean | No | `false` | Run asynchronously via the job system |
| `confirm` | string | No | `""` | Confirmation message shown before execution |
| `bulk_max` | number | No | `0` | Maximum entities for bulk execution (0 = unlimited) |

Registering a duplicate `id` within the same plugin raises a Lua error.

## Action Parameters

Parameters define the input fields shown to the user before the action runs.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Parameter key passed to the handler |
| `type` | string | Yes | `"text"`, `"textarea"`, `"number"`, `"select"`, `"boolean"`, `"hidden"` |
| `label` | string | Yes | Display label |
| `required` | boolean | No | Whether the field must be filled |
| `default` | any | No | Default value |
| `options` | table | No | Choices for `"select"` type |
| `min` | number | No | Minimum value for `"number"` type |
| `max` | number | No | Maximum value for `"number"` type |
| `step` | number | No | Step increment for `"number"` type |

## Action Filters

Filters control which entities see the action. Empty filters match everything.

```lua
filters = {
    content_types = {"image/jpeg", "image/png", "image/webp"},  -- Resource content types
    category_ids = {5, 12},                                      -- Group Category IDs
    note_type_ids = {3}                                          -- Note Type IDs
}
```

| Filter | Entity | Description |
|--------|--------|-------------|
| `content_types` | Resource | Match Resources with these MIME types |
| `category_ids` | Group | Match Groups with these Category IDs |
| `note_type_ids` | Note | Match Notes with these Note Type IDs |

If a filter is set but the entity lacks the filtered field, the action does not match.

## Placement

| Placement | Location |
|-----------|----------|
| `detail` | Entity detail page (single entity) |
| `card` | Entity card in list views (single entity) |
| `bulk` | Bulk action bar (multiple selected entities) |

## Synchronous Execution

Sync actions (the default) run within a single request-response cycle. The handler receives a context table and returns a result table.

**Timeout**: 5 seconds.

```lua
mah.action({
    id = "tag-by-type",
    label = "Auto-Tag by Type",
    entity = "resource",
    handler = function(ctx)
        local resource = mah.db.get_resource(ctx.entity_id)
        -- do something quick...
        return { success = true, message = "Tagged" }
    end
})
```

### Handler Context (Sync)

| Field | Type | Description |
|-------|------|-------------|
| `entity_id` | number | ID of the target entity |
| `params` | table | User-supplied parameter values |
| `settings` | table | Plugin settings |

### ActionResult

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | Whether the action succeeded |
| `message` | string | Message displayed to the user |
| `redirect` | string | Optional URL to redirect to after completion |
| `data` | table | Optional additional data |

## Asynchronous Execution

Async actions (`async = true`) run in a background goroutine via the job system. The API returns immediately with a `job_id`.

**Timeout**: 5 minutes. **Max concurrent**: 3 async actions across all plugins.

```lua
mah.action({
    id = "process-video",
    label = "Process Video",
    entity = "resource",
    async = true,
    handler = function(ctx)
        mah.job_progress(ctx.job_id, 10, "Downloading...")
        -- long-running work...
        mah.job_progress(ctx.job_id, 50, "Processing...")
        -- more work...
        mah.job_complete(ctx.job_id, { message = "Video processed" })
    end
})
```

### Handler Context (Async)

Same as sync, plus:

| Field | Type | Description |
|-------|------|-------------|
| `job_id` | string | Job ID for progress reporting |

### Job Progress Control

| Function | Description |
|----------|-------------|
| `mah.job_progress(job_id, percent, message)` | Report progress (0-100). SSE updates throttled to 200ms. |
| `mah.job_complete(job_id, result_table)` | Mark job as completed. Sets progress to 100. |
| `mah.job_fail(job_id, error_message)` | Mark job as failed. |

If the handler returns without calling `mah.job_complete` or `mah.job_fail`, the return value is parsed as an `ActionResult` and the job is updated accordingly.

### Abort

Call `mah.abort(reason)` from any handler to abort the action:

```lua
handler = function(ctx)
    local resource = mah.db.get_resource(ctx.entity_id)
    if not resource then
        mah.abort("Resource not found")
    end
    -- ...
end
```

This returns `{ success = false, message = reason }`.

## API Endpoints

### List Available Actions

```
GET /v1/plugin/actions
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `entity` | string | Required: `"resource"`, `"note"`, or `"group"` |
| `content_type` | string | Optional: filter by content type |
| `category_id` | uint | Optional: filter by Category ID |
| `note_type_id` | uint | Optional: filter by Note Type ID |

```bash
curl "http://localhost:8181/v1/plugin/actions?entity=resource&content_type=image/jpeg"
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
    "params": { "style": "realistic", "intensity": 75 }
}
```

- **Sync actions**: Returns `200 OK` with `ActionResult`
- **Async actions**: Returns `202 Accepted` with `{ "job_id": "abc123..." }`
- **Bulk** (multiple `entity_ids`): Returns an array of results or job IDs. Respects `bulk_max`.

### Get Action Job Status

```
GET /v1/jobs/action/job?id={jobId}
```

```bash
curl "http://localhost:8181/v1/jobs/action/job?id=abc123def456"
```

Returns the current job state including status, progress, message, and result.
