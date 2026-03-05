# Plugin Key-Value Store Design

## Problem

Plugins have no way to persist runtime state. They can manage entities and read settings, but cannot remember things like "I created category #5 for AI-generated images" across restarts.

## Solution

A per-plugin key-value store backed by a new database table, exposed via `mah.kv.*` Lua API.

## Data Model

New GORM model `PluginKV`:

```go
type PluginKV struct {
    ID         uint      `gorm:"primarykey"`
    CreatedAt  time.Time
    UpdatedAt  time.Time
    PluginName string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
    Key        string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
    Value      string    `gorm:"type:text;not null"` // JSON-encoded
}
```

- Composite unique index on `(plugin_name, key)`
- Value stored as JSON text (strings, numbers, booleans, objects, arrays)
- Auto-migrated alongside `PluginState` in `main.go`

## Lua API

```lua
mah.kv.set("my_category_id", 42)
mah.kv.set("config:theme", {primary = "#ff0000", dark = true})

local cat_id = mah.kv.get("my_category_id")  -- 42
local missing = mah.kv.get("nope")            -- nil

mah.kv.delete("old_key")

local all_keys = mah.kv.list()              -- {"config:theme", "my_category_id"}
local config_keys = mah.kv.list("config:")  -- {"config:theme"}
```

- `set`: upserts — overwrites existing key
- `get`: returns deserialized JSON value, nil if not found
- `delete`: no error if key missing
- `list`: returns key names only, sorted alphabetically, optional prefix filter
- All operations auto-scoped to calling plugin (no plugin name argument)

## Go Layer

### Interface

```go
type KVStore interface {
    KVGet(pluginName, key string) (string, bool, error)
    KVSet(pluginName, key, value string) error
    KVDelete(pluginName, key string) error
    KVList(pluginName, prefix string) ([]string, error)
    KVPurge(pluginName string) error
}
```

### Implementation

- `KVGet` — `WHERE plugin_name = ? AND key = ?`
- `KVSet` — GORM upsert with `clause.OnConflict`
- `KVDelete` — `DELETE WHERE plugin_name = ? AND key = ?`
- `KVList` — `SELECT key WHERE plugin_name = ? AND key LIKE ?` with `ORDER BY key`
- `KVPurge` — `DELETE WHERE plugin_name = ?`

### Files

- `models/plugin_kv_model.go` — GORM model
- `plugin_system/kv_api.go` — Lua module registration (`registerKvModule`)
- `application_context/plugin_kv_context.go` — KVStore implementation
- Auto-migrate in `main.go`

## Purge

- "Purge Data" button on plugin management page, only for disabled plugins
- Calls `POST /v1/plugin/purge-data` endpoint
- Deletes all KV rows for that plugin

## Lifecycle

- **Disable:** Data stays in DB, access removed at runtime
- **Re-enable:** Data accessible again
- **Purge:** Manual action from management UI, only when disabled
- **Plugin removed from filesystem:** Orphaned rows stay harmless, can be purged from UI

## Isolation

Strict — each plugin can only access its own keys. No cross-plugin data access.

## Limits

None. Trust plugin authors.

## Concurrent Access

KV operations go through GORM (DB-level locking). Each plugin VM has a mutex, and async jobs acquire the VM lock, so writes from the same plugin are serialized.
