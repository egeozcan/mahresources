---
sidebar_position: 13
title: Plugin Lua API Reference
---

# Plugin Lua API Reference

The `mah` module is available to all enabled plugins and provides database access, HTTP requests, JSON encoding, settings, and job control.

## VM Sandboxing

Each plugin runs in an isolated Lua VM.

**Allowed libraries**: `base`, `table`, `string`, `math`, `coroutine`

**Blocked libraries**: `os`, `io`, `debug`, `package`

**Removed base functions**: `dofile`, `loadfile`, `load`

Each VM has a mutex. All calls (hooks, actions, page handlers, HTTP callbacks) acquire this mutex, ensuring single-threaded execution within a single plugin. Different plugins run in separate VMs and can execute concurrently.

## mah.db -- Database API

Read access to all entity types and write access for Resource creation.

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

Sync functions block the Lua execution until the response arrives. Use these inside action handlers where async callbacks cannot fire (the VM lock is held).

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

## Job Progress Functions

Available in async action handlers. See [Plugin Actions](./plugin-actions.md) for full details.

| Function | Description |
|----------|-------------|
| `mah.job_progress(job_id, percent, message)` | Report progress (0-100) |
| `mah.job_complete(job_id, result_table)` | Mark job completed |
| `mah.job_fail(job_id, error_message)` | Mark job failed |
