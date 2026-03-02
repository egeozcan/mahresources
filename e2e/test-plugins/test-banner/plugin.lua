plugin = {
    name = "test-banner",
    version = "1.0",
    description = "Test plugin that injects a banner on every page"
}

function init()
    mah.inject("page_top", function(ctx)
        return '<div data-testid="plugin-banner" style="background:yellow;padding:8px;text-align:center;">Plugin Banner Active</div>'
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

    mah.menu("Test Page", "test-page")
    mah.menu("Echo Query", "echo-query")
end
