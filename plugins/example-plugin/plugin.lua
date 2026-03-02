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
end
