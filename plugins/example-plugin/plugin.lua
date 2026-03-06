-- Example plugin for mahresources
-- Place plugin directories in the plugins/ folder (or your configured -plugin-path)
-- Note: query results are capped at 100 (default limit: 20)

plugin = {
    name = "example-plugin",
    version = "1.0",
    description = "Demonstrates the plugin API -- inject HTML, hook events, and use settings",
    settings = {
        { name = "greeting", type = "string", label = "Greeting Message", default = "Hello from Example Plugin!" },
        { name = "show_footer", type = "boolean", label = "Show Footer Banner", default = true },
    }
}

function init()
    -- Inject a small footer note on every page (controlled by settings)
    mah.inject("page_bottom", function(ctx)
        local show = mah.get_setting("show_footer")
        if show == false then return "" end
        local greeting = mah.get_setting("greeting") or "Powered by plugins"
        return '<div style="text-align:center;padding:4px;color:#888;font-size:12px;">' .. greeting .. '</div>'
    end)

    -- Log when a note is created
    mah.on("after_note_create", function(note)
        mah.log("info", "Note created: " .. (note.name or "unknown"))
    end)

    -- Log when a resource is uploaded
    mah.on("after_resource_create", function(resource)
        mah.log("info", "Resource created: " .. (resource.name or "unknown"))
    end)

    -- Example: fetch data from an external API (async, non-blocking)
    -- mah.http.get("https://api.example.com/status", function(resp)
    --     if resp.error then
    --         mah.log("error", "HTTP request failed: " .. resp.error)
    --         return
    --     end
    --     mah.log("info", "API status: " .. resp.status_code .. " body: " .. resp.body)
    -- end)

    -- Example: POST with custom headers
    -- mah.http.post("https://api.example.com/notify", '{"event":"init"}', {
    --     headers = { ["Content-Type"] = "application/json", ["Authorization"] = "Bearer token" },
    --     timeout = 15
    -- }, function(resp)
    --     if resp.error then
    --         mah.log("error", "Notification failed: " .. resp.error)
    --     end
    -- end)

    -- Register a custom plugin page that uses settings
    mah.page("info", function(ctx)
        local greeting = mah.get_setting("greeting") or "Hello!"
        return "<h2>Example Plugin</h2><p>" .. greeting .. "</p><p>This page is rendered by Lua.</p>"
    end)

    -- Add a menu item for the page (appears in the Plugins dropdown)
    mah.menu("Plugin Info", "info")

    -- =========================================================================
    -- Entity CRUD API Examples (mah.db.*)
    -- =========================================================================
    -- All write functions return (result, nil) on success or (nil, error_string)
    -- on failure. Always check for errors in production plugins.

    -- ---- Creating entities ----

    -- Create a tag
    -- local tag, err = mah.db.create_tag({ name = "auto-tagged" })
    -- if err then mah.log("error", "create_tag failed: " .. err) end

    -- Create a category (for groups / note types)
    -- local cat, err = mah.db.create_category({ name = "Automation" })

    -- Create a resource category
    -- local rcat, err = mah.db.create_resource_category({ name = "Generated" })

    -- Create a note type
    -- local ntype, err = mah.db.create_note_type({ name = "Plugin Log" })

    -- Create a group (owner_id is optional)
    -- local group, err = mah.db.create_group({
    --     name        = "Plugin Workspace",
    --     description = "Created by example-plugin",
    --     category_id = cat.id,   -- reference a category created above
    -- })

    -- Create a note (note_type_id is optional)
    -- local note, err = mah.db.create_note({
    --     name         = "Auto-generated note",
    --     description  = "Created during plugin init",
    --     note_type_id = ntype.id,
    -- })

    -- ---- Updating entities ----
    -- NOTE: updates replace ALL fields; omitted fields revert to defaults.

    -- Update a tag by ID (first arg = id, second arg = fields to set)
    -- local updated_tag, err = mah.db.update_tag(tag.id, { name = "renamed-tag" })

    -- Update a group
    -- local updated_group, err = mah.db.update_group(group.id, {
    --     description = "Updated description",
    -- })

    -- Update a note
    -- local updated_note, err = mah.db.update_note(note.id, {
    --     name = "Revised note title",
    -- })

    -- ---- Deleting entities ----

    -- Delete returns true on success or (nil, error_string) on failure
    -- local ok, err = mah.db.delete_tag(tag.id)
    -- local ok, err = mah.db.delete_note(note.id)
    -- local ok, err = mah.db.delete_group(group.id)
    -- local ok, err = mah.db.delete_resource(42)
    -- local ok, err = mah.db.delete_category(cat.id)
    -- local ok, err = mah.db.delete_resource_category(rcat.id)
    -- local ok, err = mah.db.delete_note_type(ntype.id)

    -- ---- Managing relationships ----

    -- Add/remove tags: mah.db.add_tags(entity_type, entity_id, {tag_id, ...})
    -- entity_type is "note", "resource", or "group"
    -- local ok, err = mah.db.add_tags("note", note.id, {tag.id})
    -- local ok, err = mah.db.remove_tags("note", note.id, {tag.id})

    -- Add/remove groups: mah.db.add_groups(entity_type, entity_id, {group_id, ...})
    -- entity_type is "note" or "resource"
    -- local ok, err = mah.db.add_groups("resource", 1, {group.id})
    -- local ok, err = mah.db.remove_groups("resource", 1, {group.id})

    -- Attach/detach resources to/from a note
    -- local ok, err = mah.db.add_resources_to_note(note.id, {1, 2, 3})
    -- local ok, err = mah.db.remove_resources_from_note(note.id, {3})

    -- ---- Group relations & relation types ----

    -- local rtype, err = mah.db.create_relation_type({ name = "depends-on" })
    -- local rel, err = mah.db.create_group_relation({
    --     from_group_id     = group.id,
    --     to_group_id       = 42,
    --     relation_type_id  = rtype.id,
    -- })
    -- local ok, err = mah.db.delete_group_relation(rel.id)
    -- local ok, err = mah.db.delete_relation_type(rtype.id)

    -- =========================================================================
    -- JSON API Endpoint Example (mah.api)
    -- =========================================================================
    -- Register a JSON API endpoint at /v1/plugins/example-plugin/status
    -- mah.api("GET", "status", function(ctx)
    --     local notes = mah.db.query_notes({ limit = 0 })
    --     local resources = mah.db.query_resources({ limit = 0 })
    --     ctx.json({
    --         plugin = "example-plugin",
    --         notes = #notes,
    --         resources = #resources,
    --         greeting = mah.get_setting("greeting")
    --     })
    -- end)

    -- POST endpoint with custom status and body parsing
    -- mah.api("POST", "webhook", function(ctx)
    --     local payload = mah.json.decode(ctx.body)
    --     mah.kv.set("last_webhook", payload)
    --     mah.log("info", "Webhook received", payload)
    --     ctx.status(201)
    --     ctx.json({ received = true })
    -- end, { timeout = 60 })
end
