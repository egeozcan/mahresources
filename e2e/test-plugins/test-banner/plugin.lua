plugin = {
    name = "test-banner",
    version = "1.0",
    description = "Test plugin that injects a banner on every page"
}

function init()
    mah.inject("page_top", function(ctx)
        return '<div data-testid="plugin-banner" style="background:yellow;padding:8px;text-align:center;">Plugin Banner Active</div>'
    end)
end
