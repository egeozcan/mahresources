-- Example plugin for mahresources
-- Place plugin directories in the plugins/ folder (or your configured -plugin-path)
-- Note: query results are capped at 100 (default limit: 20)

plugin = {
    name = "example-plugin",
    version = "1.0",
    description = "Demonstrates the plugin API -- inject HTML and hook into entity events"
}

function init()
    -- Inject a small footer note on every page
    mah.inject("page_bottom", function(ctx)
        return '<div style="text-align:center;padding:4px;font-size:12px;color:#999;">Powered by mahresources plugins</div>'
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
end
