# Plugin Activation & Settings Management

## Problem

Plugins are currently always active if present on disk. There is no way to:
- Disable a plugin without removing its files
- Configure plugin settings (e.g., API keys) before activation
- Manage plugins from the UI

## Decisions

- **Storage:** Database table (`plugin_states`) via GORM
- **Settings declaration:** Lua table (`plugin.settings`) in `plugin.lua`
- **Setting types:** string, password, boolean, number, select
- **Runtime access:** `mah.get_setting(name)` Lua API
- **Activation model:** Runtime load/unload (no restart required)
- **Default state:** All plugins disabled until explicitly enabled

## Data Model

### New table: `plugin_states`

| Column | Type | Description |
|--------|------|-------------|
| id | uint (PK) | Auto-increment |
| plugin_name | string (unique) | Matches plugin directory name |
| enabled | bool | Default: false |
| settings_json | text | JSON blob of key-value settings |
| created_at | timestamp | |
| updated_at | timestamp | |

### Lua-side settings declaration

```lua
plugin = {
    name = "weather-widget",
    version = "1.0",
    description = "Shows weather data",
    settings = {
        { name = "api_key",      type = "password", label = "API Key",        required = true },
        { name = "city",         type = "string",   label = "Default City",   default = "Berlin" },
        { name = "units",        type = "select",   label = "Units",          options = {"metric", "imperial"}, default = "metric" },
        { name = "auto_refresh", type = "boolean",   label = "Auto Refresh",  default = true },
        { name = "interval",     type = "number",    label = "Refresh (sec)", default = 300 },
    }
}
```

Settings are parsed at discovery time (before `init()`) to extract metadata for the UI.

## Plugin Lifecycle

### Startup

1. Scan plugin directories, parse `plugin.lua` metadata without calling `init()`
2. For each discovered plugin, ensure a `plugin_states` row exists (create with `enabled = false` if missing)
3. For plugins with `enabled = true`, create Lua VM, run `init()`, register hooks/pages/menus/injections

### Runtime Enable

1. `POST /v1/plugin/{name}/enable`
2. Validate all required settings have values; reject if missing
3. Acquire PluginManager write lock
4. Create Lua VM, execute `init()`
5. Update DB: `enabled = true`
6. Release lock

### Runtime Disable

1. `POST /v1/plugin/{name}/disable`
2. Acquire PluginManager write lock
3. Remove all hooks, injections, pages, menu items for this plugin
4. Close Lua VM
5. Update DB: `enabled = false`
6. Release lock

### Settings Update

1. `POST /v1/plugin/{name}/settings` with JSON body
2. Validate against declared schema (types, required fields, select options)
3. Store in `plugin_states.settings_json`
4. If plugin is enabled, update in-memory settings map
5. No automatic restart; plugins read settings on-demand via `mah.get_setting()`

Settings can be configured before enabling a plugin.

## Management Page

### Route: `/plugins/manage`

Rendered by Go templates (not a plugin page). Accessible from a "Manage Plugins" link in the Plugins dropdown, separated from plugin menu items by a divider.

### Layout

Each discovered plugin shown as a card:
- Name, version, description
- Enable/disable toggle
- Settings form (renders fields based on declared types)
- Visual indicator for missing required settings
- Save Settings button

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/plugins/manage` | List all plugins with state and settings schema |
| POST | `/v1/plugin/{name}/enable` | Enable a plugin |
| POST | `/v1/plugin/{name}/disable` | Disable a plugin |
| POST | `/v1/plugin/{name}/settings` | Update plugin settings |

## Runtime Settings Access

### Lua API

```lua
local key = mah.get_setting("api_key")
```

Reads from an in-memory map (loaded from DB at enable time, updated on settings save). No DB query per call.

### Validation on Save

| Type | Validation |
|------|-----------|
| string | Non-empty if required |
| password | Non-empty if required. Stored plaintext in DB (no-auth trust model). Masked in UI. |
| boolean | Must be true or false |
| number | Must parse as float64 |
| select | Must be one of declared options |

### Required Settings on Enable

Enabling a plugin checks that all `required` settings have values. If not, the enable is rejected with an error listing missing fields.

## Testing

### Go Unit Tests

- Plugin discovery (metadata parsing without init)
- Enable/disable lifecycle (hooks/pages/menus appear/disappear)
- Settings validation (all types, required field enforcement)
- `mah.get_setting()` returns correct values, nil for unknown
- Enable rejected when required settings missing

### E2E Tests (Playwright)

- Management page renders with discovered plugins
- Toggle enable/disable, verify effects appear/disappear
- Settings form renders correct field types
- Save settings, reload, verify persistence
- Required field validation blocks enable
- Plugin reads settings at runtime (test plugin renders setting value on page)

### Test Plugin

Extend or create a test plugin that declares settings and renders them on a page for E2E verification.
