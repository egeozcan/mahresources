---
sidebar_position: 12
title: Plugin Hooks, Injections, Pages & Menus
---

# Plugin Hooks, Injections, Pages & Menus

Plugins can intercept entity operations with hooks, inject HTML into existing pages, register custom pages, and add navigation menu items.

## Hooks

Hooks fire before or after entity operations. Register them during `init()` using `mah.on(event_name, handler)`.

```lua
function init()
    mah.on("before_resource_create", function(data)
        -- modify data before the Resource is created
        data.name = string.upper(data.name)
        return data
    end)

    mah.on("after_resource_create", function(data)
        -- fire-and-forget: log, notify, etc.
        print("Resource created: " .. tostring(data.id))
    end)
end
```

### Before Hooks

Before hooks run sequentially before the operation executes. Each hook has a **5-second timeout**.

| Behavior | Description |
|----------|-------------|
| **Data modification** | Return a table to replace the data for subsequent hooks and the operation |
| **Abort** | Call `mah.abort(reason)` to cancel the operation entirely |
| **Pass-through** | Return nothing to leave the data unchanged |
| **Error handling** | Runtime errors are logged; execution continues to the next hook |

```lua
mah.on("before_note_update", function(data)
    if not data.name or data.name == "" then
        mah.abort("Note name cannot be empty")
    end
    return data
end)
```

### After Hooks

After hooks run sequentially after the operation completes. They are fire-and-forget: return values are ignored and errors are logged without affecting the result. Each hook has a **5-second timeout**.

```lua
mah.on("after_group_delete", function(data)
    -- cleanup or notification logic
end)
```

### Abort Mechanism

`mah.abort(reason)` raises a special Lua error that the hook runner intercepts. The operation is cancelled and the reason is returned to the client. This works in both before hooks and action handlers.

### Complete Hook Reference

All 30 lifecycle hooks, organized by entity type:

| Entity | Before Create | After Create | Before Update | After Update | Before Delete | After Delete |
|--------|--------------|-------------|---------------|-------------|---------------|-------------|
| Resource | `before_resource_create` | `after_resource_create` | `before_resource_update` | `after_resource_update` | `before_resource_delete` | `after_resource_delete` |
| Note | `before_note_create` | `after_note_create` | `before_note_update` | `after_note_update` | `before_note_delete` | `after_note_delete` |
| Group | `before_group_create` | `after_group_create` | `before_group_update` | `after_group_update` | `before_group_delete` | `after_group_delete` |
| Tag | `before_tag_create` | `after_tag_create` | `before_tag_update` | `after_tag_update` | `before_tag_delete` | `after_tag_delete` |
| Category | `before_category_create` | `after_category_create` | `before_category_update` | `after_category_update` | `before_category_delete` | `after_category_delete` |

## Injections

Injections render HTML into named slots on existing pages. Register them during `init()` using `mah.inject(slot_name, render_function)`.

```lua
function init()
    mah.inject("resource_sidebar", function(ctx)
        local resource = mah.db.get_resource(ctx.entity_id)
        if resource and resource.content_type == "image/jpeg" then
            return '<div class="p-2 bg-blue-50 rounded">JPEG image</div>'
        end
        return ""
    end)
end
```

### How Injections Render

1. When a page renders a slot, all registered injection functions for that slot are called
2. Each function receives a context table and must return an HTML string
3. Results from all plugins are concatenated in registration order
4. Each renderer has a **5-second timeout**
5. Errors in individual renderers are logged and skipped (other injections still render)

## Pages

Plugins can serve custom pages at `/plugins/{pluginName}/{path}`. Register them during `init()` using `mah.page(path, handler)`.

```lua
function init()
    mah.page("dashboard", function(ctx)
        local notes = mah.db.query_notes({ limit = 10 })
        local html = "<h1>Plugin Dashboard</h1><ul>"
        for _, note in ipairs(notes) do
            html = html .. "<li>" .. note.name .. "</li>"
        end
        html = html .. "</ul>"
        return html
    end)
end
```

Page handlers have a **30-second timeout**.

### Path Validation

Paths must match `^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$` -- alphanumeric characters, hyphens, underscores, and forward slashes. No leading or trailing slashes.

### Route

```
GET|POST /plugins/{pluginName}/{path}
```

For a plugin named `my-plugin` with `mah.page("dashboard", handler)`, the URL is:

```
http://localhost:8181/plugins/my-plugin/dashboard
```

### PageContext

The handler receives a context table:

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | The full request URL (path + query string) |
| `method` | string | HTTP method (`GET` or `POST`) |
| `query` | table | URL query parameters as key-value pairs |
| `headers` | table | HTTP request headers as key-value pairs |
| `params` | table | Form-decoded parameters (for POST requests) |
| `body` | string | Request body (for POST requests) |

```lua
mah.page("search", function(ctx)
    local query = ctx.query.q or ""
    local results = mah.db.query_resources({ name = query, limit = 20 })
    -- build HTML from results...
    return html
end)
```

## Menus

Add navigation menu items that link to plugin pages. Register them during `init()` using `mah.menu(label, path)`.

```lua
function init()
    mah.page("dashboard", dashboard_handler)
    mah.menu("My Dashboard", "dashboard")
end
```

The path uses the same validation rules as `mah.page()`. The full URL is constructed as `/plugins/{pluginName}/{path}`.

Menu items appear in the application navigation and are removed when the plugin is disabled.

## Complete Example

A plugin that adds a hook, an injection, a page, and a menu item:

```lua
plugin = {
    name = "project-tracker",
    version = "1.0.0",
    description = "Track project status on Groups"
}

function init()
    -- Validate Group metadata before updates
    mah.on("before_group_update", function(data)
        if data.meta and data.meta.status then
            local valid = { active = true, paused = true, completed = true }
            if not valid[data.meta.status] then
                mah.abort("Invalid status: " .. tostring(data.meta.status))
            end
        end
        return data
    end)

    -- Show status badge on Group sidebar
    mah.inject("group_sidebar", function(ctx)
        local group = mah.db.get_group(ctx.entity_id)
        if group and group.meta and group.meta.status then
            return '<span class="px-2 py-1 bg-green-100 rounded">' .. group.meta.status .. '</span>'
        end
        return ""
    end)

    -- Custom status overview page
    mah.page("status", function(ctx)
        local groups = mah.db.query_groups({ limit = 50 })
        local html = "<h1>Project Status</h1><table><tr><th>Name</th><th>Status</th></tr>"
        for _, g in ipairs(groups) do
            html = html .. "<tr><td>" .. g.name .. "</td><td>" .. (g.description or "") .. "</td></tr>"
        end
        return html .. "</table>"
    end)

    mah.menu("Project Status", "status")
end
```

## Related Pages

- [Plugin Lua API Reference](./plugin-lua-api.md) -- includes `mah.api()` for JSON API endpoints
