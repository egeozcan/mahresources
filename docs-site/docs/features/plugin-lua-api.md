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

### Reads and Errors

Every read returns two values: the result, and an error string. A read that
failed returns `nil, error_string`; a getter that simply found nothing returns
`nil` with no error. That distinction matters: without it a plugin cannot tell
an empty library from a database outage, and any branch that archives, deletes
or re-uploads on "no rows" acts on a false premise.

```lua
local count, err = mah.db.count_resources({ owner_id = 5 })
if err then
    mah.log("warning", "could not count resources: " .. err)
    return                       -- back off; do NOT treat this as zero
end
if count == 0 then
    -- genuinely empty
end
```

Assigning a single value is still valid, so existing plugins are unaffected:

```lua
local note = mah.db.get_note(1)  -- the error return is simply discarded
```

### Single Entity Getters

| Function | Returns |
|----------|---------|
| `mah.db.get_note(id)` | Note table, or `nil` |
| `mah.db.get_resource(id)` | Resource table, or `nil` |
| `mah.db.get_group(id)` | Group table, or `nil` |
| `mah.db.get_tag(id)` | Tag table, or `nil` |
| `mah.db.get_category(id)` | Category table, or `nil` |
| `mah.db.get_note_type(id)` | Note Type table, or `nil` |
| `mah.db.get_resource_category(id)` | Resource Category table, or `nil` |

All IDs are numbers (float64 in Lua). A missing entity is `nil` with no error;
a failed read is `nil, error_string`.

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
| `width` | number | Pixel width (0 if unknown) |
| `height` | number | Pixel height (0 if unknown) |
| `file_size` | number | File size in bytes |
| `owner_id` | number | Owner Group ID (if set) |
| `tags` | table | Array of `{ id, name }` |
| `groups` | table | Array of `{ id, name }` (only if the resource has groups) |
| `notes` | table | Array of `{ id, name }` (only if the resource has notes) |

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

`id` (number), `name` (string), `description` (string), `custom_header`,
`custom_sidebar`, `custom_summary`, `custom_avatar`, `custom_list_header`,
`custom_mrql_result`, `custom_css`, `meta_schema` (all strings)

#### Note Type Fields

The Category fields above, plus `section_config` (string, JSON-encoded).

#### Resource Category Fields

The Category fields above, plus `auto_detect_rules` (string).

### Taxonomy Listing

| Function | Filter Fields | Returns |
|----------|--------------|---------|
| `mah.db.list_tags(filter)` | `name`, `description`, `sort_by`, `limit`, `offset` | Array of Tag tables |
| `mah.db.list_categories(filter)` | `name`, `description`, `sort_by`, `limit`, `offset` | Array of Category tables |
| `mah.db.list_note_types(filter)` | `name`, `description`, `limit`, `offset` | Array of Note Type tables |
| `mah.db.list_resource_categories(filter)` | `name`, `description`, `limit`, `offset` | Array of Resource Category tables |

Taxonomies have no owner, so these take no scoping fields. **Limits**: default
20, maximum 100. **Offset**: default 0, maximum 10,000.

Find-or-create a tag by name -- the reason these exist:

```lua
local function tag_id_for(name)
    local matches, err = mah.db.list_tags({ name = name })
    if err then return nil, err end
    if #matches > 0 then return matches[1].id end
    local created, createErr = mah.db.create_tag({ name = name })
    if createErr then return nil, createErr end
    return created.id
end
```

### Query Functions

| Function | Filter Fields | Result Fields |
|----------|--------------|---------------|
| `mah.db.query_notes(filter)` | `name`, `owner_id`, `note_type_id`, `tags`, `groups`, `sort_by`, `limit`, `offset` | `id`, `name`, `description`, `meta`, `owner_id`, `created_at`, `updated_at` |
| `mah.db.query_resources(filter)` | `name`, `content_type`, `owner_id`, `resource_category_id`, `tags`, `groups`, `sort_by`, `limit`, `offset` | `id`, `name`, `description`, `content_type`, `original_filename`, `hash`, `meta`, `owner_id`, `created_at`, `updated_at` |
| `mah.db.query_groups(filter)` | `name`, `owner_id`, `category_id`, `tags`, `sort_by`, `limit`, `offset` | `id`, `name`, `description`, `meta`, `owner_id`, `created_at`, `updated_at` |

**Limits**: Default 20, maximum 100. **Offset**: Default 0, maximum 10,000.

Filter field types: `tags` and `groups` accept arrays of numeric IDs. `sort_by` accepts an array of sort strings (e.g., `{"created_at desc", "name"}`).

```lua
local images = mah.db.query_resources({
    content_type = "image/jpeg",
    owner_id = 5,
    tags = {1, 3},
    sort_by = {"created_at desc"},
    limit = 50,
    offset = 0
})

for _, img in ipairs(images) do
    print(img.id, img.name, img.created_at)
end
```

### Count Functions

Return the total number of matching entities as a number, or `nil, error_string` if the count failed. Accept the same filter fields as the corresponding query functions (excluding `limit` and `offset`). A failed count is never reported as `0` -- see [Reads and Errors](#reads-and-errors).

| Function | Description |
|----------|-------------|
| `mah.db.count_notes(filter)` | Count notes matching filter |
| `mah.db.count_resources(filter)` | Count resources matching filter |
| `mah.db.count_groups(filter)` | Count groups matching filter |

```lua
local total = mah.db.count_resources({ owner_id = 5, content_type = "image/%" })
local tagged = mah.db.count_notes({ tags = {1} })
```

### MRQL Query

```lua
local result, err = mah.db.mrql_query("type=resource AND name ~ $needle", {
    limit = 50,
    buckets = 5,
    scope = "entity",             -- "global" | "entity" | "parent" | "root"
    scope_entity_id = ctx.entity_id,
    entity_type = ctx.entity_type,
    params = { needle = "sunset" }, -- binds $name placeholders (value positions only)
})
```

Runs an [MRQL](./mrql.md) query and returns a result table (`{entity_type, mode, items|rows|groups}`) or `nil, error_string`. The `params` table binds `$name` placeholders; values are stringified and coerced like typed literals. Every placeholder must be supplied or the call errors. Results are cached per `(query, resolved scope owner id, limit, buckets, params)`, where the scope owner id is the group id that `scope` resolves to for this entity, not the literal scope string.

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
| `options.meta` | string | JSON-encoded metadata string |

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

### Resource Editing

```lua
local resource, err = mah.db.update_resource(id, opts)
local resource, err = mah.db.patch_resource(id, opts)
```

`update_resource` replaces **every** field, associations included: omitting
`tags` clears the resource's tags. `patch_resource` changes only the keys you
supply and reads the rest back from the stored resource, which is what you want
for anything that edits one field.

Accepted keys: `name`, `description`, `meta` (JSON string), `owner_id`,
`groups`, `tags`, `notes` (arrays of numeric IDs), `category`,
`content_category`, `resource_category_id`, `original_filename`,
`original_location`, `width`, `height`, `series_id`. `update_resource` also
accepts `series_slug`.

`width` and `height` are ignored when `0`, so they cannot be cleared -- the same
rule the HTTP resource-edit path applies, since a resource's pixel dimensions
describe its file.

```lua
-- Fill in an empty description without touching anything else.
local updated, err = mah.db.patch_resource(id, { description = caption })
```

Both return the updated resource table, or `nil, error_string`.

:::caution Patch is last-write-wins
Every `patch_*` function reads the current entity, merges your keys over it, and
writes the whole thing back. The read is not inside the write's transaction, so
a concurrent edit landing in between is overwritten by the values the patch
read: patching a resource's `name` while someone else changes its
`description` restores the description the patch saw. Prefer `patch_*` over
`update_*` regardless -- `update_*` clears every field you omit, including
associations -- but do not treat a patch as an atomic read-modify-write.
:::

### Resource Deletion

```lua
local ok, err = mah.db.delete_resource(id)
```

Returns `true` on success, or `nil, error_string` on failure.

### Resource Versions

```lua
local version, err = mah.db.add_resource_version_from_url(resource_id, url, comment)
```

Downloads the content at `url` and appends it as a new version of an existing resource.

| Parameter | Type | Description |
|-----------|------|-------------|
| `resource_id` | number | ID of the resource to add a version to |
| `url` | string | Must use `http://` or `https://` scheme |
| `comment` | string | Optional version comment (defaults to `""`) |

Returns a version table (`id`, `resource_id`, `version_number`, `content_type`, `file_size`, `hash`) on success, or `nil, error_string` on failure.

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

Action handlers MUST use sync HTTP functions. An async callback cannot fire while the handler holds the VM lock; it is queued and only runs after the handler returns and releases the lock, which is too late to consume inside the handler. Use the sync functions to read a response within a handler.

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

## mah.image -- Image Processing

Image manipulation utilities that operate on base64 data URIs.

### mah.image.pad_to_aspect_ratio(data_uri, target_ratio)

Pads an image with white borders so it exactly matches the target aspect ratio, without stretching or cropping the original content.

| Parameter | Type | Description |
|-----------|------|-------------|
| `data_uri` | string | A `data:image/...;base64,...` URI |
| `target_ratio` | string | Aspect ratio such as `"16:9"`, `"1:1"`, or `"4:3"` |

Returns `padded_data_uri` (string), `new_width` (number), `new_height` (number) on success, or `nil, error_string` on failure. Check the first return value before using it.

```lua
local padded, w, h = mah.image.pad_to_aspect_ratio(data_uri, "16:9")
if not padded then
    print("Error: " .. w)  -- error string is the second return value
    return
end
```

## mah.util -- Clock, Encoding, Hashing

The primitives a plugin cannot build for itself inside the sandbox. The VM opens
`base`, `table`, `string`, `math` and `coroutine` and nothing else, so without
these a plugin has no clock, no base64, and no way to verify a signature.

Every function is a direct wrapper over Go's standard library with no
filesystem, process or network reach.

| Function | Returns |
|----------|---------|
| `mah.util.now()` | Unix seconds as a number, fractional |
| `mah.util.now_iso()` | RFC3339 timestamp in **UTC** |
| `mah.util.base64.encode(str)` | Base64 string |
| `mah.util.base64.decode(str)` | Decoded string, or `nil, error_string` |
| `mah.util.hex.encode(str)` | Lowercase hex string |
| `mah.util.hex.decode(str)` | Decoded string, or `nil, error_string` |
| `mah.util.sha256(str)` | Lowercase hex digest |
| `mah.util.hmac_sha256(key, message)` | Lowercase hex digest |
| `mah.util.secure_compare(a, b)` | Boolean, constant-time for equal-length inputs |

`now_iso()` is UTC deliberately: local-offset timestamps compare
lexicographically against UTC bounds and mis-sort silently.

### Verifying a webhook signature

The reason `hmac_sha256` and `secure_compare` exist -- a `mah.api` endpoint that
cannot check a signature has to trust every caller:

```lua
mah.api("POST", "hook", function(ctx)
    local secret = mah.get_setting("webhook_secret")
    local expected = mah.util.hmac_sha256(secret, ctx.body)
    -- ctx.headers keys are lowercased.
    local supplied = ctx.headers["x-signature"] or ""
    if not mah.util.secure_compare(expected, supplied) then
        ctx.status(401)
        ctx.json({ error = "bad signature" })
        return
    end
    ctx.json({ ok = true })
end)
```

Compare digests with `secure_compare`, not `==`: string equality returns as soon
as it finds a differing byte, which leaks the expected value through timing.

### Caching with a TTL

`mah.kv` round-trips Lua tables through JSON, so a cached value can carry its own
timestamp -- which is what makes it expirable:

```lua
local cached = mah.kv.get("rates")
if cached and (mah.util.now() - cached.fetched_at) < 3600 then
    return cached.value
end

local response = mah.http.get_sync(RATES_URL)
if response.status_code ~= 200 then
    return cached and cached.value or nil   -- serve stale rather than nothing
end
mah.kv.set("rates", { value = response.body, fetched_at = mah.util.now() })
return response.body
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
| `ctx.params` | table | Always empty for `mah.api` handlers; parse `ctx.body` instead |
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
| Handler timeout | 504 | `{"error": "handler timed out after <duration>"}` |
| `mah.abort()` called | 400 (or a custom code set via `ctx.status()`) | `{"error": "reason"}` |
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

### Filters

```lua
filters = {
    note_type_ids = {2},   -- the note's own Note Type
    category_ids  = {7},   -- the Category of the note's OWNING GROUP
}
```

A note has no category of its own, so `category_ids` means "owned by a group in
one of these categories". Both filters are AND-joined, and an empty filter
admits every note. A note that is untyped cannot satisfy `note_type_ids`, and
one that is unowned (or owned by a group with no category) cannot satisfy
`category_ids` -- an unset value is not a wildcard.

The same rule governs both halves: the "+ Add Block" picker lists only the types
a note may use (`GET /v1/note/block/types?noteId=N`), and creating a block the
filters exclude is refused.

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
| `ctx.entity_type` | string | `"resource"`, `"note"` or `"group"`; empty string in the schema editor's preview, which is bound to no stored entity |
| `ctx.entity_id` | number | The entity's ID; `0` in the schema editor's preview |
| `ctx.settings` | table | Plugin settings key-value pairs |

`entity_type` and `entity_id` let a renderer link back to what it is rendering
or fetch a related record. Like `ctx.value`, they are supplied by the browser:
use them to look something up and to build a link, never as proof of who is
asking. Nothing in this context authorizes a write.

The function must return an HTML string. The HTML is rendered inside the metadata panel on the detail page, inheriting Tailwind CSS classes from the host page.

The render endpoint is `POST /v1/plugins/{pluginName}/display/render` with a 5-second timeout.

### Listing Installed Renderers

```
GET /v1/plugin/displayTypes
```

Returns every registered display type as `{ type, label, pluginName }`, where
`type` is the full `plugin:<pluginName>:<type>` string that `x-display` expects.
Use it to offer a picker instead of asking schema authors to hand-type the
string -- a typo in `x-display` degrades silently to the default renderer.

Note the singular `/v1/plugin/` prefix: the catalogue enumerates registrations
and runs no plugin code, unlike the `/v1/plugins/...` endpoints.

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

## mah.shortcode -- Custom Shortcodes

Register a custom shortcode that can be used in category Custom render locations. Call during `init()`.

### mah.shortcode(table)

```lua
mah.shortcode({
    name = "rating",              -- required, lowercase kebab-case
    label = "Star Rating",        -- required, display label
    render = function(ctx)        -- required, returns HTML string
        local max = tonumber(ctx.attrs.max) or 5
        return "<span>" .. string.rep("★", max) .. "</span>"
    end
})
```

Usage in a Custom field: `[plugin:my-plugin:rating max="5"]`

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Shortcode name (lowercase kebab-case, max 50 chars). Automatically prefixed as `plugin:<pluginName>:<name>` |
| `label` | string | Yes | Human-readable display label |
| `render` | function | Yes | Lua function that returns an HTML string |

### Render Context

The `render` function receives a single `ctx` table:

| Field | Description |
|-------|-------------|
| `ctx.entity_type` | `"group"`, `"resource"`, or `"note"` |
| `ctx.entity_id` | Entity ID |
| `ctx.value` | Entity's full Meta as a Lua table |
| `ctx.attrs` | Shortcode attributes as a key-value table |
| `ctx.settings` | Plugin settings key-value pairs |
| `ctx.inner_content` | Content between opening and closing tags (empty for self-closing shortcodes) |
| `ctx.is_block` | `true` if the shortcode was used as a block `[name]...[/name]`, `false` otherwise |
| `ctx.entity` | The full entity as a Lua table (fields vary by type). Present only when the render was given an entity |

### Name Rules

Must match `^[a-z][a-z0-9_-]{0,49}$`. The system expands the shortcode name to `plugin:<plugin-name>:<shortcode-name>` automatically.

### Execution

Server-side at template render time. 5-second timeout per render call. Returned HTML is inlined directly into the page. Use `mah.html_escape(str)` when rendering user-supplied content.

### Block Shortcodes

Plugin shortcodes support block mode. When used as `[plugin:name:sc]content[/plugin:name:sc]`, the render function receives `ctx.inner_content` with the raw content between tags, and `ctx.is_block = true`. Nested shortcodes inside plugin block output are expanded automatically after the plugin render function returns.

In docs preview, nested shortcodes inside plugin block output are not expanded (they render as literal text). This is a preview-only limitation.

### Example

```lua
function init()
    mah.shortcode({
        name = "rating",
        label = "Star Rating",
        render = function(ctx)
            local value = tonumber(ctx.attrs.value) or 0
            local max = tonumber(ctx.attrs.max) or 5
            local stars = string.rep("★", value) .. string.rep("☆", max - value)
            return '<span title="' .. value .. '/' .. max .. '" class="text-yellow-500">'
                .. stars .. '</span>'
        end
    })
end
```

## mah.doc -- General Plugin Documentation

Register a documentation entry for any plugin feature (actions, pages, settings, or custom categories). Entries appear on the plugin's documentation page alongside shortcode docs. Call during `init()`.

### mah.doc(table)

```lua
mah.doc({
    name = "colorize",              -- required, lowercase kebab-case
    label = "Colorize Action",      -- required, display label
    description = "Colorize a black and white image using AI.",
    category = "Action",            -- optional grouping label
    attrs = {                       -- optional parameter docs
        { name = "model", type = "string", required = false, description = "AI model to use", default = "default" }
    },
    examples = {                    -- optional usage examples
        { title = "Basic usage", code = "Select a B&W image and run the action" }
    },
    notes = {                       -- optional notes
        "Requires an API key in plugin settings."
    }
})
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | URL slug (lowercase kebab-case, max 50 chars, must match `^[a-z][a-z0-9_-]{0,49}$`) |
| `label` | string | Yes | Human-readable display label |
| `description` | string | No | Feature description |
| `category` | string | No | Grouping label (e.g. "Action", "Page") |
| `attrs` | table | No | Array of `{name, type, required, description, default}` parameter docs |
| `examples` | table | No | Array of `{title, code, notes, example_data}` usage examples |
| `notes` | table | No | Array of note strings |

Doc entry names must be unique within a plugin and must not conflict with shortcode names in the same plugin.

## mah.get_setting(key)

Returns the value of a plugin setting, or `nil` if not set.

```lua
local api_key = mah.get_setting("api_key")  -- string
local max_size = mah.get_setting("max_size") -- number
local enabled = mah.get_setting("enabled")   -- boolean
```

Values are returned with their correct Lua type based on the setting definition.

## mah.sleep(seconds)

Blocks the calling plugin VM for the given number of seconds. The value is clamped to the range `[0, 30]`: negatives become `0` and anything above `30` becomes `30`. Useful for polling an external async API from within a sync action handler.

```lua
mah.sleep(2)  -- pause for 2 seconds
```

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
