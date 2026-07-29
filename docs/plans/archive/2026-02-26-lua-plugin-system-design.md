# Lua Plugin System Design

## Summary

A Lua-based plugin system for mahresources that allows plugins to inject HTML/JS/CSS at named slots throughout the UI and register before/after hooks on entity CRUD operations. Plugins get read-only database access and run in isolated Lua VMs.

## Decisions

- **Lua VM:** gopher-lua (pure Go, Lua 5.1, one VM per plugin for isolation)
- **Plugin path:** Default `./plugins/`, overridable via `-plugin-path` / `PLUGIN_PATH`
- **Loading:** Startup only, no hot-reload
- **Event hooks:** Before and after hooks for create/update/delete on all entities
- **Injection:** Named slots in templates, plugins register render functions
- **DB access:** Read-only queries through typed API, no raw SQL
- **Security:** None — internal network app, all users trusted

## Plugin Structure

Each plugin is a directory containing a `plugin.lua` entry point:

```
plugins/
  auto-tagger/
    plugin.lua
    helpers.lua       # optional
  custom-sidebar/
    plugin.lua
```

### plugin.lua

```lua
plugin = {
    name = "auto-tagger",
    version = "1.0",
    description = "Automatically tags resources based on file type"
}

function init()
    mah.on("before_resource_create", on_resource_create)
    mah.inject("resource_detail_sidebar", render_sidebar)
end

function on_resource_create(resource)
    if resource.content_type:find("image/") then
        resource:add_tag("image")
    end
end

function render_sidebar(ctx)
    return '<div class="p-2">Resource: ' .. ctx.entity.name .. '</div>'
end
```

## Lifecycle

1. Server starts, scan plugin directory for `*/plugin.lua`
2. For each plugin: create `*lua.LState`, pre-register `mah` module
3. Execute `plugin.lua`, read `plugin` metadata table
4. Call `init()` — plugin registers hooks and injections
5. Server runs, calling registered hooks/injections at appropriate points
6. Server shuts down, close all Lua VMs

Loading order: alphabetical by directory name.

## Event Hook System

### Supported Events

Before/after pairs for create, update, delete on each entity:

- `before_resource_create`, `after_resource_create`, `before_resource_update`, `after_resource_update`, `before_resource_delete`, `after_resource_delete`
- `before_note_create`, `after_note_create`, `before_note_update`, `after_note_update`, `before_note_delete`, `after_note_delete`
- `before_group_create`, `after_group_create`, `before_group_update`, `after_group_update`, `before_group_delete`, `after_group_delete`
- `before_tag_create`, `after_tag_create`, `before_tag_update`, `after_tag_update`, `before_tag_delete`, `after_tag_delete`
- `before_category_create`, `after_category_create`, `before_category_update`, `after_category_update`, `before_category_delete`, `after_category_delete`

### Before-hooks

Receive entity data as a Lua table. Can:
- Modify fields (name, description, meta, etc.)
- Call `mah.abort("reason")` to cancel the operation
- Multiple plugins on the same event run in load order

### After-hooks

Receive the saved entity with ID populated. Informational only — cannot modify or abort.

### Entity Data in Lua

```lua
function on_resource_create(resource)
    -- resource.id, resource.name, resource.description
    -- resource.content_type, resource.original_filename
    -- resource.meta (table from JSON)
    -- resource:add_tag("name")
    -- resource:set_meta("key", value)
end
```

### Integration Point

Hooks are called inside existing `application_context` CRUD methods:

```
parse request → run before hooks → save to DB → run after hooks
```

## HTML/JS/CSS Injection

### Named Slots

**Global slots** (every page):

| Slot | Location |
|------|----------|
| `head` | Inside `<head>` |
| `page_top` | Start of body |
| `page_bottom` | End of body |
| `sidebar_top` | Top of sidebar |
| `sidebar_bottom` | Bottom of sidebar |
| `scripts` | Before `</body>` |

**Entity-specific slots** (relevant pages only):

| Slot | Page |
|------|------|
| `resource_detail_before`, `resource_detail_after`, `resource_detail_sidebar` | displayResource |
| `resource_list_before`, `resource_list_after` | listResources |
| `note_detail_before`, `note_detail_after`, `note_detail_sidebar` | displayNote |
| `note_list_before`, `note_list_after` | listNotes |
| `group_detail_before`, `group_detail_after`, `group_detail_sidebar` | displayGroup |
| `group_list_before`, `group_list_after` | listGroups |

### Render Function

```lua
mah.inject("resource_detail_sidebar", function(ctx)
    -- ctx.entity = current entity (detail pages)
    -- ctx.entities = entity list (list pages)
    -- ctx.path = current URL path
    -- ctx.query = URL query params as table
    return '<div class="p-3">Widget content</div>'
end)
```

### Template Integration

New Pongo2 function in templates:

```html
{{ plugin_slot "head" }}
{{ plugin_slot "resource_detail_sidebar" }}
```

Collects output from all plugins registered for the slot, concatenates in load order, outputs unescaped HTML.

## Read-Only Database API

```lua
-- Single entity by ID
mah.db.get_note(id)
mah.db.get_resource(id)
mah.db.get_group(id)
mah.db.get_tag(id)
mah.db.get_category(id)

-- Filtered queries
mah.db.query_notes({ name = "meeting%", limit = 10 })
mah.db.query_resources({ content_type = "image/%", limit = 50 })
mah.db.query_groups({ limit = 20 })

-- Relationship queries
mah.db.get_resource_tags(resource_id)
mah.db.get_resource_notes(resource_id)
mah.db.get_resource_groups(resource_id)
mah.db.get_note_resources(note_id)
mah.db.get_group_children(group_id)
mah.db.get_group_relations(group_id)
```

Returns Lua tables mirroring Go model fields. No auto-loaded associations.

## Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-plugin-path` | `PLUGIN_PATH` | `./plugins` | Plugin directory |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | `false` | Disable all plugins |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Plugin fails to load | Log warning, skip plugin, continue |
| Before-hook runtime error | Log error, skip hook, proceed with operation |
| `mah.abort("reason")` | Cancel operation, return reason as error |
| After-hook error | Log and ignore |
| Injection render error | Log error, render nothing for that slot |

## Plugin Logging

```lua
mah.log("info", "Processed resource " .. resource.id)
mah.log("warn", "Unexpected content type")
```

Routes through the existing application logger.

## Non-goals

- No plugin management UI
- No hot-reload
- No raw SQL access
- No authentication/authorization for plugins
- No plugin dependencies or inter-plugin communication
