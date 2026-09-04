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

### When Hooks Run Relative to the Database

An **after hook runs once the change is durable** -- after the transaction that made it has committed. A bulk delete of fifty notes commits, then fires fifty `after_note_delete` hooks; if the transaction rolls back, none of them fire. This means a write your after hook makes through `mah.db` is an ordinary write, not one contending with a transaction that is still open.

A **before hook can veto, and a veto means the change does not happen.** On a bulk operation that covers the whole batch: aborting the deletion of one resource in a selection of fifty leaves all fifty in place, and aborting the deletion of one merge loser rolls the entire merge back.

:::warning Writing to the database from a before hook
A before hook may run inside the caller's open transaction, while your `mah.db` write is issued on a separate connection. On SQLite that write contends with the transaction for the writer lock and can fail. Read freely in a before hook; do your writing in the matching after hook, where the transaction has already committed.
:::

### Reentrancy: a Plugin Is Not Notified of Its Own Writes

If a hook dispatch would re-enter a plugin that is **already running on the current call chain**, that plugin's hook is skipped and a warning is logged. The write itself still happens; only the notification is dropped.

This covers two shapes:

- Your plugin hooks `after_tag_create` and something in your plugin creates a tag. Your hook does not fire for that tag.
- Your plugin writes a group, another plugin's `after_group_create` hook writes a note, and your plugin hooks `after_note_create`. Your hook does not fire for that note, because your plugin is still running further up the chain.

The rule applies per plugin: it never suppresses a *different* plugin's hook. (A different plugin's hook can still be skipped for the unrelated reason below, if its own VM is busy.) The rule exists because each plugin runs in a single Lua VM behind a non-reentrant lock: without it, re-entering that VM would block forever.

### When a Hook's Plugin Is Busy

A plugin is single-threaded, so a hook may have to wait for its own plugin's VM -- a long `mah.http.*_sync` call holds it for up to 120 seconds. A hook dispatched from **inside another plugin's code** waits at most 5 seconds, which stops two plugins that hook each other's writes from deadlocking. What happens then depends on which side of the operation the hook is on:

| | On timeout |
|---|---|
| **Before hook** | The operation **fails** with an error. The hook that could not run might have been the one that would have vetoed, so proceeding would let unrelated contention silently disable a guard. |
| **After hook** | Skipped and logged. The change has already committed, so this is a missed notification, not a bypassed check. |

A hook dispatched from ordinary application code -- the common case -- waits as long as it needs to and is never skipped for this reason.

The same split decides what a **cancelled request** does to a hook that is waiting. A before hook stops waiting when the caller that made the write goes away, and the write fails; nothing was written, so nobody is left believing otherwise, and it is the answer the table above already gives for a busy VM. An after hook never stops waiting. It describes a change that has already committed, so abandoning it would leave your plugin's view of the database permanently out of step with the database, and a browser tab that closed is not a reason for that. Hooks deferred by [`mah.db.transaction`](./plugin-lua-api.md#transactions) make that plainer still: they are dispatched when the transaction commits, by which point the request that started it is often gone by design.

Neither rule bounds how long your hook may *run* once it has the VM. That is the 5-second Lua timeout, and a cancelled request does not shorten it.

### Abort Mechanism

`mah.abort(reason)` raises a special Lua error that the hook runner intercepts. The operation is cancelled and the reason is returned to the client. This works in both before hooks and action handlers.

A veto answers **HTTP 400**, the same status a plugin API endpoint gives
`mah.abort`, with the reason as the error message. The status does not depend on
how the reason is worded -- it used to, because the abort reached the HTTP layer
as an ordinary error whose message was scanned for familiar phrases, so
`"this cannot be deleted"` came back 400 and `"protected by policy"` came back
500 for the same event.

### Complete Hook Reference

These 30 entity events, plus the job and mass-edit events below, are the whole set, and `mah.on` refuses anything else. A misspelled event used to register happily and never fire, which left you with a plugin that loaded cleanly and did nothing; now the plugin fails to load, and the error names the event you asked for alongside the ones that exist.

A refusal takes the whole load with it. Everything the plugin registered before the error is swept, so a plugin reported as failed is never half-installed.

The hooks, organized by entity type:

| Entity | Before Create | After Create | Before Update | After Update | Before Delete | After Delete |
|--------|--------------|-------------|---------------|-------------|---------------|-------------|
| Resource | `before_resource_create` | `after_resource_create` | `before_resource_update` | `after_resource_update` | `before_resource_delete` | `after_resource_delete` |
| Note | `before_note_create` | `after_note_create` | `before_note_update` | `after_note_update` | `before_note_delete` | `after_note_delete` |
| Group | `before_group_create` | `after_group_create` | `before_group_update` | `after_group_update` | `before_group_delete` | `after_group_delete` |
| Tag | `before_tag_create` | `after_tag_create` | `before_tag_update` | `after_tag_update` | `before_tag_delete` | `after_tag_delete` |
| Category | `before_category_create` | `after_category_create` | `before_category_update` | `after_category_update` | `before_category_delete` | `after_category_delete` |

### Mass-edit events

One request-scoped pair brackets a **mass edit** — the bulk operation that applies several
edits (tags, related groups/notes/resources, owner, meta) to a whole selection, or to every
entity matching a list page's filter, in one transaction:

| Event | When |
|-------|------|
| `before_mass_edit` | before the edit's transaction opens; `mah.abort` vetoes the whole edit |
| `after_mass_edit` | after the transaction has committed |

The `before_mass_edit` payload carries `entity` (`resource`, `note` or `group`), `count`, the
op list (`ops`, e.g. `["tags.add", "owner.set"]`), and — for a filter-targeted edit —
`target: "filter"` and the raw `filter` string. The full id list is included only up to 100
entities; above that the plugin gets the count only.

Two deliberate limits: the pair is **veto-only** (`mah.abort` refuses the entire edit; there
are no field rewrites, because there are no fields to rewrite), and there is **no per-row
hook**. A per-row `before_resource_update` on a 10,000-entity edit would mean 10,000 Lua
invocations inside one write transaction on single-writer SQLite — the same mistake the bulk
delete hook pair exists to avoid at scale. `after_mass_edit` receives `entity`, `matched`,
`affected` and `ops`.

### Job lifecycle events

Three events fire when a job the download queue runs reaches a terminal state,
which is a download, a group export or import, or an admin maintenance job such
as the similarity recompute:

| Event | When |
|-------|------|
| `after_job_completed` | the job finished successfully |
| `after_job_failed` | the job ended with an error |
| `after_job_cancelled` | the job was cancelled |

```lua
plugin = {
    api_version = 1,
    capabilities = { "hooks", "job_events" },
    name = "job-watcher", version = "1.0", description = "",
}

function init()
    mah.on("after_job_completed", function(data)
        mah.log("job " .. data.job_id .. " (" .. data.source .. ") finished")
        return data
    end)
end
```

The handler receives `job_id`, `source`, `status`, `name`, `url`, `error`, plus
`resource_id` and `total_size` when the job produced them.

:::note These need the `job_events` capability, not `hooks`

An entity hook fires on a write the caller just made. A job event fires when
*any* job in the deployment finishes, whoever started it -- so it is its own
capability. A plugin holding only `hooks` is refused at load, with an error
naming the capability it needs.
:::

A plugin's own action, and a job it starts with `mah.start_job`, does not fire
one: those run on the plugin job system and report through `mah.job_complete` and
`mah.job_fail`.

They are **after-only**: a job that has already finished cannot be vetoed, and
returning a modified table changes nothing. Delivery is best-effort -- the queue
hands the event to a dispatcher and never waits, so under sustained load an
event can be dropped rather than delay a download. A dropped event is logged.

## Injections

Injections render HTML into named slots on existing pages. Register them during `init()` using `mah.inject(slot_name, render_function)`.

```lua
function init()
    mah.inject("resource_detail_sidebar", function(ctx)
        local resource = mah.db.get_resource(ctx.entity_id)
        if resource and resource.content_type == "image/jpeg" then
            return '<div class="p-2 bg-blue-50 rounded">JPEG image</div>'
        end
        return ""
    end)
end
```

An injection runs inside the request that is rendering the page, so it is
cancelled when the reader navigates away instead of running to its 5-second
timeout, and repeated identical `mah.db.mrql_query` calls within one page render
-- across every slot on it -- collapse to a single execution.

Injections run on **every** page for the six slots that live in the base layout,
so keep them cheap: the plugin's VM is single-threaded, and a slow injection
blocks every other surface of that plugin for the duration.

### Slot Names

Slots are declared with `{% plugin_slot "..." %}` in the templates. Injecting into a name outside the set below is refused on the same terms as an unknown event: the plugin fails to load, and the error names the slot you asked for alongside the ones that exist. The full set of 21 slots:

| Slot | Location |
|------|----------|
| `head` | Inside `<head>` on every page |
| `page_top` | Top of the page body, above the main content |
| `sidebar_top` | Top of the global navigation sidebar |
| `sidebar_bottom` | Bottom of the global navigation sidebar |
| `page_bottom` | Bottom of the page body |
| `scripts` | End of the body, alongside script tags |
| `resource_detail_before` | Resource detail page, before the main content |
| `resource_detail_after` | Resource detail page, after the main content |
| `resource_detail_sidebar` | Resource detail page sidebar |
| `note_detail_before` | Note detail page, before the main content |
| `note_detail_after` | Note detail page, after the main content |
| `note_detail_sidebar` | Note detail page sidebar |
| `group_detail_before` | Group detail page, before the main content |
| `group_detail_after` | Group detail page, after the main content |
| `group_detail_sidebar` | Group detail page sidebar |
| `resource_list_before` | Resource list page, before the list |
| `resource_list_after` | Resource list page, after the list |
| `note_list_before` | Note list page, before the list |
| `note_list_after` | Note list page, after the list |
| `group_list_before` | Group list page, before the list |
| `group_list_after` | Group list page, after the list |

### How Injections Render

1. When a page renders a slot, all registered injection functions for that slot are called
2. Each function receives a context table and must return an HTML string
3. Results from all plugins are concatenated in registration order
4. Each renderer has a **5-second timeout**
5. Errors in individual renderers are logged and skipped (other injections still render)

## Pages

Plugins can serve custom pages at `/plugins/{pluginName}/{path}`. Register them during `init()` using `mah.page(path, handler, [opts])`.

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

Pages use the host's standard content-and-sidebar layout by default. A page
whose UI needs the full content width can opt out of the sidebar:

```lua
mah.page("board", function(ctx)
    return '<div class="board"></div>'
end, { hide_sidebar = true })
```

`hide_sidebar` must be a boolean. It applies only to that registered page.

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
| `principal` | table | The acting user: `userId`, `username`, `role`, `isAdmin`, `scopeGroupId`, `superUser`. With auth disabled it reflects the root admin (`superUser = true`). |

By default a group-confined user or guest is refused every plugin-code endpoint with HTTP 403 before the handler runs, so `scopeGroupId` is unset on the requests that reach you. An operator can open one plugin to those accounts (Allow limited users on `/plugins/manage`), and a confined caller then does reach your page with `scopeGroupId` carrying its scope group. See [Plugin Permissions](./plugin-permissions.md). What enforces the subtree either way is `mah.db` being bound to the acting principal, not that field, so do not rely on it for subtree enforcement inside a plugin page.

```lua
mah.page("search", function(ctx)
    local query = ctx.query.q or ""
    local results = mah.db.query_resources({ name = query, limit = 20 })
    -- build HTML from results...
    return html
end)
```

## Static Assets

Files in a plugin's own `public/` directory are served at
`/plugins/<name>/public/<path>` while that plugin is **enabled**. Nothing needs
to be declared: create the directory and the files are reachable.

```
plugins/
  my-plugin/
    plugin.lua
    public/
      app.js
      app.css
```

```lua
mah.inject("head", function(ctx)
    return [[
      <link rel="stylesheet" href="/plugins/my-plugin/public/app.css">
      <script src="/plugins/my-plugin/public/app.js"></script>
    ]]
end)
```

This is what replaces embedding browser code in Lua long strings, where every
page render re-sends the same bytes through the VM lock.

:::caution Do not add `defer` or `type="module"`

`main.js` is a module and therefore deferred, and the `head` slot is emitted
after it. Two deferred scripts run in document order, so a deferred or module
plugin script runs **after** `Alpine.start()` -- `alpine:init` has already fired
and your plugin silently does nothing.

A classic `<script src>` runs before Alpine starts, which is early enough to
register an `alpine:init` listener:

```html
<script src="/plugins/my-plugin/public/app.js"></script>
```
```js
document.addEventListener('alpine:init', () => {
  Alpine.data('myWidget', () => ({ /* ... */ }));
});
```
:::

**No capability is required.** Serving a file grants the plugin nothing -- the Lua
VM has no filesystem access at all, so your plugin cannot read these files; the
host reads them, from a directory whoever installed the plugin already wrote.
An asset matters only when something references it, and every surface that can
(`inject`, `pages`, and the render surfaces: shortcodes, block types and display
types) is capability-gated already. A plugin holding none of them can have a
`public/` directory served with nothing pointing at it.

**Access follows the plugin.** The URL carries the plugin's name in its first
segment, so these files are governed by the same per-plugin "allow group-limited
users" toggle as its pages and shortcodes. A confined account that may not use
the plugin is refused both the `<script>` tag and the file it names.

**Containment.** The directory is opened as a root, so `..` and symlinks pointing
outside `public/` are refused. Directory listings are not served, and neither is
anything from a disabled plugin.

**`public` is reserved.** A plugin page registered at the path `public` is
shadowed by this route.

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
    mah.inject("group_detail_sidebar", function(ctx)
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
