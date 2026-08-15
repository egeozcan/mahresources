---
sidebar_position: 11
title: Plugin Actions
---

# Plugin Actions

Actions are plugin-contributed operations that appear in the UI alongside Resources, Notes, and Groups, collecting user input through typed parameters and running synchronously or asynchronously against specific entity types or content types.

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
| `type` | string | Yes | `"text"`, `"textarea"`, `"number"`, `"select"`, `"boolean"`, `"hidden"`, `"info"`, `"entity_ref"` |
| `label` | string | Yes | Display label |
| `required` | boolean | No | Whether the field must be filled |
| `default` | any | No | Default value |
| `options` | table | No | Choices for `"select"` type |
| `min` | number | No | Minimum value for `"number"` type |
| `max` | number | No | Maximum value for `"number"` type |
| `step` | number | No | Step increment for `"number"` type |

## Entity Reference Parameters

The `entity_ref` param type lets a plugin action accept references to one or more resources, notes, or groups as additional input. Use cases: an image-edit action that takes multiple source images, a "merge two notes" action, a "tag groups by another group's tags" action.

### Schema

```lua
{
    name        = "extra_images",
    type        = "entity_ref",
    label       = "Additional Images",
    entity      = "resource",                                    -- "resource" | "note" | "group"
    multi       = true,                                          -- false → single ID; true → array of IDs
    required    = false,
    min         = 0,                                             -- multi only; omit for no minimum
    max         = 9,                                             -- multi only; omit or set to 0 for no maximum
    default     = "trigger",                                     -- "trigger" | "selection" | "both" | ""
    filters     = { content_types = {"image/jpeg", "image/png"} }, -- optional; inherits action.filters when omitted
    show_when   = { model = {"flux2", "nanobanana2"} },         -- standard show_when
    description = "Reference images sent alongside the source.",
}
```

### Behavior

- The picker UI opens layered over the action modal. It applies the effective filter (per-param `filters` if set, else inherits `action.filters`).
- The handler receives the IDs as `ctx.params.<name>` — a Lua number for `multi=false`, a Lua table of numbers for `multi=true`. Server-side validation guarantees every ID exists and matches the filter at request time.
- `default` controls what the picker is prefilled with:
  - `"trigger"` (default when omitted) — the entity the action was launched from.
  - `"selection"` — IDs from the current bulk-selection store.
  - `"both"` — union of trigger and selection (requires `multi=true`).
  - `""` — empty; user picks every entry.
- Trigger and selection are silently ignored if `param.entity` does not match the action's launch entity type. (Example: an action declared `entity = "resource"` with an `entity_ref entity = "group" default = "trigger"` will open with an empty picker on resource pages, since the trigger resource ID is not a valid group ID.)

### Constraints

- `default = "both"` requires `multi = true`.
- `entity` must be one of `"resource"`, `"note"`, `"group"`. Other values are rejected at plugin load time.

## `show_when`

`show_when` gates a param on the values of other params. Every key must match
(AND-joined), and an array value means any-of:

```lua
show_when = { model = {"flux2", "flux2pro", "nanobanana2"} }
-- Visible when model is any of the listed values.

show_when = { model = "b", advanced = true }
-- Visible only when BOTH hold.
```

Scalar values use strict equality, except that numbers compare numerically
regardless of how they arrived.

### It is enforced on the server, not just drawn in the browser

Visibility is resolved from the submitted values **before** validation runs:

- A hidden param is **not validated**, so `required = true` combines with
  `show_when`. A mandatory field inside a branch is enforced exactly when the
  user is in that branch -- previously the pair was rejected at plugin load and
  the workaround was to mark nothing required and re-check in Lua.
- A hidden param's submitted value is **dropped before the handler sees it**.
  The modal already strips hidden params, but a direct API caller does not, so a
  handler that assumed `show_when` implied absence was wrong for that caller.
- A param whose controlling param is **absent** from the request counts as
  hidden. That is the one point where the server can disagree with the browser,
  which evaluates against its live form state, and it errs toward not
  validating rather than toward rejecting a field the user was never shown.

### Chaining

A `show_when` may name a param that is itself gated -- a sub-mode selector shown
only for one model, with fields shown only for one sub-mode -- provided the
dependent **repeats the controller's own conditions**:

```lua
{name = "upscale_mode", type = "select", options = {"factor", "target"},
    show_when = {model = "seedvr"}},
{name = "upscale_factor", type = "number",
    show_when = {model = "seedvr", upscale_mode = "factor"}},   -- repeats model
```

Omitting the repeated condition is rejected at plugin load. Without it the
dependent could be visible while its controller is hidden, and a hidden
controller is stripped from the request -- so the server would see no value for
it, conclude the dependent was hidden too, and silently discard what the user
typed into a field they were shown.

```lua
params = {
    { name = "mode", type = "select", label = "Mode", options = {"simple", "scheduled"}, default = "simple" },
    -- Mandatory, but only in the branch that shows it.
    { name = "publish_at", type = "text", label = "Publish at", required = true,
      show_when = { mode = "scheduled" } },
}
```

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

The `bulk` placement renders a button only on the Resource and Group list pages. The Note list page does not expose bulk plugin actions, so a `bulk` action declared with `entity = "note"` shows no button there. Running it directly via `POST /v1/jobs/action/run` with multiple note IDs still works.

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
| `job_id` | string | Optional job ID a sync handler can return as a poll handle |
| `data` | table | Optional additional data |

### How the modal reports a sync result

`success` is honoured, so a handler can refuse and say why:

- `success = true` -- the result is announced in a `role="status"` box and the
  page reloads after 1.5s.
- `success = false` (including `mah.abort` on the sync path) -- the `message` is
  shown as an error in a `role="alert"` box, the modal stays open, and the page
  does **not** reload, so the reason stays readable.
- A **bulk** run answers with one result per selected entity. The modal reports
  how many failed and lists them by entity ID. If some succeeded, the page
  reloads when the reader dismisses the result rather than under them while they
  are still reading it.

Return a message with a refusal -- it is the only thing the user will see:

```lua
handler = function(ctx)
    if not mah.get_setting("api_key") then
        return { success = false, message = "Set the API key in plugin settings first" }
    end
    -- ...
end
```

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

If the handler returns without calling `mah.job_complete` or `mah.job_fail`, an async job is always marked completed with progress 100. Only a returned `message` string is read from the table; a `success = false` field is ignored on the async path (unlike the sync path, which honors it). To fail an async action, call `mah.job_fail` or `mah.abort`.

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

On the synchronous path this returns `{ success = false, message = reason }`. On the asynchronous path there is no `success` field: the job is marked failed with its message set to the reason.

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
- **Bulk** (multiple `entity_ids`): Returns `{ "results": [...] }` for sync actions or `{ "job_ids": [...] }` for async actions. Respects `bulk_max`.

### Get Action Job Status

```
GET /v1/jobs/action/job?id={jobId}
```

```bash
curl "http://localhost:8181/v1/jobs/action/job?id=abc123def456"
```

Returns the current job state including status, progress, message, and result.

## Related Pages

- [Plugin System](./plugin-system.md) -- plugin installation, configuration, and lifecycle
- [Plugin Hooks](./plugin-hooks.md) -- hook registration, injections, custom pages, and menus
- [Job System](./job-system.md) -- unified job listing, SSE events, and cleanup behavior
- [Plugin Lua API](./plugin-lua-api.md) -- full Lua API reference for the `mah` module
