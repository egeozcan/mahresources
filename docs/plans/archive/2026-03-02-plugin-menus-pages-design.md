# Plugin Menu Items and Pages

**Date:** 2026-03-02
**Status:** Approved

## Goal

Allow Lua plugins to register custom pages and add menu items to the navigation, extending the application's UI without modifying core code.

## Lua API

### `mah.page(path, handler_fn)`

Registers a page handler under the plugin's namespace.

- `path` — relative path (e.g., `"dashboard"` becomes `/plugins/{plugin-name}/dashboard`)
- `handler_fn(ctx)` — called on each request, returns HTML string
- `ctx` table: `path`, `query` (table), `method`, `headers` (table), `body` (string or nil)
- Multiple pages per plugin allowed

### `mah.menu(label, path)`

Adds a menu item to the "Plugins" dropdown in the navigation.

- `label` — display text
- `path` — relative path (auto-resolved to `/plugins/{plugin-name}/{path}`)
- Multiple items per plugin allowed; items grouped by plugin in dropdown

### Example

```lua
plugin = { name = "analytics", version = "1.0" }

function init()
    mah.page("dashboard", function(ctx)
        local notes = mah.db.query_notes({ limit = 5 })
        local html = "<h2>Recent Notes</h2><ul>"
        for _, n in ipairs(notes) do
            html = html .. "<li>" .. n.name .. "</li>"
        end
        return html .. "</ul>"
    end)

    mah.menu("Dashboard", "dashboard")
end
```

## Go Architecture

### PluginManager Additions

- `PageRegistration` struct: `{PluginName, Path, HandlerFn *lua.LFunction}`
- `MenuRegistration` struct: `{PluginName, Label, FullPath string}`
- `pages` map: `map[string]map[string]*lua.LFunction` (pluginName → path → handler)
- `menuItems` slice: `[]MenuRegistration`
- `GetMenuItems() []MenuRegistration` — returns all registered menu items
- `HandlePage(pluginName, path string, ctx PageContext) (string, error)` — executes Lua handler with VM lock, 5s timeout

### Routing

Single wildcard route in `routes.go`:

```
GET /plugins/{pluginName}/{path:.*}
```

Handled by a new template handler that:
1. Extracts plugin name and path from URL
2. Builds `PageContext` from request (method, query, headers, body)
3. Calls `PluginManager.HandlePage()`
4. Renders `pluginPage.tpl` (extends `base.tpl`) with returned HTML in body block

### Template Changes

- **New:** `templates/pluginPage.tpl` — extends base layout, renders plugin HTML in main area
- **Modified:** `templates/partials/menu.tpl` — adds "Plugins" dropdown (conditional, only shown when `pluginMenuItems` is non-empty)
- **Modified:** `wrapContextWithPlugins` — adds `pluginMenuItems` to every template context

### Menu Data Flow

1. Plugins call `mah.menu()` during `init()` → items stored in PluginManager
2. `wrapContextWithPlugins` adds `pluginMenuItems` to every template context
3. `menu.tpl` renders "Plugins" dropdown if items exist

## Error Handling

- Handler error/timeout (5s): render user-friendly error page within base layout
- Unknown plugin/path: 404 within base layout
- Lua errors logged with `[plugin]` prefix (consistent with existing system)

## Security

- Plugin HTML rendered unescaped (same trust model as injections — plugins are admin-installed)
- POST body uses Go default size limits
- Plugin pages are read-only from routing perspective (no data modification endpoints)

## Testing

- **E2E:** Plugin registers page and menu item → verify dropdown appears, navigate to page, verify content
- **Unit:** `HandlePage()` with test Lua VM
- **404:** Unknown plugin/path returns proper 404
- **Error:** Lua runtime errors in handler produce error page, not crash
