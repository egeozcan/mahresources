# Plugin Declarative Actions & Unified Jobs Panel

## Problem

Plugins currently inject raw HTML via `mah.inject(slot, fn)` to add buttons and actions to the GUI. This means:
- Plugins break when the GUI changes (they own the rendering)
- No standard filtering by entity type, content type, category, etc.
- No standard UI for collecting action parameters from the user
- No visibility into long-running plugin operations

## Solution

A new `mah.action()` Lua API for declarative action registration. Plugins declare metadata (entity type, filters, parameters, handler). Mahresources owns all rendering — standard buttons, auto-generated modal forms, and a unified Jobs panel for tracking execution.

## Action Declaration (Lua API)

```lua
mah.action({
    -- Identity
    id = "ai-image-edit",
    label = "Edit with AI",
    description = "Transform this image using an AI model",
    icon = "sparkles",                 -- optional, predefined icon set

    -- Where it appears
    entity = "resource",               -- "resource" | "note" | "group"
    placement = { "detail", "card", "bulk" },

    -- Filtering: action only shows when these match
    filters = {
        content_types = { "image/png", "image/jpeg", "image/webp" },  -- resource only
        -- category_ids = { 1, 2 },     -- group only
        -- note_type_ids = { 3 },        -- note only
    },

    -- Parameters shown in the modal form
    params = {
        { name = "prompt", type = "text", label = "Prompt", required = true },
        { name = "model", type = "select", label = "Model",
          options = { "model-a", "model-b" }, default = "model-a" },
        { name = "strength", type = "number", label = "Strength",
          default = 0.7, min = 0, max = 1, step = 0.1 },
    },

    -- Execution
    async = true,
    handler = function(ctx)
        -- ctx.entity_id, ctx.entity, ctx.params, ctx.settings, ctx.job_id
    end,

    -- Optional confirmation before running
    confirm = "This will modify the image. Continue?",
})
```

**Parameter types:** `text`, `textarea`, `number`, `select`, `boolean`, `hidden`.

## Go-Side Data Structures

### ActionRegistration

```go
// plugin_system/actions.go

type ActionParam struct {
    Name     string   `json:"name"`
    Type     string   `json:"type"`
    Label    string   `json:"label"`
    Required bool     `json:"required"`
    Default  any      `json:"default,omitempty"`
    Options  []string `json:"options,omitempty"`
    Min      *float64 `json:"min,omitempty"`
    Max      *float64 `json:"max,omitempty"`
    Step     *float64 `json:"step,omitempty"`
}

type ActionFilter struct {
    ContentTypes []string `json:"content_types,omitempty"`
    CategoryIDs  []uint   `json:"category_ids,omitempty"`
    NoteTypeIDs  []uint   `json:"note_type_ids,omitempty"`
}

type ActionRegistration struct {
    PluginName  string        `json:"plugin_name"`
    ID          string        `json:"id"`
    Label       string        `json:"label"`
    Description string        `json:"description,omitempty"`
    Icon        string        `json:"icon,omitempty"`
    Entity      string        `json:"entity"`
    Placement   []string      `json:"placement"`
    Filters     ActionFilter  `json:"filters"`
    Params      []ActionParam `json:"params"`
    Async       bool          `json:"async"`
    Confirm     string        `json:"confirm,omitempty"`
    Handler     *lua.LFunction `json:"-"`
}
```

Stored in-memory in the plugin manager (`pm.actions[pluginName]`). Cleaned up on plugin disable, same as hooks/injections.

### Querying

`pm.GetActions(entity string, entityData map[string]any) []ActionRegistration` — returns actions matching entity type + filters. Called by template context providers. The `entityData` carries content type / category ID / note type ID so filtering happens in Go without DB calls.

## Async Job System

### Job Model

```go
type ActionJob struct {
    ID         string         `json:"id"`
    Source     string         `json:"source"`      // "plugin" or "download"
    PluginName string         `json:"pluginName,omitempty"`
    ActionID   string         `json:"actionId,omitempty"`
    Label      string         `json:"label,omitempty"`
    EntityID   uint           `json:"entityId,omitempty"`
    EntityType string         `json:"entityType,omitempty"`
    Status     string         `json:"status"`      // pending, running, completed, failed
    Progress   int            `json:"progress"`    // 0-100
    Message    string         `json:"message"`
    Result     map[string]any `json:"result,omitempty"`
    CreatedAt  time.Time      `json:"createdAt"`
}
```

In-memory, not DB. Jobs pruned after 1 hour.

### Lua-side async handler

```lua
handler = function(ctx)
    local job_id = ctx.job_id

    mah.job_progress(job_id, 10, "Downloading image...")
    local resource = mah.db.get_resource(ctx.entity_id)

    mah.job_progress(job_id, 30, "Sending to AI service...")
    mah.http.post("https://api.example.com/edit", { ... },
    { headers = { Authorization = "Bearer " .. ctx.settings.api_key } },
    function(resp)
        if resp.error then
            mah.job_fail(job_id, resp.error)
            return
        end
        mah.job_progress(job_id, 90, "Saving result...")
        mah.job_complete(job_id, { message = "Image edited", redirect = "/resource?id=" .. ctx.entity_id })
    end)
end
```

New Lua APIs: `mah.job_progress(job_id, percent, message)`, `mah.job_complete(job_id, result)`, `mah.job_fail(job_id, error)`.

## Unified Jobs Panel (renamed from Download Cockpit)

The existing download cockpit becomes the **Jobs** panel. It displays both download jobs and plugin action jobs in a single SSE-powered list.

### What changes

- **SSE stream** (`/v1/jobs/events`) carries both job types, distinguished by `source` field
- **Download jobs** render as before: filename, URL, byte-based progress bar, pause/resume/cancel
- **Plugin jobs** render: action label, entity link, percentage progress bar, status message, "View result" link on completion
- **Section filter** (optional): "All / Downloads / Plugin Actions" — or just a mixed list with icons
- **Keyboard shortcut** stays `Cmd/Ctrl+Shift+D`

### SSE fan-out

The download manager already has `Subscribe() → chan JobEvent`. The plugin manager publishes action job events via the same `EmitEvent` method, so both types flow through one stream.

## GUI Integration — Three Surfaces

### 1. Detail Page Sidebar

Template calls `pm.GetActions("resource", entityData)` and renders buttons before existing plugin slots:

```html
{% for action in pluginActions %}
<button class="sidebar-action-btn"
        data-plugin="{{ action.PluginName }}"
        data-action="{{ action.ID }}"
        data-entity-id="{{ entity.ID }}"
        data-entity-type="resource"
        data-params='{{ action.Params|json }}'
        data-confirm="{{ action.Confirm }}"
        @click="$dispatch('plugin-action', $el.dataset)">
    {{ action.Label }}
</button>
{% endfor %}
```

Same pattern for note and group detail sidebars.

### 2. Card Dropdown (List Views)

Each entity card gets a kebab menu (three dots) when plugin actions exist with `"card"` placement. Actions filtered server-side in the template context provider using entity data already loaded. No extra API call per card.

### 3. Bulk Editor

Actions with `"bulk"` placement appear as buttons in existing bulk editor forms. Clicking opens the modal with "Applying to N selected items". `POST /v1/jobs/action/run` accepts `entity_ids: [1, 2, 3]` — creates one job per entity. Optional `bulk_max` in declaration limits batch size.

### Action Modal

Single Alpine component `pluginActionModal` in base layout. Any surface dispatches `plugin-action` event. Modal:

1. Shows action label + description
2. Renders form fields from `params` declarations
3. Shows confirmation text if `confirm` is set
4. On submit: POSTs to `/v1/jobs/action/run`
5. Sync result → success/error toast, optional redirect
6. Async result → closes modal, opens Jobs panel, job appears with live progress

## API Endpoints

### Renamed (download → jobs)

| Old | New |
|---|---|
| `GET /v1/download/events` | `GET /v1/jobs/events` |
| `POST /v1/download/submit` | `POST /v1/jobs/download/submit` |
| `GET /v1/download/queue` | `GET /v1/jobs/queue` |
| `POST /v1/download/cancel` | `POST /v1/jobs/cancel` |
| `POST /v1/download/pause` | `POST /v1/jobs/pause` |
| `POST /v1/download/resume` | `POST /v1/jobs/resume` |
| `POST /v1/download/retry` | `POST /v1/jobs/retry` |

Old `/v1/download/*` routes kept as aliases for backward compatibility.

### New

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/plugin/actions?entity=resource&id=123` | Get matching actions for an entity |
| `POST` | `/v1/jobs/action/run` | Execute a plugin action |
| `GET` | `/v1/jobs/action/job?id=abc` | Poll a specific job |

### `POST /v1/jobs/action/run`

```json
{
    "plugin": "ai-edit",
    "action": "ai-image-edit",
    "entity_ids": [123],
    "params": {
        "prompt": "make it watercolor",
        "model": "model-a",
        "strength": 0.7
    }
}
```

Sync response: `{ "success": true, "message": "Done", "redirect": "/resource?id=123" }`
Async response: `{ "job_id": "abc-123" }`

## Error Handling

- **Plugin disabled mid-job**: Job context cancelled → status `failed`, message "Plugin was disabled"
- **Invalid params**: Server-side validation (required, min/max, options). Returns 400 with field-level errors. Modal shows errors inline.
- **Handler panics**: Wrapped in recover → `failed` status, sanitized error message. Full stack trace logged server-side.
- **Bulk failures**: Each entity gets its own job. Partial success is fine — failed jobs show individually in Jobs panel.
- **Filter mismatch**: Action simply doesn't appear. No error.
- **Concurrent limit**: Plugin actions share a semaphore (default 3) separate from download semaphore.
- **SSE reconnect**: Existing exponential backoff. `init` event replays all active jobs including plugin jobs.
