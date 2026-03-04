---
sidebar_position: 10
title: Plugin System
---

# Plugin System

Lua-based plugins extend Mahresources with custom actions, hooks, pages, and menu items. Plugins run in sandboxed VMs, are discovered automatically from a configurable directory, and can be enabled or disabled at runtime.

## Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-plugin-path` | `PLUGIN_PATH` | `./plugins` | Directory to scan for plugin subdirectories |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | `false` | Disable the plugin system entirely |

## Plugin Discovery

At startup, the plugin manager scans the plugin directory for subdirectories containing a `plugin.lua` file. Discovery is sorted alphabetically for deterministic load order.

```
plugins/
+-- my-plugin/
|   +-- plugin.lua
+-- another-plugin/
    +-- plugin.lua
```

During discovery, a temporary Lua VM executes only the top-level code of `plugin.lua` (not `init()`) to read the `plugin` global table for metadata and settings. The temporary VM is then closed.

## Plugin Metadata

Every plugin declares a global `plugin` table:

```lua
plugin = {
    name = "image-processor",
    version = "1.0.0",
    description = "Processes images using external APIs"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Plugin identifier (displayed in management UI) |
| `version` | No | Version string |
| `description` | No | Short description |
| `settings` | No | Array of setting definitions |

## Plugin Lifecycle

1. **Discovery** -- Plugin directory is scanned at startup. Metadata and settings are read from each `plugin.lua`.
2. **State check** -- The database is queried for previously enabled plugins. Those plugins are enabled automatically.
3. **Enable** -- A full Lua VM is created with safe libraries. `plugin.lua` is executed, then `init()` is called (if defined). Hooks, actions, injections, pages, and menus registered during `init()` become active.
4. **Run** -- The plugin responds to hooks, serves pages, and executes actions.
5. **Disable** -- All hooks, injections, pages, menus, and actions are removed. In-flight async actions are awaited. The Lua VM is closed.

## Plugin Settings

Settings are defined in the `plugin.settings` table and appear in the management UI when the plugin is selected.

```lua
plugin = {
    name = "my-plugin",
    settings = {
        { name = "api_key", type = "password", label = "API Key", required = true },
        { name = "model", type = "select", label = "Model", options = {"fast", "quality"}, default = "fast" },
        { name = "max_size", type = "number", label = "Max Size", default = 1024 },
        { name = "enabled", type = "boolean", label = "Feature Enabled", default = true },
        { name = "prefix", type = "string", label = "Output Prefix", default = "processed_" }
    }
}
```

### Setting Types

| Type | Validation | UI Element |
|------|-----------|------------|
| `string` | Required check only | Text input |
| `password` | Required check only | Password input |
| `boolean` | Must be boolean | Checkbox |
| `number` | Must be numeric | Number input |
| `select` | Must match one of `options` | Dropdown |

Required settings must be configured before the plugin can be enabled.

### Reading Settings at Runtime

```lua
local api_key = mah.get_setting("api_key")
local max_size = mah.get_setting("max_size")
```

Returns the setting value with the correct Lua type (string, number, boolean), or `nil` if not set.

## State Persistence

Plugin enabled/disabled state and settings are stored in the database (`PluginState` table). This means:
- Plugins that were enabled before a restart are re-enabled automatically
- Settings survive server restarts
- The plugin directory itself only needs the Lua source files

## Management UI

Navigate to the plugin management page to see all discovered plugins with their name, version, description, and current state (enabled/disabled). From this page:

- Enable or disable individual plugins
- Configure plugin settings
- View registered actions, hooks, and pages

## Management API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/plugins/manage` | List all discovered plugins with state |
| `POST` | `/v1/plugin/enable` | Enable a plugin (form: `name`) |
| `POST` | `/v1/plugin/disable` | Disable a plugin (form: `name`) |
| `POST` | `/v1/plugin/settings` | Save settings (query: `name`, JSON body: key-value pairs) |

### Enable a Plugin

```bash
curl -X POST http://localhost:8181/v1/plugin/enable \
  -d "name=image-processor"
```

Required settings must be saved before enabling. If required settings are missing, the enable request fails with a validation error.

### Save Settings

```bash
curl -X POST "http://localhost:8181/v1/plugin/settings?name=image-processor" \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "sk-abc123",
    "model": "quality"
  }'
```

Only keys declared in `plugin.settings` are persisted; unknown keys are ignored.

## Lua VM Sandboxing

Each enabled plugin runs in an isolated Lua VM with restricted libraries.

**Allowed**: `base`, `table`, `string`, `math`, `coroutine`

**Blocked**: `os`, `io`, `debug`, `package`

**Removed base functions**: `dofile`, `loadfile`, `load`

Each VM has a mutex ensuring single-threaded access. All calls into the VM (hooks, actions, page handlers) acquire this lock.
