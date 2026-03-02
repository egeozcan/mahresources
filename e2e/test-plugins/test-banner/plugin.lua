plugin = {
    name = "test-banner",
    version = "1.0",
    description = "Test plugin that injects a banner on every page",
    settings = {
        { name = "banner_text", type = "string", label = "Banner Text", default = "Plugin Banner Active" },
        { name = "api_key", type = "password", label = "API Key", required = true },
        { name = "show_banner", type = "boolean", label = "Show Banner", default = true },
        { name = "mode", type = "select", label = "Mode", options = {"simple", "advanced"}, default = "simple" },
        { name = "count", type = "number", label = "Count", default = 5 },
    }
}

function init()
    mah.inject("page_top", function(ctx)
        local text = mah.get_setting("banner_text") or "Plugin Banner Active"
        return '<div data-testid="plugin-banner" style="background:yellow;padding:8px;text-align:center;">' .. text .. '</div>'
    end)

    mah.on("before_note_create", function(data)
        data.name = "[Plugin] " .. data.name
        return data
    end)

    mah.page("test-page", function(ctx)
        return '<div data-testid="plugin-page-content"><h2>Test Plugin Page</h2><p>Method: ' .. ctx.method .. '</p><p>Path: ' .. ctx.path .. '</p></div>'
    end)

    mah.page("echo-query", function(ctx)
        local q = ctx.query.msg or "no message"
        return '<div data-testid="plugin-echo">' .. q .. '</div>'
    end)

    mah.page("show-settings", function(ctx)
        local key = mah.get_setting("api_key") or "not-set"
        local mode = mah.get_setting("mode") or "not-set"
        local count = mah.get_setting("count")
        local countStr = count and tostring(count) or "not-set"
        return '<div data-testid="plugin-settings-display">'
            .. '<span data-testid="setting-api-key">' .. key .. '</span>'
            .. '<span data-testid="setting-mode">' .. mode .. '</span>'
            .. '<span data-testid="setting-count">' .. countStr .. '</span>'
            .. '</div>'
    end)

    mah.menu("Test Page", "test-page")
    mah.menu("Echo Query", "echo-query")
    mah.menu("Show Settings", "show-settings")
end
