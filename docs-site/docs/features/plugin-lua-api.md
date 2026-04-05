---
sidebar_position: 13
title: Plugin Lua API Reference
---

# Plugin Lua API Reference

The `mah` module is available to all enabled plugins and provides database read/write access, HTTP requests, JSON encoding, key-value storage, settings, logging, job control, and operation management.

## VM Sandboxing

Each plugin runs in an isolated Lua VM.

**Allowed libraries**: `base`, `table`, `string`, `math`, `coroutine`

**Blocked libraries**: `os`, `io`, `debug`, `package`

**Removed base functions**: `dofile`, `loadfile`, `load`

Each VM has a mutex. All calls (hooks, actions, page handlers, HTTP callbacks) acquire this mutex, ensuring single-threaded execution within a single plugin. Different plugins run in separate VMs and can execute concurrently.

## mah.db -- Database API

Full CRUD access to all entity types, plus relationship management and resource file operations.

### Single Entity Getters

| Function | Returns |
|----------|---------|
| `mah.db.get_note(id)` | Note table or `nil` |
| `mah.db.get_resource(id)` | Resource table or `nil` |
| `mah.db.get_group(id)` | Group table or `nil` |
| `mah.db.get_tag(id)` | Tag table or `nil` |
| `mah.db.get_category(id)` | Category table or `nil` |

All IDs are numbers (float64 in Lua). Returns `nil` on error or not found.

#### Note Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | number | Note ID |
| `name` | string | Note name |
| `description` | string | Note description |
| `meta` | string | JSON-encoded metadata string |
| `note_type` | string | Note Type name (if set) |
| `owner_id` | number | Owner Group ID (if set) |
| `tags` | table | Array of `{ id, name }` |

#### Resource Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | number | Resource ID |
| `name` | string | Resource name |
| `description` | string | Description |
| `meta` | string | JSON-encoded metadata string |
| `content_type` | string | MIME type |
| `original_filename` | string | Original upload filename |
| `hash` | string | SHA1 content hash |
| `owner_id` | number | Owner Group ID (if set) |
| `tags` | table | Array of `{ id, name }` |

#### Group Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | number | Group ID |
| `name` | string | Group name |
| `description` | string | Description |
| `meta` | string | JSON-encoded metadata string |
| `owner_id` | number | Owner Group ID (if set) |
| `category` | string | Category name (if set) |
| `tags` | table | Array of `{ id, name }` |

#### Tag Fields

`id` (number), `name` (string)

#### Category Fields

`id` (number), `name` (string), `description` (string)

### Query Functions

| Function | Filter Fields | Result Fields |
|----------|--------------|---------------|
| `mah.db.query_notes(filter)` | `name`, `limit`, `offset` | `id`, `name`, `description` |
| `mah.db.query_resources(filter)` | `name`, `content_type`, `limit`, `offset` | `id`, `name`, `content_type` |
| `mah.db.query_groups(filter)` | `name`, `limit`, `offset` | `id`, `name`, `description` |

**Limits**: Default 20, maximum 100. **Offset**: Default 0, maximum 10,000.

```lua
local images = mah.db.query_resources({
    content_type = "image/jpeg",
    limit = 50,
    offset = 0
})

for _, img in ipairs(images) do
    print(img.id, img.name)
end
```

### Resource File Access

```lua
local base64_data, mime_type = mah.db.get_resource_data(id)
```

Returns base64-encoded file content and MIME type string. Maximum file size: **50 MB**. Returns `nil` on error or if the file exceeds the size limit.

### Resource Creation

#### From URL

```lua
local resource, err = mah.db.create_resource_from_url(url, options)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Must use `http://` or `https://` scheme |
| `options.name` | string | Override the default URL-based filename |
| `options.description` | string | Resource description |
| `options.owner_id` | number | Owner Group ID |
| `options.tags` | table | Array of Tag IDs |
| `options.groups` | table | Array of Group IDs |

Returns a Resource table (`id`, `name`, `description`, `content_type`, `original_filename`, `hash`, `owner_id`) on success. Returns `nil, error_string` on failure.

```lua
local resource, err = mah.db.create_resource_from_url(
    "https://example.com/image.jpg",
    { name = "Downloaded Image", owner_id = 5, tags = {1, 3} }
)
if not resource then
    print("Error: " .. err)
end
```

#### From Base64 Data

```lua
local resource, err = mah.db.create_resource_from_data(base64_string, options)
```

Same options and return format as `create_resource_from_url`. Default filename is `"plugin_upload"` if no `name` is provided.

### Resource Deletion

```lua
local ok, err = mah.db.delete_resource(id)
```

Returns `true` on success, or `nil, error_string` on failure.

### Group CRUD

```lua
-- Create
local group, err = mah.db.create_group({
    name = "My Group",
    description = "A new group",
    owner_id = 1,
    category_id = 2
})

-- Full update (replaces all fields)
local group, err = mah.db.update_group(group.id, {
    name = "Updated Name",
    description = "Updated description"
})

-- Partial update (preserves unspecified fields)
local group, err = mah.db.patch_group(group.id, {
    description = "Only this field changes"
})

-- Delete
local ok, err = mah.db.delete_group(group.id)
```

All create/update/patch functions return a table on success or `nil, error_string` on failure. Delete returns `true` on success or `nil, error_string` on failure.

### Note CRUD

```lua
local note, err = mah.db.create_note({ name = "Meeting Notes", description = "Q1 planning" })
local note, err = mah.db.update_note(note.id, { name = "Updated Notes" })
local note, err = mah.db.patch_note(note.id, { description = "Revised" })
local ok, err = mah.db.delete_note(note.id)
```

### Tag CRUD

```lua
local tag, err = mah.db.create_tag({ name = "important" })
local tag, err = mah.db.update_tag(tag.id, { name = "critical" })
local tag, err = mah.db.patch_tag(tag.id, { name = "high-priority" })
local ok, err = mah.db.delete_tag(tag.id)
```

### Category CRUD

```lua
local cat, err = mah.db.create_category({ name = "Project", description = "Project groups" })
local cat, err = mah.db.update_category(cat.id, { name = "Active Project" })
local cat, err = mah.db.patch_category(cat.id, { description = "Updated" })
local ok, err = mah.db.delete_category(cat.id)
```

### Resource Category CRUD

```lua
local rc, err = mah.db.create_resource_category({ name = "Photo" })
local rc, err = mah.db.update_resource_category(rc.id, { name = "Photograph" })
local rc, err = mah.db.patch_resource_category(rc.id, { name = "Image" })
local ok, err = mah.db.delete_resource_category(rc.id)
```

### Note Type CRUD

```lua
local nt, err = mah.db.create_note_type({ name = "Meeting" })
local nt, err = mah.db.update_note_type(nt.id, { name = "Meeting Minutes" })
local nt, err = mah.db.patch_note_type(nt.id, { name = "Minutes" })
local ok, err = mah.db.delete_note_type(nt.id)
```

### Group Relation CRUD

```lua
local rel, err = mah.db.create_group_relation({
    from_group_id = 1,
    to_group_id = 2,
    relation_type_id = 3
})
local rel, err = mah.db.update_group_relation({ id = rel.id, name = "updated" })
local rel, err = mah.db.patch_group_relation({ id = rel.id, name = "patched" })
local ok, err = mah.db.delete_group_relation(rel.id)
```

### Relation Type CRUD

```lua
local rt, err = mah.db.create_relation_type({ name = "depends-on" })
local rt, err = mah.db.update_relation_type({ id = rt.id, name = "blocks" })
local rt, err = mah.db.patch_relation_type({ id = rt.id, name = "blocked-by" })
local ok, err = mah.db.delete_relation_type(rt.id)
```

### CRUD Summary

Most entity types follow the `(id, opts)` pattern for update/patch:

| Function Pattern | Returns | Description |
|-----------------|---------|-------------|
| `mah.db.create_{entity}(opts)` | table or `nil, error` | Create a new entity |
| `mah.db.update_{entity}(id, opts)` | table or `nil, error` | Full update (replaces all fields) |
| `mah.db.patch_{entity}(id, opts)` | table or `nil, error` | Partial update (preserves unspecified fields) |
| `mah.db.delete_{entity}(id)` | `true` or `nil, error` | Delete an entity |

**Exceptions:** `group_relation` and `relation_type` use `(opts)` for update/patch with `id` embedded in opts (e.g., `mah.db.update_group_relation({ id = 1, name = "new" })`).

Supported entity types: `group`, `note`, `tag`, `category`, `resource_category`, `note_type`, `group_relation`, `relation_type`, `resource` (delete only).

### Relationship Management

#### Tag Operations

Add or remove tags from resources, notes, or groups:

```lua
-- Add tags to a resource
local ok, err = mah.db.add_tags("resource", 42, {1, 3, 5})

-- Remove tags from a note
local ok, err = mah.db.remove_tags("note", 10, {2, 4})

-- Add tags to a group
local ok, err = mah.db.add_tags("group", 7, {1})
```

| Function | Parameters | Returns |
|----------|-----------|---------|
| `mah.db.add_tags(entity_type, id, tag_ids)` | entity type string, entity ID, array of tag IDs | `true` or `nil, error` |
| `mah.db.remove_tags(entity_type, id, tag_ids)` | entity type string, entity ID, array of tag IDs | `true` or `nil, error` |

Valid `entity_type` values: `"resource"`, `"note"`, `"group"`.

#### Group Operations

Add or remove group associations from resources or notes:

```lua
-- Add groups to a resource
local ok, err = mah.db.add_groups("resource", 42, {1, 2})

-- Remove groups from a note
local ok, err = mah.db.remove_groups("note", 10, {3})
```

| Function | Parameters | Returns |
|----------|-----------|---------|
| `mah.db.add_groups(entity_type, id, group_ids)` | entity type string, entity ID, array of group IDs | `true` or `nil, error` |
| `mah.db.remove_groups(entity_type, id, group_ids)` | entity type string, entity ID, array of group IDs | `true` or `nil, error` |

Valid `entity_type` values: `"resource"`, `"note"`.

#### Resource-Note Associations

Attach or detach resources from notes:

```lua
-- Attach resources to a note
local ok, err = mah.db.add_resources_to_note(10, {42, 43, 44})

-- Detach resources from a note
local ok, err = mah.db.remove_resources_from_note(10, {42})
```

| Function | Parameters | Returns |
|----------|-----------|---------|
| `mah.db.add_resources_to_note(note_id, resource_ids)` | note ID, array of resource IDs | `true` or `nil, error` |
| `mah.db.remove_resources_from_note(note_id, resource_ids)` | note ID, array of resource IDs | `true` or `nil, error` |

## mah.kv -- Key-Value Storage

Persistent key-value storage scoped to the calling plugin. Values are JSON-serialized before storage and JSON-deserialized on read, so Lua tables, strings, numbers, and booleans are all supported.

| Function | Returns | Description |
|----------|---------|-------------|
| `mah.kv.get(key)` | value or `nil` | Read a stored value |
| `mah.kv.set(key, value)` | `nil` | Write a value (overwrites existing) |
| `mah.kv.delete(key)` | `nil` | Delete a stored key |
| `mah.kv.list([prefix])` | table of strings | List keys, optionally filtered by prefix |

```lua
-- Store a table
mah.kv.set("config", { threshold = 0.8, model = "fast" })

-- Read it back
local config = mah.kv.get("config")
print(config.threshold)  -- 0.8

-- List keys with a prefix
local keys = mah.kv.list("cache_")
for _, key in ipairs(keys) do
    print(key)
end

-- Delete a key
mah.kv.delete("config")
```

Data is scoped by plugin name -- plugins cannot access another plugin's keys. To purge all KV data for a disabled plugin, use the `POST /v1/plugin/purge-data` endpoint.

## mah.log -- Logging

```lua
mah.log(level, message, [details])
```

Writes a log entry to the application activity log.

| Parameter | Type | Description |
|-----------|------|-------------|
| `level` | string | `"info"`, `"warning"`, or `"error"` |
| `message` | string | Log message |
| `details` | table | Optional: additional context (JSON-serialized) |

```lua
mah.log("info", "Processing started", { resource_id = 42 })
mah.log("warning", "Rate limit approaching")
mah.log("error", "External API failed", { status = 500, url = "https://api.example.com" })
```

Log entries appear in the activity log with the plugin name as the entity name.

## mah.start_job -- Background Jobs

```lua
local job_id = mah.start_job(label, fn)
```

Creates an async job and runs `fn(job_id)` in a background goroutine. Returns the job ID string immediately. Use this for long-running work outside of action handlers.

| Parameter | Type | Description |
|-----------|------|-------------|
| `label` | string | Display label for the job |
| `fn` | function | Callback receiving `job_id` as its argument |

```lua
local job_id = mah.start_job("Import data", function(jid)
    mah.job_progress(jid, 10, "Reading file...")
    -- do work...
    mah.job_progress(jid, 50, "Processing records...")
    -- more work...
    mah.job_complete(jid, { imported = 150 })
end)
```

The job appears in the job system and is tracked via SSE events.

## mah.http -- HTTP API

Supports both async (callback-based) and sync (blocking) requests.

### Constants

| Constant | Value |
|----------|-------|
| Default timeout | 10 seconds |
| Maximum timeout | 120 seconds |
| Maximum response body | 5 MB |
| Maximum redirects | 10 |
| Maximum concurrent requests | 16 |
| User agent | `mahresources-plugin/1.0` |

### Async Functions

Async functions return immediately. The callback fires later when the response arrives. Only `http://` and `https://` URLs are allowed.

#### mah.http.get(url, [options,] callback)

```lua
mah.http.get("https://api.example.com/data", function(response)
    if response.error then
        print("Error: " .. response.error)
        return
    end
    local data = mah.json.decode(response.body)
    -- process data...
end)
```

#### mah.http.post(url, body, [options,] callback)

```lua
mah.http.post("https://api.example.com/process",
    mah.json.encode({ input = "test" }),
    { headers = { ["Content-Type"] = "application/json" } },
    function(response)
        print(response.status_code, response.body)
    end
)
```

#### mah.http.request(method, url, options, callback)

```lua
mah.http.request("PUT", "https://api.example.com/item/1", {
    headers = { ["Content-Type"] = "application/json", ["Authorization"] = "Bearer token" },
    body = mah.json.encode({ status = "done" }),
    timeout = 30
}, function(response)
    print(response.status_code)
end)
```

#### Options Table

| Field | Type | Description |
|-------|------|-------------|
| `headers` | table | Key-value pairs of HTTP headers |
| `timeout` | number | Request timeout in seconds (max 120) |
| `body` | string | Request body (for `request()` only) |

#### Response Table

| Field | Type | Description |
|-------|------|-------------|
| `status_code` | number | HTTP status code |
| `status` | string | Full status text |
| `body` | string | Response body (truncated at 5 MB) |
| `headers` | table | Lowercase header names, comma-joined values |
| `url` | string | Request URL |
| `method` | string | Request method |

On network error, the response contains `error` (string), `url`, and `method` instead.

Callbacks are queued and executed on the plugin's VM thread with a 5-second deadline per callback.

### Sync Functions

Action handlers MUST use sync HTTP functions. Async callbacks cannot fire while the VM lock is held by the handler, so async requests will silently never complete.

Sync functions block the Lua execution until the response arrives.

#### mah.http.get_sync(url, [options])

```lua
local response = mah.http.get_sync("https://api.example.com/data")
if response.status_code == 200 then
    local data = mah.json.decode(response.body)
end
```

#### mah.http.post_sync(url, body, [options])

```lua
local response = mah.http.post_sync(
    "https://api.example.com/process",
    mah.json.encode({ input = "test" }),
    { headers = { ["Content-Type"] = "application/json" } }
)
```

Returns the same response table format as async functions.

## mah.json -- JSON API

### mah.json.encode(value)

Converts a Lua value to a JSON string. Returns the string on success, or `nil, error` on failure.

**Array detection**: A Lua table is treated as a JSON array if it has consecutive integer keys starting from 1 with no gaps and no string keys. All other tables are encoded as JSON objects.

```lua
mah.json.encode({1, 2, 3})           -- '[1,2,3]'
mah.json.encode({a = 1, b = 2})      -- '{"a":1,"b":2}'
mah.json.encode({1, 2, a = 3})       -- '{"1":1,"2":2,"a":3}' (mixed = object)
```

### mah.json.decode(string)

Parses a JSON string into Lua values. Returns the value on success, or `nil, error` on failure.

| JSON Type | Lua Type |
|-----------|----------|
| object | table (string keys) |
| array | table (integer keys starting at 1) |
| number | number (float64) |
| boolean | boolean |
| null | nil |

```lua
local data, err = mah.json.decode('{"name": "test", "count": 42}')
if data then
    print(data.name, data.count)
end
```

## mah.api -- JSON API Endpoints

Register custom JSON API endpoints accessible at `/v1/plugins/{pluginName}/{path}`.

### mah.api(method, path, handler, [opts])

| Parameter | Type | Description |
|-----------|------|-------------|
| `method` | string | HTTP method: `"GET"`, `"POST"`, `"PUT"`, or `"DELETE"` |
| `path` | string | Endpoint path (alphanumeric, hyphens, underscores, slashes) |
| `handler` | function | Receives a context table with request data and response helpers |
| `opts` | table | Optional. `{ timeout = 30 }` -- seconds (default 30, max 120) |

### Handler Context

The handler receives a single `ctx` table:

| Field | Type | Description |
|-------|------|-------------|
| `ctx.path` | string | Full request URL path |
| `ctx.method` | string | HTTP method |
| `ctx.query` | table | URL query parameters |
| `ctx.params` | table | Form-decoded parameters (empty for non-form requests) |
| `ctx.headers` | table | Request headers (lowercase keys) |
| `ctx.body` | string | Raw request body |
| `ctx.json(data)` | function | Set the JSON response body |
| `ctx.status(code)` | function | Set the HTTP status code (default: 200) |

### Response Behavior

| Scenario | Status | Body |
|----------|--------|------|
| `ctx.json()` called | 200 (or custom via `ctx.status()`) | JSON-encoded data |
| `ctx.json()` not called | 204 No Content | Empty |
| Handler error | 500 | `{"error": "internal plugin error"}` |
| Handler timeout | 504 | `{"error": "handler timed out"}` |
| `mah.abort()` called | 400 | `{"error": "reason"}` |
| Path not found | 404 | `{"error": "endpoint not found"}` |
| Wrong HTTP method | 405 | `{"error": "method not allowed"}` |

### Example

```lua
function init()
    -- GET endpoint returning JSON
    mah.api("GET", "stats", function(ctx)
        local notes = mah.db.query_notes({ limit = 0 })
        ctx.json({ total_notes = #notes, query = ctx.query })
    end)

    -- POST endpoint with custom status
    mah.api("POST", "webhook", function(ctx)
        local payload = mah.json.decode(ctx.body)
        mah.kv.set("last_webhook", payload)
        ctx.status(201)
        ctx.json({ received = true })
    end, { timeout = 60 })

    -- DELETE with no body
    mah.api("DELETE", "cache", function(ctx)
        mah.kv.delete("cached_data")
        ctx.status(204)
    end)
end
```

Duplicate registrations for the same method + path overwrite the previous handler.

## mah.block_type -- Plugin Block Types

Register a custom block type for the note block editor. Call during `init()`.

### mah.block_type(config)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config.type` | string | Yes | Block type name (lowercase, alphanumeric and hyphens, max 50 chars). Automatically prefixed as `plugin:<pluginName>:<type>` |
| `config.label` | string | Yes | Display label in the block type picker |
| `config.render_view` | function | Yes | Lua function that returns an HTML string for view mode |
| `config.render_edit` | function | Yes | Lua function that returns an HTML string for edit mode |
| `config.icon` | string | No | Icon for the block type picker |
| `config.description` | string | No | Description of the block type |
| `config.content_schema` | table | No | JSON Schema (as Lua table) for content validation |
| `config.state_schema` | table | No | JSON Schema (as Lua table) for state validation |
| `config.default_content` | table | No | Default content for new blocks |
| `config.default_state` | table | No | Default state for new blocks |
| `config.filters` | table | No | Restrict availability by `note_type_ids` and/or `category_ids` |

### Render Functions

Both `render_view` and `render_edit` receive a context table:

| Field | Type | Description |
|-------|------|-------------|
| `ctx.block.id` | number | Block ID |
| `ctx.block.content` | table | Block content (parsed from JSON) |
| `ctx.block.state` | table | Block state (parsed from JSON) |
| `ctx.block.position` | string | Lexicographic ordering key |
| `ctx.note.id` | number | Parent note ID |
| `ctx.note.name` | string | Parent note name |
| `ctx.note.note_type_id` | number | Parent note's note type ID |
| `ctx.settings` | table | Plugin settings key-value pairs |

Each function must return an HTML string. Use `mah.html_escape(str)` to escape user-provided content.

The rendered HTML is served via `GET /v1/plugins/{pluginName}/block/render?blockId={id}&mode=view|edit` (see [Custom Block Types](./custom-block-types.md#plugin-block-render-endpoint)).

### Example

```lua
function init()
    mah.block_type({
        type = "quote",
        label = "Quote",
        icon = "Q",
        description = "A styled quotation block",
        content_schema = {
            type = "object",
            properties = {
                text = { type = "string" },
                author = { type = "string" }
            },
            required = {"text"}
        },
        default_content = { text = "", author = "" },
        default_state = {},
        render_view = function(ctx)
            local html = '<blockquote class="border-l-4 pl-4 italic">'
            html = html .. '<p>' .. mah.html_escape(ctx.block.content.text or "") .. '</p>'
            if ctx.block.content.author then
                html = html .. '<footer>— ' .. mah.html_escape(ctx.block.content.author) .. '</footer>'
            end
            return html .. '</blockquote>'
        end,
        render_edit = function(ctx)
            return '<div>'
                .. '<textarea name="text">' .. mah.html_escape(ctx.block.content.text or "") .. '</textarea>'
                .. '<input name="author" value="' .. mah.html_escape(ctx.block.content.author or "") .. '">'
                .. '</div>'
        end,
        filters = {
            note_type_ids = {1, 2}
        }
    })
end
```

## mah.display_type -- Custom Display Renderers

Register a custom display renderer for the schema-driven metadata display on detail views. When a schema property has `"x-display": "plugin:<pluginName>:<type>"`, the plugin's render function is called to produce the HTML.

### mah.display_type(config)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config.type` | string | Yes | Display type name (lowercase, alphanumeric and hyphens, max 50 chars). Automatically prefixed as `plugin:<pluginName>:<type>` |
| `config.label` | string | Yes | Human-readable label for this renderer |
| `config.render` | function | Yes | Lua function that returns an HTML string |

### Render Function

The `render` function receives a context table:

| Field | Type | Description |
|-------|------|-------------|
| `ctx.value` | table | The object value from the entity's metadata |
| `ctx.schema` | table | The JSON Schema of the property |
| `ctx.field_path` | string | Dot-notation path (e.g., `"images"`) |
| `ctx.field_label` | string | Display label (e.g., `"Image Gallery"`) |
| `ctx.settings` | table | Plugin settings key-value pairs |

The function must return an HTML string. The HTML is rendered inside the metadata panel on the detail page, inheriting Tailwind CSS classes from the host page.

The render endpoint is `POST /v1/plugins/{pluginName}/display/render` with a 5-second timeout.

### Schema Usage

Add `x-display` to a property in the Category's MetaSchema:

```json
{
  "type": "object",
  "properties": {
    "gallery": {
      "type": "object",
      "x-display": "plugin:my-plugin:image-grid",
      "properties": { "images": { "type": "array" } }
    }
  }
}
```

When `x-display` is set on an object property, the object is passed whole to the renderer (not flattened into individual fields).

### Example

```lua
function init()
    mah.display_type({
        type = "color-swatch",
        label = "Color Swatch",
        render = function(ctx)
            local hex = ctx.value.hex or "#000000"
            local name = ctx.value.name or hex
            return '<div style="display:flex;align-items:center;gap:8px;">'
                .. '<div style="width:24px;height:24px;border-radius:4px;background:'
                .. mah.html_escape(hex) .. ';border:1px solid #e5e7eb;"></div>'
                .. '<span>' .. mah.html_escape(name) .. '</span>'
                .. '</div>'
        end
    })
end
```

## mah.get_setting(key)

Returns the value of a plugin setting, or `nil` if not set.

```lua
local api_key = mah.get_setting("api_key")  -- string
local max_size = mah.get_setting("max_size") -- number
local enabled = mah.get_setting("enabled")   -- boolean
```

Values are returned with their correct Lua type based on the setting definition.

## mah.abort(reason)

Aborts the current operation (hook or action) with a message. Works in before hooks and action handlers.

```lua
mah.abort("Invalid input: name is required")
```

In before hooks, this cancels the entity operation. In action handlers, this returns `{ success = false, message = reason }`.

## mah.html_escape(str)

Escapes a string for safe HTML output. Replaces `&`, `<`, `>`, `"`, and `'` with their HTML entity equivalents.

| Parameter | Type | Description |
|-----------|------|-------------|
| `str` | string | The string to escape |

Returns the escaped string.

```lua
local safe = mah.html_escape('<script>alert("xss")</script>')
-- Result: &lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;
```

Use this in `render_view` and `render_edit` functions to prevent XSS when rendering user-provided content.

## Job Progress Functions

Available in async action handlers and `mah.start_job` callbacks. See [Plugin Actions](./plugin-actions.md) for full details.

| Function | Description |
|----------|-------------|
| `mah.job_progress(job_id, percent, message)` | Report progress (0-100). SSE updates throttled to 200ms. |
| `mah.job_complete(job_id, result_table)` | Mark job completed. Sets progress to 100. |
| `mah.job_fail(job_id, error_message)` | Mark job failed. |

## Complete Example

A plugin that uses database CRUD, KV storage, logging, and HTTP:

```lua
plugin = {
    name = "data-sync",
    version = "1.0.0",
    description = "Sync group data to an external service",
    settings = {
        { name = "api_url", type = "string", label = "API URL", required = true },
        { name = "api_key", type = "password", label = "API Key", required = true }
    }
}

function init()
    mah.action({
        id = "sync-group",
        label = "Sync to External",
        entity = "group",
        async = true,
        handler = function(ctx)
            local group = mah.db.get_group(ctx.entity_id)
            if not group then
                mah.job_fail(ctx.job_id, "Group not found")
                return
            end

            mah.job_progress(ctx.job_id, 20, "Preparing data...")

            local api_url = mah.get_setting("api_url")
            local api_key = mah.get_setting("api_key")
            local payload = mah.json.encode({
                name = group.name,
                description = group.description,
                meta = group.meta
            })

            mah.job_progress(ctx.job_id, 50, "Sending to API...")

            local response = mah.http.post_sync(
                api_url .. "/groups",
                payload,
                {
                    headers = {
                        ["Content-Type"] = "application/json",
                        ["Authorization"] = "Bearer " .. api_key
                    }
                }
            )

            if response.status_code ~= 200 then
                mah.log("error", "Sync failed", { status = response.status_code })
                mah.job_fail(ctx.job_id, "API returned " .. response.status_code)
                return
            end

            local result = mah.json.decode(response.body)
            mah.kv.set("last_sync_" .. ctx.entity_id, {
                synced = true,
                external_id = result.id
            })

            mah.log("info", "Group synced", { group_id = ctx.entity_id })
            mah.job_complete(ctx.job_id, { message = "Synced", external_id = result.id })
        end
    })
end
```

## Related Pages

- [Plugin System](./plugin-system.md) -- discovery, lifecycle, settings, and management
- [Plugin Actions](./plugin-actions.md) -- action registration, parameters, filters, and execution
- [Plugin Hooks, Injections, Pages & Menus](./plugin-hooks.md) -- hooks, HTML injections, custom pages, and menu items
- [Custom Block Types](./custom-block-types.md) -- adding new block types (built-in and plugin-based)
